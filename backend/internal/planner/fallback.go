package planner

import "jodata/internal/dataset"

type FallbackPlanner struct {
	Primary  Planner
	Fallback Planner
}

func (p FallbackPlanner) Plan(prompt string, ds dataset.Dataset) (ChartIntent, error) {
	if p.Primary != nil {
		intent, err := p.Primary.Plan(prompt, ds)
		if err == nil {
			return intent, nil
		}
	}
	return p.Fallback.Plan(prompt, ds)
}
