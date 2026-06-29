package chart

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"jodata/internal/dataset"
	"jodata/internal/planner"
)

type Config struct {
	ID        string              `json:"id"`
	DatasetID string              `json:"dataset_id"`
	Title     string              `json:"title"`
	Type      string              `json:"type"`
	Intent    planner.ChartIntent `json:"intent"`
	ECharts   map[string]any      `json:"echarts"`
}

func Build(id string, ds dataset.Dataset, intent planner.ChartIntent) (Config, error) {
	if err := Validate(ds, intent); err != nil {
		return Config{}, err
	}

	return Config{
		ID:        id,
		DatasetID: ds.ID,
		Title:     intent.Title,
		Type:      intent.ChartType,
		Intent:    intent,
		ECharts:   echartsConfig(ds, intent),
	}, nil
}

func Validate(ds dataset.Dataset, intent planner.ChartIntent) error {
	if intent.DatasetID != ds.ID {
		return fmt.Errorf("intent dataset %q does not match dataset %q", intent.DatasetID, ds.ID)
	}
	if intent.ChartType == "" {
		return errors.New("chart type is required")
	}
	if !supportedChart(intent.ChartType) {
		return fmt.Errorf("unsupported chart type %q", intent.ChartType)
	}
	if len(intent.Metrics) == 0 {
		return errors.New("at least one metric is required")
	}
	for _, metric := range intent.Metrics {
		if _, ok := dataset.FindMetric(ds, metric); !ok {
			return fmt.Errorf("metric %q does not exist", metric)
		}
	}
	for _, group := range intent.GroupBy {
		col, ok := dataset.FindColumn(ds, group)
		if !ok {
			return fmt.Errorf("group by column %q does not exist", group)
		}
		if col.Role == dataset.RoleMeasure {
			return fmt.Errorf("group by column %q is a measure", group)
		}
	}
	if intent.TimeColumn != "" {
		col, ok := dataset.FindColumn(ds, intent.TimeColumn)
		if !ok {
			return fmt.Errorf("time column %q does not exist", intent.TimeColumn)
		}
		if !col.IsTemporal {
			return fmt.Errorf("time column %q is not temporal", intent.TimeColumn)
		}
	}
	for _, filter := range intent.Filters {
		if _, ok := dataset.FindColumn(ds, filter.Column); !ok {
			return fmt.Errorf("filter column %q does not exist", filter.Column)
		}
	}
	return nil
}

func supportedChart(chartType string) bool {
	switch strings.ToLower(chartType) {
	case "bar", "line", "pie", "table":
		return true
	default:
		return false
	}
}

func echartsConfig(ds dataset.Dataset, intent planner.ChartIntent) map[string]any {
	switch intent.ChartType {
	case "table":
		return map[string]any{
			"dataset": map[string]any{"source": ds.Rows},
			"title":   map[string]any{"text": intent.Title},
		}
	case "pie":
		category := firstCategory(intent)
		points := aggregateRows(ds, intent, category)
		data := make([]map[string]any, 0, len(points))
		for _, point := range points {
			data = append(data, map[string]any{"name": point.Category, "value": point.Value})
		}
		return map[string]any{
			"title":  map[string]any{"text": intent.Title},
			"series": []map[string]any{{"type": "pie", "name": intent.Metrics[0], "data": data}},
		}
	default:
		xAxis := firstCategory(intent)
		if intent.TimeColumn != "" {
			xAxis = intent.TimeColumn
		}
		points := aggregateRows(ds, intent, xAxis)
		categories := make([]string, 0, len(points))
		values := make([]float64, 0, len(points))
		for _, point := range points {
			categories = append(categories, point.Category)
			values = append(values, point.Value)
		}
		return map[string]any{
			"title":   map[string]any{"text": intent.Title},
			"tooltip": map[string]any{"trigger": "axis"},
			"xAxis":   map[string]any{"type": "category", "name": xAxis, "data": categories},
			"yAxis":   map[string]any{"type": "value"},
			"series":  []map[string]any{{"type": intent.ChartType, "name": intent.Metrics[0], "data": values}},
		}
	}
}

type point struct {
	Category string
	Value    float64
}

func firstCategory(intent planner.ChartIntent) string {
	if intent.TimeColumn != "" {
		return intent.TimeColumn
	}
	if len(intent.GroupBy) > 0 {
		return intent.GroupBy[0]
	}
	return "all"
}

func aggregateRows(ds dataset.Dataset, intent planner.ChartIntent, categoryColumn string) []point {
	metric, _ := dataset.FindMetric(ds, intent.Metrics[0])
	values := map[string]*aggState{}

	for _, row := range ds.Rows {
		if !matchesFilters(row, intent.Filters) {
			continue
		}
		category := "All"
		if categoryColumn != "" && categoryColumn != "all" {
			category = row[categoryColumn]
			if category == "" {
				category = "(empty)"
			}
		}
		if values[category] == nil {
			values[category] = newAggState(metric)
		}
		values[category].Add(row, metric)
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	points := make([]point, 0, len(keys))
	for _, key := range keys {
		points = append(points, point{Category: key, Value: values[key].Value()})
	}
	return points
}

type aggState struct {
	aggregate string
	sum       float64
	count     float64
	min       float64
	max       float64
}

func newAggState(metric dataset.Metric) *aggState {
	return &aggState{aggregate: strings.ToUpper(metric.Aggregate), min: math.Inf(1), max: math.Inf(-1)}
}

func (s *aggState) Add(row map[string]string, metric dataset.Metric) {
	if metric.Name == "count_rows" || strings.ToUpper(metric.Aggregate) == "COUNT" {
		s.count++
		return
	}
	raw := strings.ReplaceAll(row[metric.Column], ",", "")
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return
	}
	s.sum += value
	s.count++
	if value < s.min {
		s.min = value
	}
	if value > s.max {
		s.max = value
	}
}

func (s *aggState) Value() float64 {
	switch s.aggregate {
	case "COUNT":
		return s.count
	case "AVG":
		if s.count == 0 {
			return 0
		}
		return s.sum / s.count
	case "MIN":
		if math.IsInf(s.min, 1) {
			return 0
		}
		return s.min
	case "MAX":
		if math.IsInf(s.max, -1) {
			return 0
		}
		return s.max
	default:
		return s.sum
	}
}

func matchesFilters(row map[string]string, filters []planner.Filter) bool {
	for _, filter := range filters {
		value := row[filter.Column]
		switch strings.ToUpper(filter.Operator) {
		case "", "=", "==", "EQ":
			if value != filter.Value {
				return false
			}
		case "!=", "<>", "NEQ":
			if value == filter.Value {
				return false
			}
		case "CONTAINS":
			if !strings.Contains(strings.ToLower(value), strings.ToLower(filter.Value)) {
				return false
			}
		}
	}
	return true
}
