package planner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jodata/internal/dataset"
)

func TestLLMPlannerParsesChartIntent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("authorization") != "Bearer test-key" {
			t.Fatalf("missing auth header")
		}
		intent := ChartIntent{
			DatasetID:  "ignored",
			ChartType:  "line",
			Title:      "Revenue Trend",
			Metrics:    []string{"sum_revenue"},
			TimeColumn: "month",
			TimeGrain:  "month",
		}
		content, _ := json.Marshal(intent)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": string(content)}},
			},
		})
	}))
	defer server.Close()

	planner, err := NewLLMPlanner(LLMConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	ds := dataset.Build("dataset_1", "sales", dataset.Source{Type: dataset.SourceCSV, Name: "sales.csv"}, []string{"month", "revenue"}, [][]string{{"2026-01-01", "10"}})
	intent, err := planner.Plan("revenue trend", ds)
	if err != nil {
		t.Fatal(err)
	}
	if intent.DatasetID != "dataset_1" {
		t.Fatalf("dataset id = %s", intent.DatasetID)
	}
	if intent.ChartType != "line" {
		t.Fatalf("chart type = %s", intent.ChartType)
	}
}

func TestFallbackPlannerUsesHeuristicOnPrimaryFailure(t *testing.T) {
	ds := dataset.Build("dataset_1", "sales", dataset.Source{Type: dataset.SourceCSV, Name: "sales.csv"}, []string{"region", "revenue"}, [][]string{{"west", "10"}})
	badLLM, err := NewLLMPlanner(LLMConfig{
		BaseURL: "http://127.0.0.1:1",
		APIKey:  "test-key",
		Timeout: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := FallbackPlanner{Primary: badLLM, Fallback: NewHeuristicPlanner()}.Plan("revenue by region", ds)
	if err != nil {
		t.Fatal(err)
	}
	if intent.ChartType != "bar" {
		t.Fatalf("fallback chart type = %s", intent.ChartType)
	}
}
