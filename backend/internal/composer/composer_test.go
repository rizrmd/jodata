package composer

import (
	"testing"

	"jodata/internal/dataset"
)

func TestSuggestedPromptsUsesDatasetSemantics(t *testing.T) {
	ds := dataset.Build("dataset_1", "sales", dataset.Source{Type: dataset.SourceCSV, Name: "sales.csv"}, []string{"month", "region", "revenue"}, [][]string{{"2026-01-01", "west", "10"}})
	prompts := SuggestedPrompts(ds, 4)
	if len(prompts) < 3 {
		t.Fatalf("prompts = %#v", prompts)
	}
	if prompts[0] != "revenue by region as a bar chart" {
		t.Fatalf("first prompt = %q", prompts[0])
	}
}
