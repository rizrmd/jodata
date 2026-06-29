package httpapi

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"jodata/internal/chart"
	"jodata/internal/composer"
	"jodata/internal/dashboard"
	"jodata/internal/dataset"
	"jodata/internal/ingest"
	"jodata/internal/planner"
	"jodata/internal/query"
	"jodata/internal/security"
	"jodata/internal/store"
)

//go:embed index.html
var indexHTML []byte
const defaultFrontendIndexPath = "../frontend/index.html"
const defaultFrontendBuildDir = "../frontend/dist"
const defaultMaxRequestBytes = 32 * 1024 * 1024

type Server struct {
	store    *store.MemoryStore
	parser   ingest.Parser
	planner  planner.Planner
	security security.Manager
}

type createChartRequest struct {
	DatasetID string `json:"dataset_id"`
	Prompt    string `json:"prompt"`
}

type createMetricRequest struct {
	Name      string `json:"name"`
	Label     string `json:"label,omitempty"`
	Column    string `json:"column,omitempty"`
	Aggregate string `json:"aggregate"`
}

type updateChartRequest struct {
	Prompt string `json:"prompt"`
}

type autoDashboardRequest struct {
	Title     string   `json:"title"`
	Prompts   []string `json:"prompts,omitempty"`
	MaxCharts int      `json:"max_charts,omitempty"`
}

type autoChartsRequest struct {
	Prompts   []string `json:"prompts,omitempty"`
	MaxCharts int      `json:"max_charts,omitempty"`
}

type autoChartsResponse struct {
	DatasetID string         `json:"dataset_id"`
	ChartIDs  []string       `json:"chart_ids"`
	Charts    []chart.Config `json:"charts"`
}

type autoBuildResponse struct {
	DatasetID   string         `json:"dataset_id"`
	DashboardID string         `json:"dashboard_id"`
	Title       string         `json:"title"`
	ChartIDs    []string       `json:"chart_ids"`
	Charts      []chart.Config `json:"charts"`
}

type createSourceFromURLRequest struct {
	URL             string   `json:"url"`
	CustomerID      string   `json:"customer_id"`
	PreferredSheet  string   `json:"preferred_sheet,omitempty"`
	HeaderRow       int      `json:"header_row,omitempty"`
	RequiredColumns []string `json:"required_columns,omitempty"`
}

type createSourceFromAPIRequest struct {
	URL             string            `json:"url"`
	Method          string            `json:"method"`
	Headers         map[string]string `json:"headers,omitempty"`
	Query           map[string]string `json:"query,omitempty"`
	Body            any               `json:"body,omitempty"`
	CustomerID      string            `json:"customer_id"`
	PreferredSheet  string            `json:"preferred_sheet,omitempty"`
	HeaderRow       int               `json:"header_row,omitempty"`
	RequiredColumns []string          `json:"required_columns,omitempty"`
}

type importIntermediaryRequest struct {
	Name    string                `json:"name"`
	Source  dataset.Source        `json:"source"`
	Headers []string              `json:"headers"`
	Rows    [][]string            `json:"rows"`
	Metrics []dataset.MetricInput `json:"metrics,omitempty"`
}

type createDashboardRequest struct {
	Title    string   `json:"title"`
	ChartIDs []string `json:"chart_ids"`
}

type addChartToDashboardRequest struct {
	ChartID string `json:"chart_id"`
}

type dashboardDetail struct {
	ID       string           `json:"id"`
	Title    string           `json:"title"`
	ChartIDs []string         `json:"chart_ids"`
	Layout   []dashboard.Item `json:"layout"`
	Charts   []chart.Config   `json:"charts"`
}

func NewServer(store *store.MemoryStore) Server {
	return Server{
		store:    store,
		parser:   ingest.NewParser(func() string { return store.NextID("dataset") }),
		planner:  plannerFromEnv(),
		security: security.NewManager(os.Getenv("JODATA_API_KEYS")),
	}
}

