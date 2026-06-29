package ingest

import (
	"testing"

	"jodata/internal/dataset"
)

func TestParseCSVBuildsSemanticDataset(t *testing.T) {
	parser := NewParser(func() string { return "dataset_1" })
	ds, err := parser.Parse("sales.csv", []byte("month,region,revenue\n2026-01-01,west,10\n2026-02-01,east,20\n"), ParserProfile{})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Source.Type != dataset.SourceCSV {
		t.Fatalf("source type = %s", ds.Source.Type)
	}
	if _, ok := dataset.FindColumn(ds, "month"); !ok {
		t.Fatal("expected month column")
	}
	if _, ok := dataset.FindMetric(ds, "sum_revenue"); !ok {
		t.Fatal("expected sum_revenue metric")
	}
}

func TestParseJSONBuildsDataset(t *testing.T) {
	parser := NewParser(func() string { return "dataset_1" })
	ds, err := parser.Parse("sales.json", []byte(`[{"month":"2026-01-01","region":"west","revenue":10}]`), ParserProfile{})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Source.Type != dataset.SourceJSON {
		t.Fatalf("source type = %s", ds.Source.Type)
	}
	if len(ds.Rows) != 1 {
		t.Fatalf("rows = %d", len(ds.Rows))
	}
}

func TestParseCSVUsesProfileHeaderRowAndRequiredColumns(t *testing.T) {
	parser := NewParser(func() string { return "dataset_1" })
	ds, err := parser.Parse("sales.csv", []byte("generated report\nmonth,region,revenue\n2026-01-01,west,10\n"), ParserProfile{
		HeaderRow:       2,
		RequiredColumns: []string{"month", "revenue"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Columns[0].Name != "month" {
		t.Fatalf("first column = %s", ds.Columns[0].Name)
	}
}

func TestParseWithoutFileExtensionUsesHeuristicForJSON(t *testing.T) {
	parser := NewParser(func() string { return "dataset_1" })
	ds, err := parser.Parse("raw", []byte(`[{"month":"2026-01-01","region":"west","revenue":10}]`), ParserProfile{})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Source.Type != dataset.SourceJSON {
		t.Fatalf("source type = %s", ds.Source.Type)
	}
}

func TestParseWithoutFileExtensionUsesHeuristicForCSV(t *testing.T) {
	parser := NewParser(func() string { return "dataset_1" })
	ds, err := parser.Parse("raw", []byte("month,region,revenue\n2026-01-01,west,10\n"), ParserProfile{})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Source.Type != dataset.SourceCSV {
		t.Fatalf("source type = %s", ds.Source.Type)
	}
}
