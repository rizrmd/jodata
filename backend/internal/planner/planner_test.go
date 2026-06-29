package planner

import (
	"testing"

	"jodata/internal/dataset"
)

func TestHeuristicPlannerPicksAverageMetric(t *testing.T) {
	ds := dataset.Build("dataset_1", "sales", dataset.Source{Type: dataset.SourceCSV, Name: "sales.csv"}, []string{"region", "revenue"}, [][]string{{"west", "10"}})
	var err error
	ds, _, err = dataset.AddMetric(ds, dataset.MetricInput{Name: "avg_revenue", Column: "revenue", Aggregate: "AVG"})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewHeuristicPlanner().Plan("average revenue by region", ds)
	if err != nil {
		t.Fatal(err)
	}
	if intent.Metrics[0] != "avg_revenue" {
		t.Fatalf("metric = %s", intent.Metrics[0])
	}
}
