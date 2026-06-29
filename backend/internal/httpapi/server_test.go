package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"jodata/internal/dataset"
	"jodata/internal/store"
)

func TestEndToEndCSVToChart(t *testing.T) {
	srv := NewServer(store.NewMemoryStore())
	handler := srv.Routes()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "sales.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "month,region,revenue\n2026-01-01,west,10\n2026-02-01,east,20\n"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	upload := httptest.NewRequest(http.MethodPost, "/api/sources", body)
	upload.Header.Set("content-type", writer.FormDataContentType())
	uploadRes := httptest.NewRecorder()
	handler.ServeHTTP(uploadRes, upload)
	if uploadRes.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body = %s", uploadRes.Code, uploadRes.Body.String())
	}

	var ds struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(uploadRes.Body).Decode(&ds); err != nil {
		t.Fatal(err)
	}

	payload := bytes.NewBufferString(`{"dataset_id":"` + ds.ID + `","prompt":"monthly revenue by region as a bar chart"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/charts", payload)
	req.Header.Set("content-type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("chart status = %d body = %s", res.Code, res.Body.String())
	}

	var chart struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&chart); err != nil {
		t.Fatal(err)
	}

	dashboardPayload := bytes.NewBufferString(`{"title":"Sales Dashboard","chart_ids":["` + chart.ID + `"]}`)
	dashboardReq := httptest.NewRequest(http.MethodPost, "/api/dashboards", dashboardPayload)
	dashboardReq.Header.Set("content-type", "application/json")
	dashboardRes := httptest.NewRecorder()
	handler.ServeHTTP(dashboardRes, dashboardReq)
	if dashboardRes.Code != http.StatusCreated {
		t.Fatalf("dashboard status = %d body = %s", dashboardRes.Code, dashboardRes.Body.String())
	}

	queryReq := httptest.NewRequest(http.MethodPost, "/api/datasets/"+ds.ID+"/query", bytes.NewBufferString(`{"metrics":["sum_revenue"],"group_by":["region"]}`))
	queryReq.Header.Set("content-type", "application/json")
	queryRes := httptest.NewRecorder()
	handler.ServeHTTP(queryRes, queryReq)
	if queryRes.Code != http.StatusOK {
		t.Fatalf("query status = %d body = %s", queryRes.Code, queryRes.Body.String())
	}
}

func TestExportAndImportBundle(t *testing.T) {
	mem := store.NewMemoryStore()
	srv := NewServer(mem)
	handler := srv.Routes()

	ds := datasetFixture(mem)

	chartReq := httptest.NewRequest(http.MethodPost, "/api/charts", bytes.NewBufferString(`{"dataset_id":"`+ds.ID+`","prompt":"revenue by region"}`))
	chartReq.Header.Set("content-type", "application/json")
	chartRes := httptest.NewRecorder()
	handler.ServeHTTP(chartRes, chartReq)
	if chartRes.Code != http.StatusCreated {
		t.Fatalf("chart status = %d body = %s", chartRes.Code, chartRes.Body.String())
	}

	var chart struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(chartRes.Body).Decode(&chart); err != nil {
		t.Fatal(err)
	}

	dashboardReq := httptest.NewRequest(http.MethodPost, "/api/dashboards", bytes.NewBufferString(`{"title":"Sales","chart_ids":["`+chart.ID+`"]}`))
	dashboardReq.Header.Set("content-type", "application/json")
	dashboardRes := httptest.NewRecorder()
	handler.ServeHTTP(dashboardRes, dashboardReq)
	if dashboardRes.Code != http.StatusCreated {
		t.Fatalf("dashboard status = %d body = %s", dashboardRes.Code, dashboardRes.Body.String())
	}

	profileReq := httptest.NewRequest(http.MethodPost, "/api/parser-profiles", bytes.NewBufferString(`{
		"customer_id":"acme",
		"header_row":2
	}`))
	profileReq.Header.Set("content-type", "application/json")
	profileRes := httptest.NewRecorder()
	handler.ServeHTTP(profileRes, profileReq)
	if profileRes.Code != http.StatusCreated {
		t.Fatalf("profile status = %d body = %s", profileRes.Code, profileRes.Body.String())
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/api/export", nil)
	exportRes := httptest.NewRecorder()
	handler.ServeHTTP(exportRes, exportReq)
	if exportRes.Code != http.StatusOK {
		t.Fatalf("export status = %d body = %s", exportRes.Code, exportRes.Body.String())
	}
	var bundle store.ExportBundle
	if err := json.NewDecoder(exportRes.Body).Decode(&bundle); err != nil {
		t.Fatal(err)
	}

	rawBundle, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}

	importSrv := NewServer(store.NewMemoryStore())
	importHandler := importSrv.Routes()
	importReq := httptest.NewRequest(http.MethodPost, "/api/import", bytes.NewBuffer(rawBundle))
	importReq.Header.Set("content-type", "application/json")
	importRes := httptest.NewRecorder()
	importHandler.ServeHTTP(importRes, importReq)
	if importRes.Code != http.StatusOK {
		t.Fatalf("import status = %d body = %s", importRes.Code, importRes.Body.String())
	}

	datasetsReq := httptest.NewRequest(http.MethodGet, "/api/datasets", nil)
	datasetsRes := httptest.NewRecorder()
	importHandler.ServeHTTP(datasetsRes, datasetsReq)
	if datasetsRes.Code != http.StatusOK {
		t.Fatalf("datasets status = %d body = %s", datasetsRes.Code, datasetsRes.Body.String())
	}
	var datasets []dataset.Dataset
	if err := json.NewDecoder(datasetsRes.Body).Decode(&datasets); err != nil {
		t.Fatal(err)
	}
	if len(datasets) != 1 {
		t.Fatalf("dataset count = %d", len(datasets))
	}
}

func TestIndexRoute(t *testing.T) {
	srv := NewServer(store.NewMemoryStore())
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.Routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("index status = %d", res.Code)
	}
	if res.Header().Get("content-type") != "text/html; charset=utf-8" {
		t.Fatalf("content type = %s", res.Header().Get("content-type"))
	}
}

func TestAuthRequiresConfiguredAPIKey(t *testing.T) {
	t.Setenv("JODATA_API_KEYS", "editor-token:editor,viewer-token:viewer")
	srv := NewServer(store.NewMemoryStore())

	unauthorized := httptest.NewRecorder()
	srv.Routes().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/datasets", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	authorizedReq := httptest.NewRequest(http.MethodGet, "/api/datasets", nil)
	authorizedReq.Header.Set("authorization", "Bearer viewer-token")
	authorized := httptest.NewRecorder()
	srv.Routes().ServeHTTP(authorized, authorizedReq)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d", authorized.Code)
	}
}

func TestSavedParserProfileIsUsedForUpload(t *testing.T) {
	srv := NewServer(store.NewMemoryStore())
	handler := srv.Routes()

	profileReq := httptest.NewRequest(http.MethodPost, "/api/parser-profiles", bytes.NewBufferString(`{
		"customer_id": "acme",
		"header_row": 2,
		"required_columns": ["month", "revenue"]
	}`))
	profileReq.Header.Set("content-type", "application/json")
	profileRes := httptest.NewRecorder()
	handler.ServeHTTP(profileRes, profileReq)
	if profileRes.Code != http.StatusCreated {
		t.Fatalf("profile status = %d body = %s", profileRes.Code, profileRes.Body.String())
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "sales.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "generated report\nmonth,region,revenue\n2026-01-01,west,10\n"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("customer_id", "acme"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	upload := httptest.NewRequest(http.MethodPost, "/api/sources", body)
	upload.Header.Set("content-type", writer.FormDataContentType())
	uploadRes := httptest.NewRecorder()
	handler.ServeHTTP(uploadRes, upload)
	if uploadRes.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body = %s", uploadRes.Code, uploadRes.Body.String())
	}
	var ds struct {
		Columns []struct {
			Name string `json:"name"`
		} `json:"columns"`
	}
	if err := json.NewDecoder(uploadRes.Body).Decode(&ds); err != nil {
		t.Fatal(err)
	}
	if ds.Columns[0].Name != "month" {
		t.Fatalf("first column = %s", ds.Columns[0].Name)
	}
}

func TestAddChartToExistingDashboard(t *testing.T) {
	mem := store.NewMemoryStore()
	srv := NewServer(mem)
	handler := srv.Routes()

	ds := datasetFixture(mem)
	chartReq := httptest.NewRequest(http.MethodPost, "/api/charts", bytes.NewBufferString(`{"dataset_id":"`+ds.ID+`","prompt":"revenue by region"}`))
	chartReq.Header.Set("content-type", "application/json")
	chartRes := httptest.NewRecorder()
	handler.ServeHTTP(chartRes, chartReq)
	if chartRes.Code != http.StatusCreated {
		t.Fatalf("chart status = %d body = %s", chartRes.Code, chartRes.Body.String())
	}
	var cfg struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(chartRes.Body).Decode(&cfg); err != nil {
		t.Fatal(err)
	}

	dashboardReq := httptest.NewRequest(http.MethodPost, "/api/dashboards", bytes.NewBufferString(`{"title":"Sales","chart_ids":["`+cfg.ID+`"]}`))
	dashboardReq.Header.Set("content-type", "application/json")
	dashboardRes := httptest.NewRecorder()
	handler.ServeHTTP(dashboardRes, dashboardReq)
	if dashboardRes.Code != http.StatusCreated {
		t.Fatalf("dashboard status = %d body = %s", dashboardRes.Code, dashboardRes.Body.String())
	}
	var dash struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(dashboardRes.Body).Decode(&dash); err != nil {
		t.Fatal(err)
	}

	addReq := httptest.NewRequest(http.MethodPut, "/api/dashboards/"+dash.ID+"/charts", bytes.NewBufferString(`{"chart_id":"`+cfg.ID+`"}`))
	addReq.Header.Set("content-type", "application/json")
	addRes := httptest.NewRecorder()
	handler.ServeHTTP(addRes, addReq)
	if addRes.Code != http.StatusOK {
		t.Fatalf("add status = %d body = %s", addRes.Code, addRes.Body.String())
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/dashboards/"+dash.ID, nil)
	detailRes := httptest.NewRecorder()
	handler.ServeHTTP(detailRes, detailReq)
	if detailRes.Code != http.StatusOK {
		t.Fatalf("detail status = %d body = %s", detailRes.Code, detailRes.Body.String())
	}
	var detail struct {
		Charts []struct {
			ID string `json:"id"`
		} `json:"charts"`
	}
	if err := json.NewDecoder(detailRes.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Charts) != 1 || detail.Charts[0].ID != cfg.ID {
		t.Fatalf("unexpected dashboard detail %#v", detail)
	}
}

func TestUpdateChartRefinesExistingChart(t *testing.T) {
	mem := store.NewMemoryStore()
	srv := NewServer(mem)
	handler := srv.Routes()
	ds := datasetFixture(mem)

	createReq := httptest.NewRequest(http.MethodPost, "/api/charts", bytes.NewBufferString(`{"dataset_id":"`+ds.ID+`","prompt":"revenue by region"}`))
	createReq.Header.Set("content-type", "application/json")
	createRes := httptest.NewRecorder()
	handler.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", createRes.Code, createRes.Body.String())
	}
	var created struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(createRes.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/api/charts/"+created.ID, bytes.NewBufferString(`{"prompt":"revenue by region as a pie chart"}`))
	updateReq.Header.Set("content-type", "application/json")
	updateRes := httptest.NewRecorder()
	handler.ServeHTTP(updateRes, updateReq)
	if updateRes.Code != http.StatusOK {
		t.Fatalf("update status = %d body = %s", updateRes.Code, updateRes.Body.String())
	}
	var updated struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(updateRes.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Type != "pie" {
		t.Fatalf("unexpected updated chart %#v", updated)
	}
}

func TestAutoDashboardCreatesChartsAndDashboard(t *testing.T) {
	mem := store.NewMemoryStore()
	srv := NewServer(mem)
	handler := srv.Routes()
	ds := datasetFixture(mem)

	req := httptest.NewRequest(http.MethodPost, "/api/datasets/"+ds.ID+"/auto-dashboard", bytes.NewBufferString(`{"title":"Auto Sales","max_charts":3}`))
	req.Header.Set("content-type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}
	var detail struct {
		Title  string `json:"title"`
		Charts []struct {
			ID string `json:"id"`
		} `json:"charts"`
	}
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Title != "Auto Sales" {
		t.Fatalf("title = %s", detail.Title)
	}
	if len(detail.Charts) == 0 {
		t.Fatal("expected generated charts")
	}
}

func TestAutoBuildCreatesDashboardAndCharts(t *testing.T) {
	mem := store.NewMemoryStore()
	srv := NewServer(mem)
	handler := srv.Routes()
	ds := datasetFixture(mem)

	req := httptest.NewRequest(http.MethodPost, "/api/datasets/"+ds.ID+"/auto-build", bytes.NewBufferString(`{"title":"Auto Generated","max_charts":3}`))
	req.Header.Set("content-type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}
	var body struct {
		DatasetID   string   `json:"dataset_id"`
		DashboardID string   `json:"dashboard_id"`
		Title       string   `json:"title"`
		ChartIDs    []string `json:"chart_ids"`
		Charts      []struct {
			ID string `json:"id"`
		} `json:"charts"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.DatasetID != ds.ID {
		t.Fatalf("dataset id = %s", body.DatasetID)
	}
	if body.DashboardID == "" {
		t.Fatal("missing dashboard id")
	}
	if body.Title != "Auto Generated" {
		t.Fatalf("title = %s", body.Title)
	}
	if len(body.ChartIDs) == 0 {
		t.Fatal("expected chart ids")
	}
	if len(body.Charts) != len(body.ChartIDs) {
		t.Fatalf("charts %d ids %d", len(body.Charts), len(body.ChartIDs))
	}
}

