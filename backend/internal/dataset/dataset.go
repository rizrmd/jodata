package dataset

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type SourceType string

const (
	SourceCSV  SourceType = "csv"
	SourceJSON SourceType = "json"
	SourceXLSX SourceType = "xlsx"
	SourceIntermediary SourceType = "intermediary"
)

type ColumnRole string

const (
	RoleDimension ColumnRole = "dimension"
	RoleMeasure   ColumnRole = "measure"
	RoleTemporal  ColumnRole = "temporal"
)

type DataType string

const (
	TypeString DataType = "string"
	TypeNumber DataType = "number"
	TypeDate   DataType = "date"
	TypeBool   DataType = "bool"
)

type Source struct {
	Type      SourceType `json:"type"`
	Name      string     `json:"name"`
	SheetName string     `json:"sheet_name,omitempty"`
}

type Column struct {
	Name       string     `json:"name"`
	Original   string     `json:"original"`
	Type       DataType   `json:"type"`
	Role       ColumnRole `json:"role"`
	IsTemporal bool       `json:"is_temporal"`
	IsMeasure  bool       `json:"is_measure"`
}

type Metric struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	Expression string `json:"expression"`
	Column     string `json:"column"`
	Aggregate  string `json:"aggregate"`
}

type MetricInput struct {
	Name      string `json:"name"`
	Label     string `json:"label,omitempty"`
	Column    string `json:"column,omitempty"`
	Aggregate string `json:"aggregate"`
}

type Dataset struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Source  Source              `json:"source"`
	Columns []Column            `json:"columns"`
	Metrics []Metric            `json:"metrics"`
	Rows    []map[string]string `json:"rows,omitempty"`
}

func Build(id string, name string, source Source, headers []string, rows [][]string) Dataset {
	cleanHeaders := NormalizeHeaders(headers)
	columns := make([]Column, 0, len(cleanHeaders))

	for i, header := range cleanHeaders {
		values := make([]string, 0, len(rows))
		for _, row := range rows {
			if i < len(row) {
				values = append(values, row[i])
			}
		}
		colType := inferType(values)
		role := roleForType(header, colType)
		columns = append(columns, Column{
			Name:       header,
			Original:   headers[i],
			Type:       colType,
			Role:       role,
			IsTemporal: role == RoleTemporal,
			IsMeasure:  role == RoleMeasure,
		})
	}

	records := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		record := map[string]string{}
		for i, header := range cleanHeaders {
			if i < len(row) {
				record[header] = row[i]
			} else {
				record[header] = ""
			}
		}
		records = append(records, record)
	}

	return Dataset{
		ID:      id,
		Name:    name,
		Source:  source,
		Columns: columns,
		Metrics: defaultMetrics(columns),
		Rows:    records,
	}
}

func NormalizeHeaders(headers []string) []string {
	seen := map[string]int{}
	out := make([]string, 0, len(headers))
	for i, header := range headers {
		name := normalizeName(header)
		if name == "" {
			name = fmt.Sprintf("column_%d", i+1)
		}
		seen[name]++
		if seen[name] > 1 {
			name = fmt.Sprintf("%s_%d", name, seen[name])
		}
		out = append(out, name)
	}
	return out
}

func FindColumn(ds Dataset, name string) (Column, bool) {
	for _, col := range ds.Columns {
		if strings.EqualFold(col.Name, name) || strings.EqualFold(col.Original, name) {
			return col, true
		}
	}
	return Column{}, false
}

func FindMetric(ds Dataset, name string) (Metric, bool) {
	for _, metric := range ds.Metrics {
		if strings.EqualFold(metric.Name, name) || strings.EqualFold(metric.Label, name) || strings.EqualFold(metric.Column, name) {
			return metric, true
		}
	}
	return Metric{}, false
}