func plannerFromEnv() planner.Planner {
	fallback := planner.NewHeuristicPlanner()
	if os.Getenv("JODATA_AI_PROVIDER") != "llm" {
		return fallback
	}
	llm, err := planner.NewLLMPlanner(planner.LLMConfig{
		BaseURL: os.Getenv("JODATA_LLM_BASE_URL"),
		APIKey:  os.Getenv("JODATA_LLM_API_KEY"),
		Model:   os.Getenv("JODATA_LLM_MODEL"),
		Timeout: 20 * time.Second,
	})
	if err != nil {
		return fallback
	}
	return planner.FallbackPlanner{Primary: llm, Fallback: fallback}
}

func (s Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/sources", s.createSource)
	mux.HandleFunc("POST /api/sources/url", s.createSourceFromURL)
	mux.HandleFunc("POST /api/sources/api", s.createSourceFromAPI)
	mux.HandleFunc("POST /api/datasets/intermediary", s.importIntermediaryDataset)
	mux.HandleFunc("GET /api/export", s.exportBundle)
	mux.HandleFunc("POST /api/import", s.importBundle)
	mux.HandleFunc("GET /api/datasets", s.listDatasets)
	mux.HandleFunc("POST /api/datasets/{id}/metrics", s.createMetric)
	mux.HandleFunc("POST /api/datasets/{id}/query", s.queryDataset)
	mux.HandleFunc("POST /api/datasets/{id}/auto-charts", s.autoCharts)
	mux.HandleFunc("POST /api/datasets/{id}/auto-dashboard", s.autoDashboard)
	mux.HandleFunc("POST /api/datasets/{id}/auto-build", s.autoBuild)
	mux.HandleFunc("POST /api/charts", s.createChart)
	mux.HandleFunc("GET /api/charts", s.listCharts)
	mux.HandleFunc("PUT /api/charts/{id}", s.updateChart)
	mux.HandleFunc("POST /api/dashboards", s.createDashboard)
	mux.HandleFunc("GET /api/dashboards", s.listDashboards)
	mux.HandleFunc("GET /api/dashboards/{id}", s.getDashboard)
	mux.HandleFunc("PUT /api/dashboards/{id}/charts", s.addChartToDashboard)
	mux.HandleFunc("POST /api/parser-profiles", s.saveParserProfile)
	mux.HandleFunc("GET /api/parser-profiles", s.listParserProfiles)
	mux.HandleFunc("/", s.spa)
	return s.wrapHandler(mux)
}

func (s Server) wrapHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		allowedOrigin := allowedCORSOrigin(origin)
		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "authorization, content-type, x-api-key")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
		}

		if r.Method == http.MethodOptions {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.NotFound(w, r)
			return
		}

		if isAPIPath(r.URL.Path) && r.ContentLength > 0 {
			limit := resolveMaxRequestBytes()
			if r.ContentLength > limit {
				writeError(w, http.StatusRequestEntityTooLarge, "request body exceeds configured limit")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}

		next.ServeHTTP(w, r)
	})
}

func isAPIPath(pathValue string) bool {
	return pathValue == "/api" || strings.HasPrefix(pathValue, "/api/")
}

func resolveMaxRequestBytes() int64 {
	if raw := strings.TrimSpace(os.Getenv("JODATA_MAX_REQUEST_BYTES")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return int64(defaultMaxRequestBytes)
}

func allowedCORSOrigin(requestOrigin string) string {
	raw := strings.TrimSpace(os.Getenv("JODATA_CORS_ORIGINS"))
	if raw == "" {
		return "*"
	}
	if raw == "*" {
		return "*"
	}
	if requestOrigin == "" {
		return ""
	}
	for _, allowed := range strings.Split(raw, ",") {
		if strings.TrimSpace(allowed) == requestOrigin {
			return requestOrigin
		}
	}
	return ""
}

func (s Server) spa(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}

	buildDir := resolveFrontendBuildDir()
	cleanPath := path.Clean(r.URL.Path)
	if cleanPath != "/" && buildDir != "" {
		requested := strings.TrimPrefix(cleanPath, "/")
		localPath := filepath.Join(buildDir, filepath.FromSlash(requested))
		if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, localPath)
			return
		}
	}

	payload := indexHTML
	loaded := false
	if override := strings.TrimSpace(os.Getenv("JODATA_FRONTEND_INDEX")); override != "" {
		if data, err := s.readFileIfExists(override); err == nil {
			payload = data
			loaded = true
		}
	}

	if !loaded && buildDir != "" {
		builtIndex := filepath.Join(buildDir, "index.html")
		if data, err := s.readFileIfExists(builtIndex); err == nil {
			payload = data
			loaded = true
		}
	}
	if !loaded {
		if data, err := s.readFileIfExists(defaultFrontendIndexPath); err == nil {
			payload = data
			loaded = true
		}
	}
	if len(payload) == 0 {
		payload = indexHTML
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func resolveFrontendBuildDir() string {
	if override := strings.TrimSpace(os.Getenv("JODATA_FRONTEND_BUILD_DIR")); override != "" {
		return override
	}
	return defaultFrontendBuildDir
}

func (s Server) readFileIfExists(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, err
	}
	return data, nil
}