func TestAutoBuildDefaultsOnEmptyBody(t *testing.T) {
	mem := store.NewMemoryStore()
	srv := NewServer(mem)
	handler := srv.Routes()
	ds := datasetFixture(mem)

	req := httptest.NewRequest(http.MethodPost, "/api/datasets/"+ds.ID+"/auto-build", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}
	var body struct {
		DashboardID string `json:"dashboard_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.DashboardID == "" {
		t.Fatal("expected dashboard id")
	}
}

func TestAutoChartsCreatesCharts(t *testing.T) {
	mem := store.NewMemoryStore()
	srv := NewServer(mem)
	handler := srv.Routes()
	ds := datasetFixture(mem)

	req := httptest.NewRequest(http.MethodPost, "/api/datasets/"+ds.ID+"/auto-charts", bytes.NewBufferString(`{"max_charts":3}`))
	req.Header.Set("content-type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}
	var body struct {
		ChartIDs []string `json:"chart_ids"`
		Charts   []struct {
			ID string `json:"id"`
		} `json:"charts"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.ChartIDs) == 0 {
		t.Fatal("expected chart ids")
	}
	if len(body.Charts) != len(body.ChartIDs) {
		t.Fatalf("charts %d ids %d", len(body.Charts), len(body.ChartIDs))
	}
}

