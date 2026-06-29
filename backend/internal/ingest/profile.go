package ingest

type ParserProfile struct {
	CustomerID      string   `json:"customer_id"`
	PreferredSheet  string   `json:"preferred_sheet,omitempty"`
	HeaderRow       int      `json:"header_row,omitempty"`
	RequiredColumns []string `json:"required_columns,omitempty"`
}

func (p ParserProfile) normalizedHeaderRow() int {
	if p.HeaderRow <= 0 {
		return 1
	}
	return p.HeaderRow
}
