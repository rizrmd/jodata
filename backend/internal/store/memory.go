package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"jodata/internal/chart"
	"jodata/internal/dashboard"
	"jodata/internal/dataset"
	"jodata/internal/ingest"
)

type MemoryStore struct {
	mu         sync.RWMutex
	next       int
	dataFile   string
	datasets   map[string]dataset.Dataset
	charts     map[string]chart.Config
	dashboards map[string]dashboard.Dashboard
	profiles   map[string]ingest.ParserProfile
}

type Snapshot struct {
	Next       int                             `json:"next"`
	Datasets   map[string]dataset.Dataset      `json:"datasets"`
	Charts     map[string]chart.Config         `json:"charts"`
	Dashboards map[string]dashboard.Dashboard  `json:"dashboards"`
	Profiles   map[string]ingest.ParserProfile `json:"profiles"`
}

type ExportBundle struct {
	Version    string    `json:"version"`
	ExportedAt time.Time `json:"exported_at"`
	Data       Snapshot  `json:"data"`
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		next:       1,
		datasets:   map[string]dataset.Dataset{},
		charts:     map[string]chart.Config{},
		dashboards: map[string]dashboard.Dashboard{},
		profiles:   map[string]ingest.ParserProfile{},
	}
}

func NewFileStore(path string) (*MemoryStore, error) {
	store := NewMemoryStore()
	store.dataFile = path
	if path == "" {
		return store, nil
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *MemoryStore) ExportBundle() ExportBundle {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ExportBundle{
		Version:    "1.0",
		ExportedAt: time.Now().UTC(),
		Data:       s.snapshotLocked(),
	}
}

func (s *MemoryStore) ImportBundle(bundle ExportBundle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next = bundle.Data.Next
	if s.next <= 0 {
		s.next = 1
	}

	s.datasets = copyDatasets(bundle.Data.Datasets)
	s.charts = copyCharts(bundle.Data.Charts)
	s.dashboards = copyDashboards(bundle.Data.Dashboards)
	s.profiles = copyProfiles(bundle.Data.Profiles)
	s.persistLocked()
}

func (s *MemoryStore) NextID(prefix string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("%s_%d", prefix, s.next)
	s.next++
	return id
}

func (s *MemoryStore) SaveDataset(ds dataset.Dataset) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.datasets[ds.ID] = ds
	s.persistLocked()
}

func (s *MemoryStore) Dataset(id string) (dataset.Dataset, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ds, ok := s.datasets[id]
	return ds, ok
}

func (s *MemoryStore) Datasets() []dataset.Dataset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]dataset.Dataset, 0, len(s.datasets))
	for _, ds := range s.datasets {
		out = append(out, ds)
	}
	return out
}

func (s *MemoryStore) SaveChart(cfg chart.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.charts[cfg.ID] = cfg
	s.persistLocked()
}

func (s *MemoryStore) UpdateChart(cfg chart.Config) {
	s.SaveChart(cfg)
}

func (s *MemoryStore) Chart(id string) (chart.Config, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.charts[id]
	return cfg, ok
}

func (s *MemoryStore) Charts() []chart.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]chart.Config, 0, len(s.charts))
	for _, cfg := range s.charts {
		out = append(out, cfg)
	}
	return out
}

func (s *MemoryStore) SaveDashboard(dash dashboard.Dashboard) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dashboards[dash.ID] = dash
	s.persistLocked()
}

func (s *MemoryStore) UpdateDashboard(dash dashboard.Dashboard) {
	s.SaveDashboard(dash)
}

func (s *MemoryStore) Dashboard(id string) (dashboard.Dashboard, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dash, ok := s.dashboards[id]
	return dash, ok
}

func (s *MemoryStore) Dashboards() []dashboard.Dashboard {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]dashboard.Dashboard, 0, len(s.dashboards))
	for _, dash := range s.dashboards {
		out = append(out, dash)
	}
	return out
}

func (s *MemoryStore) SaveProfile(profile ingest.ParserProfile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles[profile.CustomerID] = profile
	s.persistLocked()
}

func (s *MemoryStore) Profile(customerID string) (ingest.ParserProfile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	profile, ok := s.profiles[customerID]
	return profile, ok
}

func (s *MemoryStore) Profiles() []ingest.ParserProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ingest.ParserProfile, 0, len(s.profiles))
	for _, profile := range s.profiles {
		out = append(out, profile)
	}
	return out
}

type snapshot struct {
	Next       int                             `json:"next"`
	Datasets   map[string]dataset.Dataset      `json:"datasets"`
	Charts     map[string]chart.Config         `json:"charts"`
	Dashboards map[string]dashboard.Dashboard  `json:"dashboards"`
	Profiles   map[string]ingest.ParserProfile `json:"profiles"`
}

func (s *MemoryStore) load() error {
	content, err := os.ReadFile(s.dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var snap snapshot
	if err := json.Unmarshal(content, &snap); err != nil {
		return err
	}
	if snap.Next > 0 {
		s.next = snap.Next
	}
	if snap.Datasets != nil {
		s.datasets = snap.Datasets
	}
	if snap.Charts != nil {
		s.charts = snap.Charts
	}
	if snap.Dashboards != nil {
		s.dashboards = snap.Dashboards
	}
	if snap.Profiles != nil {
		s.profiles = snap.Profiles
	}
	return nil
}

func (s *MemoryStore) persistLocked() {
	if s.dataFile == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.dataFile), 0o755); err != nil {
		return
	}
	snap := snapshot{
		Next:       s.next,
		Datasets:   s.datasets,
		Charts:     s.charts,
		Dashboards: s.dashboards,
		Profiles:   s.profiles,
	}
	content, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.dataFile, content, 0o600)
}

func (s *MemoryStore) snapshotLocked() Snapshot {
	return Snapshot{
		Next:       s.next,
		Datasets:   copyDatasets(s.datasets),
		Charts:     copyCharts(s.charts),
		Dashboards: copyDashboards(s.dashboards),
		Profiles:   copyProfiles(s.profiles),
	}
}

func copyDatasets(source map[string]dataset.Dataset) map[string]dataset.Dataset {
	if source == nil {
		return map[string]dataset.Dataset{}
	}
	datasets := make(map[string]dataset.Dataset, len(source))
	for key, value := range source {
		datasets[key] = value
	}
	return datasets
}

func copyCharts(source map[string]chart.Config) map[string]chart.Config {
	if source == nil {
		return map[string]chart.Config{}
	}
	charts := make(map[string]chart.Config, len(source))
	for key, value := range source {
		charts[key] = value
	}
	return charts
}

func copyDashboards(source map[string]dashboard.Dashboard) map[string]dashboard.Dashboard {
	if source == nil {
		return map[string]dashboard.Dashboard{}
	}
	dashboards := make(map[string]dashboard.Dashboard, len(source))
	for key, value := range source {
		dashboards[key] = value
	}
	return dashboards
}

func copyProfiles(source map[string]ingest.ParserProfile) map[string]ingest.ParserProfile {
	if source == nil {
		return map[string]ingest.ParserProfile{}
	}
	profiles := make(map[string]ingest.ParserProfile, len(source))
	for key, value := range source {
		profiles[key] = value
	}
	return profiles
}
