package composer

import (
	"fmt"
	"strings"

	"jodata/internal/dataset"
)

func SuggestedPrompts(ds dataset.Dataset, max int) []string {
	if max <= 0 {
		max = 4
	}
	metric := firstMetric(ds)
	dimension := firstRole(ds, dataset.RoleDimension)
	temporal := firstRole(ds, dataset.RoleTemporal)

	prompts := []string{}
	if metric != "" && dimension != "" {
		prompts = append(prompts, fmt.Sprintf("%s by %s as a bar chart", human(metric), human(dimension)))
	}
	if metric != "" && temporal != "" {
		prompts = append(prompts, fmt.Sprintf("trend of %s by %s as a line chart", human(metric), human(temporal)))
	}
	if metric != "" && dimension != "" {
		prompts = append(prompts, fmt.Sprintf("%s share by %s as a pie chart", human(metric), human(dimension)))
	}
	prompts = append(prompts, "table summary")

	if len(prompts) > max {
		return prompts[:max]
	}
	return prompts
}

func firstMetric(ds dataset.Dataset) string {
	for _, metric := range ds.Metrics {
		if metric.Name != "count_rows" {
			return metric.Name
		}
	}
	if len(ds.Metrics) > 0 {
		return ds.Metrics[0].Name
	}
	return ""
}

func firstRole(ds dataset.Dataset, role dataset.ColumnRole) string {
	for _, col := range ds.Columns {
		if col.Role == role {
			return col.Name
		}
	}
	return ""
}

func human(value string) string {
	value = strings.TrimPrefix(value, "sum_")
	return strings.ReplaceAll(value, "_", " ")
}