func TestAutoChartsDefaultsOnEmptyBody(t *testing.T) {
	mem := store.NewMemoryStore()
	srv := NewServer(mem)
	handler := srv.Routes()
	ds := datasetFixture(mem)

	req := httptest.NewRequest(http.MethodPost, "/api/datasets/"+ds.ID+"/auto-charts", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}
}

func TestAutoDashboardDefaultsOnEmptyBody(t *testing.T) {
	mem := store.NewMemoryStore()
	srv := NewServer(mem)
	handler := srv.Routes()
	ds := datasetFixture(mem)

	req := httptest.NewRequest(http.MethodPost, "/api/datasets/"+ds.ID+"/auto-dashboard", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}
}

func TestCreateMetricAndQueryIt(t *testing.T) {
	mem := store.NewMemoryStore()
	srv := NewServer(mem)
	handler := srv.Routes()
	ds := datasetFixture(mem)

	metricReq := httptest.NewRequest(http.MethodPost, "/api/datasets/"+ds.ID+"/metrics", bytes.NewBufferString(`{"name":"avg_revenue","column":"revenue","aggregate":"AVG"}`))
	metricReq.Header.Set("content-type", "application/json")
	metricRes := httptest.NewRecorder()
	handler.ServeHTTP(metricRes, metricReq)
	if metricRes.Code != http.StatusCreated {
		t.Fatalf("metric status = %d body = %s", metricRes.Code, metricRes.Body.String())
	}

	queryReq := httptest.NewRequest(http.MethodPost, "/api/datasets/"+ds.ID+"/query", bytes.NewBufferString(`{"metrics":["avg_revenue"],"group_by":["region"]}`))
	queryReq.Header.Set("content-type", "application/json")
	queryRes := httptest.NewRecorder()
	handler.ServeHTTP(queryRes, queryReq)
	if queryRes.Code != http.StatusOK {
		t.Fatalf("query status = %d body = %s", queryRes.Code, queryRes.Body.String())
	}
}

func TestCreateSourceFromURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`[{"region":"west","revenue":10}]`))
	}))
	defer ts.Close()

	srv := NewServer(store.NewMemoryStore())
	handler := srv.Routes()
	payload := `{"url":"` + ts.URL + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sources/url", bytes.NewBufferString(payload))
	req.Header.Set("content-type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}

	var ds struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&ds); err != nil {
		t.Fatal(err)
	}
	if ds.ID == "" {
		t.Fatal("expected dataset id")
	}
}

func TestCreateSourceFromAPI(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Query().Get("page") != "2" {
			t.Fatalf("query page = %s", r.URL.Query().Get("page"))
		}
		if r.Header.Get("authorization") != "Bearer token" {
			t.Fatalf("authorization = %s", r.Header.Get("authorization"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) == "" {
			t.Fatal("expected body")
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`[{"region":"west","revenue":11}]`))
	}))
	defer ts.Close()

	srv := NewServer(store.NewMemoryStore())
	handler := srv.Routes()
	payload := `{
		"url":"` + ts.URL + `",
		"method":"POST",
		"headers":{"Authorization":"Bearer token"},
		"query":{"page":"2"},
		"body":{"page":1},
		"required_columns":["region","revenue"]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/sources/api", bytes.NewBufferString(payload))
	req.Header.Set("content-type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}

	var ds struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&ds); err != nil {
		t.Fatal(err)
	}
	if ds.ID == "" {
		t.Fatal("expected dataset id")
	}
}

func TestImportIntermediaryDataset(t *testing.T) {
	srv := NewServer(store.NewMemoryStore())
	handler := srv.Routes()
	payload := `{
		"name":"finance_report",
		"source":{"type":"intermediary","name":"finance_upload"},
		"headers":["region","revenue"],
		"rows":[["west","10"],["east","20"]],
		"metrics":[{"name":"avg_revenue","column":"revenue","aggregate":"AVG"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/intermediary", bytes.NewBufferString(payload))
	req.Header.Set("content-type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}
	var ds struct {
		ID      string         `json:"id"`
		Source  dataset.Source `json:"source"`
		Metrics []struct {
			Name string `json:"name"`
		} `json:"metrics"`
	}
	if err := json.NewDecoder(res.Body).Decode(&ds); err != nil {
		t.Fatal(err)
	}
	if ds.ID == "" {
		t.Fatal("expected dataset id")
	}
	if ds.Source.Type != "intermediary" {
		t.Fatalf("source type = %s", ds.Source.Type)
	}
	if len(ds.Metrics) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(ds.Metrics))
	}
}

func TestImportIntermediaryDatasetRequiresHeadersAndRows(t *testing.T) {
	srv := NewServer(store.NewMemoryStore())
	handler := srv.Routes()

	t.Run("missing headers", func(t *testing.T) {
		payload := `{
			"name":"finance_report",
			"headers":[],
			"rows":[["west","10"]]
		}`
		req := httptest.NewRequest(http.MethodPost, "/api/datasets/intermediary", bytes.NewBufferString(payload))
		req.Header.Set("content-type", "application/json")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
		}
	})

	t.Run("missing rows", func(t *testing.T) {
		payload := `{
			"name":"finance_report",
			"headers":["region","revenue"],
			"rows":[]
		}`
		req := httptest.NewRequest(http.MethodPost, "/api/datasets/intermediary", bytes.NewBufferString(payload))
		req.Header.Set("content-type", "application/json")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
		}
	})
}

func TestNormalizeIntermediaryPayloadTrimsAndAlignsRows(t *testing.T) {
	headers, rows := normalizeIntermediaryPayload(
		[]string{" region ", "", "revenue"},
		[][]string{{"west", "", "10", "extra"}, {"east", "22"}},
	)
	if len(headers) != 2 {
		t.Fatalf("headers len = %d", len(headers))
	}
	if headers[0] != "region" || headers[1] != "revenue" {
		t.Fatalf("headers = %#v", headers)
	}
	if len(rows) != 2 {
		t.Fatalf("rows len = %d", len(rows))
	}
	if len(rows[0]) != 2 || rows[0][1] != "10" {
		t.Fatalf("first row = %#v", rows[0])
	}
	if len(rows[1]) != 2 || rows[1][1] != "22" {
		t.Fatalf("second row = %#v", rows[1])
	}
}

func datasetFixture(mem *store.MemoryStore) dataset.Dataset {
	ds := dataset.Build(mem.NextID("dataset"), "sales", dataset.Source{Type: dataset.SourceCSV, Name: "sales.csv"}, []string{"region", "revenue"}, [][]string{{"west", "10"}})
	mem.SaveDataset(ds)
	return ds
}