func (s Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s Server) createSource(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermWrite) {
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "multipart file field is required")
		return
	}
	defer file.Close()

	content, err := ingest.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	profile := s.profileFromRequest(r)
	ds, err := s.parser.Parse(header.Filename, content, profile)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.store.SaveDataset(ds)
	writeJSON(w, http.StatusCreated, ds)
}

func (s Server) createSourceFromURL(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermWrite) {
		return
	}
	var req createSourceFromURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	requestedURL := strings.TrimSpace(req.URL)
	if requestedURL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	parsed, err := url.Parse(requestedURL)
	if err != nil || !parsed.IsAbs() || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		writeError(w, http.StatusBadRequest, "url must be absolute http/https")
		return
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(requestedURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to fetch source: status %s", resp.Status))
		return
	}

	const maxSourceBytes = 8 * 1024 * 1024
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxSourceBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(content) > maxSourceBytes {
		writeError(w, http.StatusBadRequest, "fetched source exceeds 8MB limit")
		return
	}

	profile := s.profileFromValues(req.CustomerID, req.PreferredSheet, req.HeaderRow, req.RequiredColumns)
	filename := detectSourceFilename(parsed, resp.Header.Get("content-type"))
	ds, err := s.parser.Parse(filename, content, profile)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.store.SaveDataset(ds)
	writeJSON(w, http.StatusCreated, ds)
}

func (s Server) createSourceFromAPI(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermWrite) {
		return
	}
	var req createSourceFromAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	requestedURL := strings.TrimSpace(req.URL)
	if requestedURL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	parsed, err := url.Parse(requestedURL)
	if err != nil || !parsed.IsAbs() || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		writeError(w, http.StatusBadRequest, "url must be absolute http/https")
		return
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		writeError(w, http.StatusBadRequest, "method must be GET, POST, PUT, or PATCH")
		return
	}

	query := parsed.Query()
	for key, value := range req.Query {
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()

	var bodyReader io.Reader
	if req.Body != nil && (method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch) {
		payload, err := json.Marshal(req.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		bodyReader = bytes.NewReader(payload)
	}

	httpReq, err := http.NewRequest(method, parsed.String(), bodyReader)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}
	if bodyReader != nil && httpReq.Header.Get("content-type") == "" {
		httpReq.Header.Set("content-type", "application/json")
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to fetch source: status %s", resp.Status))
		return
	}

	const maxSourceBytes = 8 * 1024 * 1024
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxSourceBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(content) > maxSourceBytes {
		writeError(w, http.StatusBadRequest, "fetched source exceeds 8MB limit")
		return
	}

	profile := s.profileFromValues(req.CustomerID, req.PreferredSheet, req.HeaderRow, req.RequiredColumns)
	filename := detectSourceFilename(parsed, resp.Header.Get("content-type"))
	ds, err := s.parser.Parse(filename, content, profile)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.store.SaveDataset(ds)
	writeJSON(w, http.StatusCreated, ds)
}