func AddMetric(ds Dataset, input MetricInput) (Dataset, Metric, error) {
	metric, err := BuildMetric(ds, input)
	if err != nil {
		return Dataset{}, Metric{}, err
	}
	for _, existing := range ds.Metrics {
		if strings.EqualFold(existing.Name, metric.Name) {
			return Dataset{}, Metric{}, fmt.Errorf("metric %q already exists", metric.Name)
		}
	}
	ds.Metrics = append(ds.Metrics, metric)
	return ds, metric, nil
}

func BuildMetric(ds Dataset, input MetricInput) (Metric, error) {
	aggregate := strings.ToUpper(strings.TrimSpace(input.Aggregate))
	if aggregate == "" {
		aggregate = "SUM"
	}
	if !validAggregate(aggregate) {
		return Metric{}, fmt.Errorf("unsupported aggregate %q", input.Aggregate)
	}
	column := strings.TrimSpace(input.Column)
	if aggregate != "COUNT" {
		col, ok := FindColumn(ds, column)
		if !ok {
			return Metric{}, fmt.Errorf("metric column %q does not exist", column)
		}
		if col.Role != RoleMeasure {
			return Metric{}, fmt.Errorf("metric column %q is not a measure", column)
		}
		column = col.Name
	}
	name := normalizeName(input.Name)
	if name == "" {
		if aggregate == "COUNT" {
			name = "count_rows"
		} else {
			name = strings.ToLower(aggregate) + "_" + normalizeName(column)
		}
	}
	label := strings.TrimSpace(input.Label)
	if label == "" {
		label = strings.ReplaceAll(name, "_", " ")
	}
	expression, err := metricExpression(aggregate, column)
	if err != nil {
		return Metric{}, err
	}
	return Metric{Name: name, Label: label, Expression: expression, Column: column, Aggregate: aggregate}, nil
}

func validAggregate(aggregate string) bool {
	switch aggregate {
	case "SUM", "AVG", "MIN", "MAX", "COUNT":
		return true
	default:
		return false
	}
}

func metricExpression(aggregate string, column string) (string, error) {
	if aggregate == "COUNT" {
		return "COUNT(*)", nil
	}
	if column == "" {
		return "", errors.New("metric column is required")
	}
	return aggregate + "(" + normalizeName(column) + ")", nil
}

func normalizeName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteRune('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func inferType(values []string) DataType {
	total := 0
	numbers := 0
	dates := 0
	bools := 0

	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		total++
		if _, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", ""), 64); err == nil {
			numbers++
			continue
		}
		if isDate(value) {
			dates++
			continue
		}
		if _, err := strconv.ParseBool(strings.ToLower(value)); err == nil {
			bools++
			continue
		}
	}

	if total == 0 {
		return TypeString
	}
	if numbers*100/total >= 80 {
		return TypeNumber
	}
	if dates*100/total >= 80 {
		return TypeDate
	}
	if bools*100/total >= 80 {
		return TypeBool
	}
	return TypeString
}

func isDate(value string) bool {
	layouts := []string{
		time.RFC3339,
		"2006-01-02",
		"01/02/2006",
		"1/2/2006",
		"2006/01/02",
		"Jan 2006",
		"January 2006",
	}
	for _, layout := range layouts {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}

func roleForType(name string, typ DataType) ColumnRole {
	if typ == TypeDate || regexp.MustCompile(`(?i)(date|time|month|year|day)`).MatchString(name) {
		return RoleTemporal
	}
	if typ == TypeNumber {
		return RoleMeasure
	}
	return RoleDimension
}

func defaultMetrics(columns []Column) []Metric {
	metrics := []Metric{{Name: "count_rows", Label: "Count rows", Expression: "COUNT(*)", Aggregate: "COUNT"}}
	for _, col := range columns {
		if col.Role == RoleMeasure {
			metrics = append(metrics, Metric{
				Name:       "sum_" + col.Name,
				Label:      "Sum " + strings.ReplaceAll(col.Name, "_", " "),
				Expression: "SUM(" + col.Name + ")",
				Column:     col.Name,
				Aggregate:  "SUM",
			})
		}
	}
	return metrics
}
