package planner

type Filter struct {
	Column   string `json:"column"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type ChartIntent struct {
	DatasetID  string   `json:"dataset_id"`
	ChartType  string   `json:"chart_type"`
	Title      string   `json:"title"`
	Metrics    []string `json:"metrics"`
	GroupBy    []string `json:"group_by"`
	TimeColumn string   `json:"time_column,omitempty"`
	TimeGrain  string   `json:"time_grain,omitempty"`
	Filters    []Filter `json:"filters,omitempty"`
}