func (s Server) importIntermediaryDataset(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermWrite) {
		return
	}
	var req importIntermediaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Headers) == 0 {
		writeError(w, http.StatusBadRequest, "headers are required")
		return
	}
	if len(req.Rows) == 0 {
		writeError(w, http.StatusBadRequest, "rows are required")
		return
	}
	source := req.Source
	if source.Name == "" {
		source.Name = name
	}
	if source.Type == "" {
		source.Type = dataset.SourceIntermediary
	} else if source.Type != dataset.SourceIntermediary && source.Type != dataset.SourceCSV && source.Type != dataset.SourceJSON && source.Type != dataset.SourceXLSX {
		writeError(w, http.StatusBadRequest, "unsupported source type")
		return
	}
	headers, rows := normalizeIntermediaryPayload(req.Headers, req.Rows)

	ds := dataset.Build(s.store.NextID("dataset"), name, source, headers, rows)
	for _, input := range req.Metrics {
		updated, _, err := dataset.AddMetric(ds, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		ds = updated
	}
	s.store.SaveDataset(ds)
	writeJSON(w, http.StatusCreated, ds)
}

func (s Server) exportBundle(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermAdmin) {
		return
	}
	writeJSON(w, http.StatusOK, s.store.ExportBundle())
}

func (s Server) importBundle(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermAdmin) {
		return
	}
	var bundle store.ExportBundle
	if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.store.ImportBundle(bundle)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s Server) listDatasets(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermRead) {
		return
	}
	writeJSON(w, http.StatusOK, s.store.Datasets())
}

func (s Server) createMetric(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermWrite) {
		return
	}
	ds, ok := s.store.Dataset(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}
	var req createMetricRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, metric, err := dataset.AddMetric(ds, dataset.MetricInput{
		Name:      req.Name,
		Label:     req.Label,
		Column:    req.Column,
		Aggregate: req.Aggregate,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.store.SaveDataset(updated)
	writeJSON(w, http.StatusCreated, metric)
}

func (s Server) queryDataset(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermRead) {
		return
	}
	id := r.PathValue("id")
	ds, ok := s.store.Dataset(id)
	if !ok {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}
	var req query.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.DatasetID = id
	result, err := query.Run(ds, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s Server) autoDashboard(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermWrite) {
		return
	}
	ds, ok := s.store.Dataset(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}
	var req autoDashboardRequest
	if err := decodeOrEmptyBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	charts, err := s.generateCharts(ds, req.Prompts, req.MaxCharts)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	chartIDs := make([]string, 0, len(charts))
	for _, cfg := range charts {
		s.store.SaveChart(cfg)
		chartIDs = append(chartIDs, cfg.ID)
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = ds.Name + " Dashboard"
	}
	dash := dashboard.New(s.store.NextID("dashboard"), title, chartIDs)
	s.store.SaveDashboard(dash)
	s.writeDashboardDetail(w, http.StatusCreated, dash)
}

func (s Server) autoBuild(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermWrite) {
		return
	}
	ds, ok := s.store.Dataset(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}
	var req autoDashboardRequest
	if err := decodeOrEmptyBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	charts, err := s.generateCharts(ds, req.Prompts, req.MaxCharts)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	chartIDs := make([]string, 0, len(charts))
	for _, cfg := range charts {
		s.store.SaveChart(cfg)
		chartIDs = append(chartIDs, cfg.ID)
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = ds.Name + " Auto Dashboard"
	}
	dash := dashboard.New(s.store.NextID("dashboard"), title, chartIDs)
	s.store.SaveDashboard(dash)

	writeJSON(w, http.StatusCreated, autoBuildResponse{
		DatasetID:   ds.ID,
		DashboardID: dash.ID,
		Title:       dash.Title,
		ChartIDs:    chartIDs,
		Charts:      charts,
	})
}

func (s Server) autoCharts(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermWrite) {
		return
	}
	ds, ok := s.store.Dataset(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}
	var req autoChartsRequest
	if err := decodeOrEmptyBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	charts, err := s.generateCharts(ds, req.Prompts, req.MaxCharts)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ids := make([]string, 0, len(charts))
	for _, cfg := range charts {
		s.store.SaveChart(cfg)
		ids = append(ids, cfg.ID)
	}
	writeJSON(w, http.StatusCreated, autoChartsResponse{
		DatasetID: ds.ID,
		ChartIDs:  ids,
		Charts:    charts,
	})
}

