package ingest

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"jodata/internal/dataset"
)

type Parser struct {
	nextID func() string
}

func NewParser(nextID func() string) Parser {
	return Parser{nextID: nextID}
}

func (p Parser) Parse(filename string, content []byte, profile ParserProfile) (dataset.Dataset, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".csv":
		return p.parseCSV(filename, content, profile)
	case ".json":
		return p.parseJSON(filename, content, profile)
	case ".xlsx", ".xlsm":
		return p.parseXLSX(filename, content, profile)
	case "":
		return p.parseUnknown(filename, content, profile)
	default:
		return dataset.Dataset{}, fmt.Errorf("unsupported source type %q", ext)
	}
}

func (p Parser) parseCSV(filename string, content []byte, profile ParserProfile) (dataset.Dataset, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return dataset.Dataset{}, err
	}
	headerIndex := profile.normalizedHeaderRow() - 1
	if len(records) <= headerIndex {
		return dataset.Dataset{}, fmt.Errorf("header row %d is outside csv", profile.HeaderRow)
	}
	ds := dataset.Build(p.nextID(), baseName(filename), dataset.Source{Type: dataset.SourceCSV, Name: filename}, records[headerIndex], records[headerIndex+1:])
	if err := validateRequiredColumns(ds, profile.RequiredColumns); err != nil {
		return dataset.Dataset{}, err
	}
	return ds, nil
}

func (p Parser) parseJSON(filename string, content []byte, profile ParserProfile) (dataset.Dataset, error) {
	var rows []map[string]any
	if err := json.Unmarshal(content, &rows); err != nil {
		return dataset.Dataset{}, err
	}
	if len(rows) == 0 {
		return dataset.Dataset{}, errors.New("json array is empty")
	}

	headers := make([]string, 0)
	seen := map[string]bool{}
	for _, row := range rows {
		for key := range row {
			if !seen[key] {
				seen[key] = true
				headers = append(headers, key)
			}
		}
	}

	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		record := make([]string, 0, len(headers))
		for _, header := range headers {
			record = append(record, fmt.Sprint(row[header]))
		}
		tableRows = append(tableRows, record)
	}

	ds := dataset.Build(p.nextID(), baseName(filename), dataset.Source{Type: dataset.SourceJSON, Name: filename}, headers, tableRows)
	if err := validateRequiredColumns(ds, profile.RequiredColumns); err != nil {
		return dataset.Dataset{}, err
	}
	return ds, nil
}

func (p Parser) parseXLSX(filename string, content []byte, profile ParserProfile) (dataset.Dataset, error) {
	file, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return dataset.Dataset{}, err
	}
	defer file.Close()

	sheet := profile.PreferredSheet
	if sheet == "" {
		sheets := file.GetSheetList()
		if len(sheets) == 0 {
			return dataset.Dataset{}, errors.New("xlsx has no sheets")
		}
		sheet = sheets[0]
	}

	rows, err := file.GetRows(sheet)
	if err != nil {
		return dataset.Dataset{}, err
	}
	headerIndex := profile.normalizedHeaderRow() - 1
	if headerIndex >= len(rows) {
		return dataset.Dataset{}, fmt.Errorf("header row %d is outside sheet %q", profile.HeaderRow, sheet)
	}
	headers := trimTrailingEmpty(rows[headerIndex])
	dataRows := make([][]string, 0)
	for _, row := range rows[headerIndex+1:] {
		trimmed := trimTrailingEmpty(row)
		if len(trimmed) == 0 {
			continue
		}
		dataRows = append(dataRows, trimmed)
	}

	ds := dataset.Build(p.nextID(), baseName(filename), dataset.Source{Type: dataset.SourceXLSX, Name: filename, SheetName: sheet}, headers, dataRows)
	if err := validateRequiredColumns(ds, profile.RequiredColumns); err != nil {
		return dataset.Dataset{}, err
	}
	return ds, nil
}

func (p Parser) parseUnknown(filename string, content []byte, profile ParserProfile) (dataset.Dataset, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return dataset.Dataset{}, errors.New("cannot parse empty source")
	}
	switch trimmed[0] {
	case '{', '[':
		return p.parseJSON(filename, content, profile)
	default:
		return p.parseCSV(filename, content, profile)
	}
}

func ReadAll(reader io.Reader) ([]byte, error) {
	return io.ReadAll(reader)
}

func validateRequiredColumns(ds dataset.Dataset, required []string) error {
	for _, requiredColumn := range required {
		if _, ok := dataset.FindColumn(ds, requiredColumn); !ok {
			return fmt.Errorf("required column %q was not found", requiredColumn)
		}
	}
	return nil
}

func baseName(filename string) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func trimTrailingEmpty(row []string) []string {
	end := len(row)
	for end > 0 && strings.TrimSpace(row[end-1]) == "" {
		end--
	}
	return row[:end]
}
