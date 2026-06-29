package query

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

type Request struct {
	DatasetID string           `json:"dataset_id"`
	Metrics   []string         `json:"metrics"`
	GroupBy   []string         `json:"group_by,omitempty"`
	Filters   []planner.Filter `json:"filters,omitempty"`
	Limit     int              `json:"limit,omitempty"`
}

type Result struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

func Run(ds dataset.Dataset, req Request) (Result, error) {
	if req.DatasetID != "" && req.DatasetID != ds.ID {
		return Result{}, fmt.Errorf("query dataset %q does not match dataset %q", req.DatasetID, ds.ID)
	}
	if len(req.Metrics) == 0 {
		req.Metrics = []string{"count_rows"}
	}
	if req.Limit <= 0 {
		req.Limit = 100
	}
	if req.Limit > 1000 {
		req.Limit = 1000
	}
	if err := validate(ds, req); err != nil {
		return Result{}, err
	}
	if len(req.GroupBy) == 0 {
		return rawRows(ds, req), nil
	}
	return aggregateRows(ds, req), nil
}

func validate(ds dataset.Dataset, req Request) error {
	if len(req.Metrics) == 0 {
		return errors.New("at least one metric is required")
	}
	for _, metric := range req.Metrics {
		if _, ok := dataset.FindMetric(ds, metric); !ok {
			return fmt.Errorf("metric %q does not exist", metric)
		}
	}
	for _, group := range req.GroupBy {
		col, ok := dataset.FindColumn(ds, group)
		if !ok {
			return fmt.Errorf("group by column %q does not exist", group)
		}
		if col.Role == dataset.RoleMeasure {
			return fmt.Errorf("group by column %q is a measure", group)
		}
	}
	for _, filter := range req.Filters {
		if _, ok := dataset.FindColumn(ds, filter.Column); !ok {
			return fmt.Errorf("filter column %q does not exist", filter.Column)
		}
	}
	return nil
}

func rawRows(ds dataset.Dataset, req Request) Result {
	columns := make([]string, 0, len(ds.Columns))
	for _, col := range ds.Columns {
		columns = append(columns, col.Name)
	}
	rows := make([]map[string]any, 0, min(req.Limit, len(ds.Rows)))
	for _, row := range ds.Rows {
		if !matchesFilters(row, req.Filters) {
			continue
		}
		out := map[string]any{}
		for _, col := range columns {
			out[col] = row[col]
		}
		rows = append(rows, out)
		if len(rows) >= req.Limit {
			break
		}
	}
	return Result{Columns: columns, Rows: rows}
}

func aggregateRows(ds dataset.Dataset, req Request) Result {
	type bucket struct {
		keys    []string
		metrics map[string]*aggState
	}

	buckets := map[string]*bucket{}
	for _, row := range ds.Rows {
		if !matchesFilters(row, req.Filters) {
			continue
		}
		keyParts := make([]string, 0, len(req.GroupBy))
		for _, group := range req.GroupBy {
			keyParts = append(keyParts, row[group])
		}
		key := strings.Join(keyParts, "\x00")
		if _, ok := buckets[key]; !ok {
			buckets[key] = &bucket{keys: keyParts, metrics: map[string]*aggState{}}
		}
		for _, metricName := range req.Metrics {
			metric, _ := dataset.FindMetric(ds, metricName)
			if buckets[key].metrics[metricName] == nil {
				buckets[key].metrics[metricName] = newAggState(metric)
			}
			buckets[key].metrics[metricName].Add(row, metric)
		}
	}

	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	columns := append([]string{}, req.GroupBy...)
	columns = append(columns, req.Metrics...)
	rows := make([]map[string]any, 0, min(req.Limit, len(keys)))
	for _, key := range keys {
		bucket := buckets[key]
		out := map[string]any{}
		for i, group := range req.GroupBy {
			out[group] = bucket.keys[i]
		}
		for _, metric := range req.Metrics {
			out[metric] = bucket.metrics[metric].Value()
		}
		rows = append(rows, out)
		if len(rows) >= req.Limit {
			break
		}
	}
	return Result{Columns: columns, Rows: rows}
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
		default:
			return false
		}
	}
	return true
}