func (s Server) createChart(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermWrite) {
		return
	}
	var req createChartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ds, ok := s.store.Dataset(req.DatasetID)
	if !ok {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}
	intent, err := s.planner.Plan(req.Prompt, ds)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	intent.DatasetID = req.DatasetID
	cfg, err := chart.Build(s.store.NextID("chart"), ds, intent)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.store.SaveChart(cfg)
	writeJSON(w, http.StatusCreated, cfg)
}

func (s Server) listCharts(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermRead) {
		return
	}
	writeJSON(w, http.StatusOK, s.store.Charts())
}

func (s Server) updateChart(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermWrite) {
		return
	}
	existing, ok := s.store.Chart(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "chart not found")
		return
	}
	ds, ok := s.store.Dataset(existing.DatasetID)
	if !ok {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}
	var req updateChartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	intent, err := s.planner.Plan(req.Prompt, ds)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	intent.DatasetID = ds.ID
	cfg, err := chart.Build(existing.ID, ds, intent)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.store.UpdateChart(cfg)
	writeJSON(w, http.StatusOK, cfg)
}

func (s Server) createDashboard(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermWrite) {
		return
	}
	var req createDashboardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.ChartIDs) == 0 {
		writeError(w, http.StatusBadRequest, "at least one chart id is required")
		return
	}
	err := dashboard.ValidateChartIDs(req.ChartIDs, func(id string) bool {
		_, ok := s.store.Chart(id)
		return ok
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	dash := dashboard.New(s.store.NextID("dashboard"), req.Title, req.ChartIDs)
	s.store.SaveDashboard(dash)
	writeJSON(w, http.StatusCreated, dash)
}

func (s Server) listDashboards(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermRead) {
		return
	}
	writeJSON(w, http.StatusOK, s.store.Dashboards())
}

func (s Server) getDashboard(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermRead) {
		return
	}
	dash, ok := s.store.Dashboard(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}
	s.writeDashboardDetail(w, http.StatusOK, dash)
}

func (s Server) generateCharts(ds dataset.Dataset, prompts []string, maxCharts int) ([]chart.Config, error) {
	if len(prompts) == 0 {
		prompts = composer.SuggestedPrompts(ds, maxCharts)
	}
	if len(prompts) == 0 {
		return nil, fmt.Errorf("no chart prompts could be generated")
	}
	charts := make([]chart.Config, 0, len(prompts))
	for _, prompt := range prompts {
		intent, err := s.planner.Plan(prompt, ds)
		if err != nil {
			return nil, err
		}
		intent.DatasetID = ds.ID
		cfg, err := chart.Build(s.store.NextID("chart"), ds, intent)
		if err != nil {
			return nil, err
		}
		charts = append(charts, cfg)
	}
	return charts, nil
}

func decodeOrEmptyBody(r *http.Request, out any) error {
	if r.Body == nil {
		return nil
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

func (s Server) writeDashboardDetail(w http.ResponseWriter, status int, dash dashboard.Dashboard) {
	charts := make([]chart.Config, 0, len(dash.ChartIDs))
	for _, chartID := range dash.ChartIDs {
		cfg, ok := s.store.Chart(chartID)
		if ok {
			charts = append(charts, cfg)
		}
	}
	writeJSON(w, status, dashboardDetail{
		ID:       dash.ID,
		Title:    dash.Title,
		ChartIDs: dash.ChartIDs,
		Layout:   dash.Layout,
		Charts:   charts,
	})
}

func (s Server) addChartToDashboard(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermWrite) {
		return
	}
	dash, ok := s.store.Dashboard(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}
	var req addChartToDashboardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ChartID == "" {
		writeError(w, http.StatusBadRequest, "chart_id is required")
		return
	}
	if _, ok := s.store.Chart(req.ChartID); !ok {
		writeError(w, http.StatusBadRequest, "chart not found")
		return
	}
	dash = dashboard.AddChart(dash, req.ChartID)
	s.store.UpdateDashboard(dash)
	writeJSON(w, http.StatusOK, dash)
}

func (s Server) saveParserProfile(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermWrite) {
		return
	}
	var profile ingest.ParserProfile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	profile.CustomerID = strings.TrimSpace(profile.CustomerID)
	if profile.CustomerID == "" {
		writeError(w, http.StatusBadRequest, "customer_id is required")
		return
	}
	s.store.SaveProfile(profile)
	writeJSON(w, http.StatusCreated, profile)
}

