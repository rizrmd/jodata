package query

import (
	"testing"

	"jodata/internal/dataset"
	"jodata/internal/planner"
)

func TestRunAggregatesByGroup(t *testing.T) {
	ds := dataset.Build("dataset_1", "sales", dataset.Source{Type: dataset.SourceCSV, Name: "sales.csv"}, []string{"region", "revenue"}, [][]string{{"west", "10"}, {"west", "5"}, {"east", "20"}})
	result, err := Run(ds, Request{
		DatasetID: "dataset_1",
		Metrics:   []string{"sum_revenue"},
		GroupBy:   []string{"region"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d", len(result.Rows))
	}
	if result.Rows[1]["sum_revenue"] != 15.0 {
		t.Fatalf("west revenue = %#v", result.Rows[1]["sum_revenue"])
	}
}

func TestRunFiltersRawRows(t *testing.T) {
	ds := dataset.Build("dataset_1", "sales", dataset.Source{Type: dataset.SourceCSV, Name: "sales.csv"}, []string{"region", "revenue"}, [][]string{{"west", "10"}, {"east", "20"}})
	result, err := Run(ds, Request{
		DatasetID: "dataset_1",
		Filters:   []planner.Filter{{Column: "region", Operator: "=", Value: "east"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["region"] != "east" {
		t.Fatalf("unexpected rows %#v", result.Rows)
	}
}

func TestRunAverageMetric(t *testing.T) {
	ds := dataset.Build("dataset_1", "sales", dataset.Source{Type: dataset.SourceCSV, Name: "sales.csv"}, []string{"region", "revenue"}, [][]string{{"west", "10"}, {"west", "20"}})
	var err error
	ds, _, err = dataset.AddMetric(ds, dataset.MetricInput{Name: "avg_revenue", Column: "revenue", Aggregate: "AVG"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(ds, Request{DatasetID: "dataset_1", Metrics: []string{"avg_revenue"}, GroupBy: []string{"region"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows[0]["avg_revenue"] != 15.0 {
		t.Fatalf("avg revenue = %#v", result.Rows[0]["avg_revenue"])
	}
}
