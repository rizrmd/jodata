package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"jodata/internal/dataset"
)

type LLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

type LLMPlanner struct {
	config LLMConfig
	client *http.Client
}

func NewLLMPlanner(config LLMConfig) (LLMPlanner, error) {
	if config.APIKey == "" {
		return LLMPlanner{}, errors.New("llm api key is required")
	}
	if config.BaseURL == "" {
		config.BaseURL = "https://api.openai.com/v1"
	}
	if config.Model == "" {
		config.Model = "gpt-4.1-mini"
	}
	if config.Timeout == 0 {
		config.Timeout = 20 * time.Second
	}
	return LLMPlanner{
		config: config,
		client: &http.Client{Timeout: config.Timeout},
	}, nil
}

func (p LLMPlanner) Plan(prompt string, ds dataset.Dataset) (ChartIntent, error) {
	reqBody := chatCompletionRequest{
		Model: p.config.Model,
		Messages: []message{
			{Role: "system", Content: systemPrompt()},
			{Role: "user", Content: userPrompt(prompt, ds)},
		},
		Temperature:    0,
		ResponseFormat: map[string]string{"type": "json_object"},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return ChartIntent{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.config.Timeout)
	defer cancel()

	endpoint := strings.TrimRight(p.config.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChartIntent{}, err
	}
	req.Header.Set("authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("content-type", "application/json")

	res, err := p.client.Do(req)
	if err != nil {
		return ChartIntent{}, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ChartIntent{}, fmt.Errorf("llm planner status %d", res.StatusCode)
	}

	var completion chatCompletionResponse
	if err := json.NewDecoder(res.Body).Decode(&completion); err != nil {
		return ChartIntent{}, err
	}
	if len(completion.Choices) == 0 {
		return ChartIntent{}, errors.New("llm returned no choices")
	}

	var intent ChartIntent
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &intent); err != nil {
		return ChartIntent{}, fmt.Errorf("llm returned invalid chart intent json: %w", err)
	}
	intent.DatasetID = ds.ID
	return intent, nil
}

type chatCompletionRequest struct {
	Model          string            `json:"model"`
	Messages       []message         `json:"messages"`
	Temperature    float64           `json:"temperature"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
}

func systemPrompt() string {
	return strings.TrimSpace(`You create BI chart intents from user requests.
Return only valid JSON with this shape:
{
  "dataset_id": "string",
  "chart_type": "bar|line|pie|table",
  "title": "string",
  "metrics": ["metric_name"],
  "group_by": ["column_name"],
  "time_column": "column_name or empty",
  "time_grain": "day|week|month|quarter|year or empty",
  "filters": [{"column":"column_name","operator":"=|!=|CONTAINS","value":"string"}]
}
Use only dataset columns and metrics provided by the user message.
Prefer existing metrics over inventing expressions.
If the request is ambiguous, choose the simplest valid chart.`)
}

func userPrompt(prompt string, ds dataset.Dataset) string {
	payload := map[string]any{
		"request": prompt,
		"dataset": map[string]any{
			"id":      ds.ID,
			"name":    ds.Name,
			"columns": ds.Columns,
			"metrics": ds.Metrics,
		},
		"supported_chart_types": []string{"bar", "line", "pie", "table"},
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}