func (s Server) listParserProfiles(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, security.PermRead) {
		return
	}
	writeJSON(w, http.StatusOK, s.store.Profiles())
}

func (s Server) profileFromRequest(r *http.Request) ingest.ParserProfile {
	customerID := strings.TrimSpace(r.FormValue("customer_id"))
	profile := ingest.ParserProfile{CustomerID: customerID}
	if customerID != "" {
		if saved, ok := s.store.Profile(customerID); ok {
			profile = saved
		}
	}
	if sheet := strings.TrimSpace(r.FormValue("sheet")); sheet != "" {
		profile.PreferredSheet = sheet
	}
	if headerRow := strings.TrimSpace(r.FormValue("header_row")); headerRow != "" {
		if parsed, err := strconv.Atoi(headerRow); err == nil {
			profile.HeaderRow = parsed
		}
	}
	if required := strings.TrimSpace(r.FormValue("required_columns")); required != "" {
		parts := strings.Split(required, ",")
		columns := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				columns = append(columns, part)
			}
		}
		profile.RequiredColumns = columns
	}
	return profile
}

func (s Server) profileFromValues(customerID, preferredSheet string, headerRow int, requiredColumns []string) ingest.ParserProfile {
	customerID = strings.TrimSpace(customerID)
	profile := ingest.ParserProfile{CustomerID: customerID}
	if customerID != "" {
		if saved, ok := s.store.Profile(customerID); ok {
			profile = saved
		}
	}
	if preferredSheet != "" {
		profile.PreferredSheet = preferredSheet
	}
	if headerRow > 0 {
		profile.HeaderRow = headerRow
	}
	if len(requiredColumns) > 0 {
		profile.RequiredColumns = requiredColumns
	}
	return profile
}

func detectSourceFilename(parsed *url.URL, contentType string) string {
	base := path.Base(parsed.Path)
	if base == "." || base == "/" || base == "" {
		base = "source"
	}
	ext := path.Ext(base)
	if ext != "" {
		return base
	}
	contentType = strings.ToLower(contentType)
	if strings.Contains(contentType, "csv") || strings.Contains(contentType, "text/plain") {
		return base + ".csv"
	}
	if strings.Contains(contentType, "json") {
		return base + ".json"
	}
	return base
}

func normalizeIntermediaryPayload(headers []string, rows [][]string) ([]string, [][]string) {
	cleanHeaders := make([]string, 0, len(headers))
	headerIndex := make([]int, 0, len(headers))
	for i, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		cleanHeaders = append(cleanHeaders, header)
		headerIndex = append(headerIndex, i)
	}
	if len(cleanHeaders) == 0 {
		return nil, nil
	}

	normalized := make([][]string, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		output := make([]string, len(cleanHeaders))
		useOriginalIndex := len(row) >= len(headers)
		if useOriginalIndex {
			for _, originalIndex := range headerIndex {
				if originalIndex >= len(row) {
					useOriginalIndex = false
					break
				}
			}
		}
		if useOriginalIndex {
			for i, originalIndex := range headerIndex {
				output[i] = strings.TrimSpace(row[originalIndex])
			}
		} else {
			for i := range output {
				if i < len(row) {
					output[i] = strings.TrimSpace(row[i])
				}
			}
		}
		normalized = append(normalized, output)
	}
	return cleanHeaders, normalized
}

func (s Server) authorize(w http.ResponseWriter, r *http.Request, permission security.Permission) bool {
	if r == nil {
		r = &http.Request{Header: http.Header{}}
	}
	if s.security.Allowed(r, permission) {
		return true
	}
	writeError(w, http.StatusUnauthorized, "unauthorized")
	return false
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
