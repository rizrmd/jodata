package planner

import (
	"regexp"
	"strings"

	"jodata/internal/dataset"
)

type Planner interface {
	Plan(prompt string, ds dataset.Dataset) (ChartIntent, error)
}

type HeuristicPlanner struct{}

func NewHeuristicPlanner() HeuristicPlanner {
	return HeuristicPlanner{}
}

func (HeuristicPlanner) Plan(prompt string, ds dataset.Dataset) (ChartIntent, error) {
	lower := strings.ToLower(prompt)
	intent := ChartIntent{
		DatasetID: ds.ID,
		ChartType: chartType(lower),
		Title:     titleFromPrompt(prompt),
		TimeGrain: "month",
	}

	if metric := pickMetric(lower, ds); metric != "" {
		intent.Metrics = []string{metric}
	}
	if timeColumn := pickTemporal(lower, ds); timeColumn != "" {
		intent.TimeColumn = timeColumn
	}
	intent.GroupBy = pickGroupBy(lower, ds, intent.TimeColumn)

	if len(intent.Metrics) == 0 {
		intent.Metrics = []string{"count_rows"}
	}

	return intent, nil
}

func chartType(prompt string) string {
	switch {
	case strings.Contains(prompt, "line") || strings.Contains(prompt, "trend"):
		return "line"
	case strings.Contains(prompt, "pie") || strings.Contains(prompt, "donut"):
		return "pie"
	case strings.Contains(prompt, "table"):
		return "table"
	default:
		return "bar"
	}
}

func pickMetric(prompt string, ds dataset.Dataset) string {
	for _, metric := range ds.Metrics {
		if aggregateMatches(prompt, metric) && metric.Column != "" && strings.Contains(prompt, strings.ReplaceAll(metric.Column, "_", " ")) {
			return metric.Name
		}
	}
	for _, metric := range ds.Metrics {
		if metric.Column != "" && strings.Contains(prompt, strings.ReplaceAll(metric.Column, "_", " ")) {
			return metric.Name
		}
		if strings.Contains(prompt, strings.ReplaceAll(metric.Name, "_", " ")) {
			return metric.Name
		}
	}
	for _, metric := range ds.Metrics {
		if metric.Name != "count_rows" {
			return metric.Name
		}
	}
	return "count_rows"
}

func aggregateMatches(prompt string, metric dataset.Metric) bool {
	switch strings.ToUpper(metric.Aggregate) {
	case "AVG":
		return strings.Contains(prompt, "average") || strings.Contains(prompt, "avg") || strings.Contains(prompt, "mean")
	case "SUM":
		return strings.Contains(prompt, "sum") || strings.Contains(prompt, "total")
	case "MIN":
		return strings.Contains(prompt, "min") || strings.Contains(prompt, "minimum")
	case "MAX":
		return strings.Contains(prompt, "max") || strings.Contains(prompt, "maximum")
	case "COUNT":
		return strings.Contains(prompt, "count") || strings.Contains(prompt, "number of")
	default:
		return false
	}
}

func pickTemporal(prompt string, ds dataset.Dataset) string {
	if !regexp.MustCompile(`(?i)(month|year|date|time|daily|weekly|monthly|trend)`).MatchString(prompt) {
		return ""
	}
	for _, col := range ds.Columns {
		if col.IsTemporal {
			return col.Name
		}
	}
	return ""
}

func pickGroupBy(prompt string, ds dataset.Dataset, timeColumn string) []string {
	groupBy := []string{}
	for _, col := range ds.Columns {
		if col.Name == timeColumn || col.Role != dataset.RoleDimension {
			continue
		}
		if strings.Contains(prompt, strings.ReplaceAll(col.Name, "_", " ")) || strings.Contains(prompt, col.Name) {
			groupBy = append(groupBy, col.Name)
		}
	}
	if len(groupBy) > 0 {
		return groupBy
	}
	for _, col := range ds.Columns {
		if col.Name != timeColumn && col.Role == dataset.RoleDimension {
			return []string{col.Name}
		}
	}
	return groupBy
}

func titleFromPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "Untitled Chart"
	}
	words := strings.Fields(prompt)
	if len(words) > 10 {
		words = words[:10]
	}
	for i, word := range words {
		words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
	}
	return strings.Join(words, " ")
}
