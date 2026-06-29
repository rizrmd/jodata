package chart

import (
	"testing"

	"jodata/internal/dataset"
	"jodata/internal/planner"
)

func TestBuildValidatesIntent(t *testing.T) {
	ds := dataset.Build("dataset_1", "sales", dataset.Source{Type: dataset.SourceCSV, Name: "sales.csv"}, []string{"month", "region", "revenue"}, [][]string{{"2026-01-01", "west", "10"}})
	intent := planner.ChartIntent{
		DatasetID:  "dataset_1",
		ChartType:  "bar",
		Title:      "Monthly Revenue",
		Metrics:    []string{"sum_revenue"},
		GroupBy:    []string{"region"},
		TimeColumn: "month",
		TimeGrain:  "month",
	}
	cfg, err := Build("chart_1", ds, intent)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ECharts["series"] == nil {
		t.Fatal("expected echarts series")
	}
	series := cfg.ECharts["series"].([]map[string]any)
	data := series[0]["data"].([]float64)
	if len(data) != 1 || data[0] != 10 {
		t.Fatalf("unexpected series data %#v", data)
	}
}

func TestRejectsUnknownMetric(t *testing.T) {
	ds := dataset.Build("dataset_1", "sales", dataset.Source{Type: dataset.SourceCSV, Name: "sales.csv"}, []string{"region", "revenue"}, [][]string{{"west", "10"}})
	intent := planner.ChartIntent{DatasetID: "dataset_1", ChartType: "bar", Metrics: []string{"does_not_exist"}}
	if err := Validate(ds, intent); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestBuildUsesAverageMetric(t *testing.T) {
	ds := dataset.Build("dataset_1", "sales", dataset.Source{Type: dataset.SourceCSV, Name: "sales.csv"}, []string{"region", "revenue"}, [][]string{{"west", "10"}, {"west", "20"}})
	var err error
	ds, _, err = dataset.AddMetric(ds, dataset.MetricInput{Name: "avg_revenue", Column: "revenue", Aggregate: "AVG"})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Build("chart_1", ds, planner.ChartIntent{
		DatasetID: "dataset_1",
		ChartType: "bar",
		Title:     "Average Revenue",
		Metrics:   []string{"avg_revenue"},
		GroupBy:   []string{"region"},
	})
	if err != nil {
		t.Fatal(err)
	}
	series := cfg.ECharts["series"].([]map[string]any)
	data := series[0]["data"].([]float64)
	if data[0] != 15 {
		t.Fatalf("avg chart data = %#v", data)
	}
}
