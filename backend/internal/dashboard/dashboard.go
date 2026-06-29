package dashboard

import (
	"fmt"
	"strings"
	"time"
)

type Item struct {
	ChartID string `json:"chart_id"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	W       int    `json:"w"`
	H       int    `json:"h"`
}

type Filter struct {
	Expression string `json:"expression"`
	AddedAt    string `json:"added_at"`
}

type Dashboard struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	ChartIDs []string `json:"chart_ids"`
	Layout   []Item   `json:"layout"`
	Filters  []Filter `json:"filters"`
}

func New(id string, title string, chartIDs []string) Dashboard {
	if title == "" {
		title = "Untitled Dashboard"
	}
	layout := make([]Item, 0, len(chartIDs))
	for i, chartID := range chartIDs {
		layout = append(layout, Item{
			ChartID: chartID,
			X:       (i % 2) * 6,
			Y:       (i / 2) * 4,
			W:       6,
			H:       4,
		})
	}
	return Dashboard{ID: id, Title: title, ChartIDs: chartIDs, Layout: layout}
}

func ValidateChartIDs(chartIDs []string, exists func(string) bool) error {
	for _, chartID := range chartIDs {
		if !exists(chartID) {
			return fmt.Errorf("chart %q does not exist", chartID)
		}
	}
	return nil
}

func AddChart(dash Dashboard, chartID string) Dashboard {
	for _, existing := range dash.ChartIDs {
		if existing == chartID {
			return dash
		}
	}
	dash.ChartIDs = append(dash.ChartIDs, chartID)
	index := len(dash.Layout)
	dash.Layout = append(dash.Layout, Item{
		ChartID: chartID,
		X:       (index % 2) * 6,
		Y:       (index / 2) * 4,
		W:       6,
		H:       4,
	})
	return dash
}

func AddFilter(dash Dashboard, expression string) Dashboard {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return dash
	}
	for _, existing := range dash.Filters {
		if existing.Expression == expression {
			return dash
		}
	}
	dash.Filters = append(dash.Filters, Filter{
		Expression: expression,
		AddedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	return dash
}
