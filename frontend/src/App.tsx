import { DragEvent, FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import * as echarts from 'echarts';
import { Button } from './components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './components/ui/card';
import { Input } from './components/ui/input';
import { Label } from './components/ui/label';
import { Textarea } from './components/ui/textarea';

import './styles.css';

type DashboardItem = {
  chart_id: string;
  x?: number;
  y?: number;
  w?: number;
  h?: number;
};

type DashboardFilter = {
  expression: string;
  added_at?: string;
};

type Chart = {
  id: string;
  title: string;
  type: string;
  dataset_id: string;
  echarts: Record<string, unknown>;
};

type Dashboard = {
  id: string;
  title: string;
  chart_ids: string[];
  layout: DashboardItem[];
  filters: DashboardFilter[];
  charts: Chart[];
};

type Dataset = {
  id: string;
  name: string;
};

type ChatMessage = {
  role: 'user' | 'ai';
  text: string;
};

const SECTION_ORDER = ['datasets', 'charts', 'dashboards'] as const;
type ActiveSection = (typeof SECTION_ORDER)[number];

const defaultIntermediaryPayload = {
  name: 'finance_report',
  source: {
    type: 'intermediary',
    name: 'finance_upload',
  },
  headers: ['region', 'revenue'],
  rows: [
    ['west', '10'],
    ['east', '20'],
  ],
  metrics: [
    {
      name: 'avg_revenue',
      column: 'revenue',
      aggregate: 'AVG',
    },
  ],
};

const defaultPrompt = 'monthly revenue by region as a bar chart';

function layoutFromChartIDs(chartIDs: string[]): DashboardItem[] {
  return chartIDs.map((chartID, index) => ({
    chart_id: chartID,
    x: (index % 3) * 4,
    y: Math.floor(index / 3) * 4,
    w: 4,
    h: 3,
  }));
}

function normalizeLayout(layout: DashboardItem[]): DashboardItem[] {
  return layout
    .map((item, index) => ({
      ...item,
      x: (index % 3) * 4,
      y: Math.floor(index / 3) * 4,
      w: 4,
      h: 3,
    }))
    .filter((item) => item.chart_id);
}

async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers as HeadersInit | undefined);
  if (!headers.has('content-type') && options.body && typeof options.body === 'string') {
    headers.set('content-type', 'application/json');
  }

  const key = localStorage.getItem('jodata.apiKey') || '';
  if (key && !headers.has('authorization')) {
    headers.set('authorization', `Bearer ${key}`);
  }

  const response = await fetch(path, {
    ...options,
    headers,
  });

  const payload = await response.json();
  if (!response.ok) {
    throw new Error((payload as { error?: string }).error || response.statusText);
  }
  return payload as T;
}

export function App() {
  const [apiKey, setApiKey] = useState(localStorage.getItem('jodata.apiKey') || '');
  const [status, setStatus] = useState('Ready to build charts and dashboards.');
  const [datasets, setDatasets] = useState<Dataset[]>([]);
  const [charts, setCharts] = useState<Chart[]>([]);
  const [dashboards, setDashboards] = useState<Dashboard[]>([]);

  const [datasetId, setDatasetId] = useState('');
  const [chartPrompt, setChartPrompt] = useState(defaultPrompt);
  const [filterPrompt, setFilterPrompt] = useState('add filter: region = west');
  const [dashboardTitle, setDashboardTitle] = useState('Analysis Dashboard');
  const [thread, setThread] = useState<ChatMessage[]>([]);

  const [currentChart, setCurrentChart] = useState<Chart | null>(null);
  const [currentDashboardId, setCurrentDashboardId] = useState('');
  const [currentDashboard, setCurrentDashboard] = useState<Dashboard | null>(null);
  const [selectedChartId, setSelectedChartId] = useState('');

  const [datasetFileName, setDatasetFileName] = useState('');
  const [customerID, setCustomerID] = useState('');
  const [sheet, setSheet] = useState('');
  const [headerRow, setHeaderRow] = useState('1');

  const [intermediary, setIntermediary] = useState(JSON.stringify(defaultIntermediaryPayload, null, 2));
  const [dragFrom, setDragFrom] = useState<number | null>(null);
  const [activeSection, setActiveSection] = useState<ActiveSection>('datasets');
  const [isReorderLocked, setIsReorderLocked] = useState(false);
  const sectionRefs = useRef<{ [key in ActiveSection]: HTMLDivElement | null }>({
    datasets: null,
    charts: null,
    dashboards: null,
  });

  const chartContainerRef = useRef<HTMLDivElement | null>(null);
  const chartInstanceRef = useRef<echarts.ECharts | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    localStorage.setItem('jodata.apiKey', apiKey);
    refresh().catch((error) => {
      setStatus((error as Error).message || 'Unable to initialize workspace.');
    });
  }, [apiKey]);

  useEffect(() => {
    const container = chartContainerRef.current;
    if (!container) {
      return;
    }
    chartInstanceRef.current = echarts.init(container);
    const onResize = () => chartInstanceRef.current?.resize();
    window.addEventListener('resize', onResize);
    return () => {
      window.removeEventListener('resize', onResize);
      chartInstanceRef.current?.dispose();
      chartInstanceRef.current = null;
    };
  }, []);

  useEffect(() => {
    const chart = chartInstanceRef.current;
    if (!chart) return;
    if (currentChart?.echarts) {
      chart.setOption(currentChart.echarts);
    } else {
      chart.clear();
    }
  }, [currentChart]);

  useEffect(() => {
    const anchorY = 120;
    const selectActiveSection = () => {
      let nextSection = activeSection;
      let nextDistance = Number.POSITIVE_INFINITY;

      SECTION_ORDER.forEach((section) => {
        const element = sectionRefs.current[section];
        if (!element) {
          return;
        }

        const rect = element.getBoundingClientRect();
        const visible = rect.bottom >= 0 && rect.top <= window.innerHeight;
        if (!visible) {
          return;
        }

        const distance = Math.abs(rect.top - anchorY);
        if (distance < nextDistance) {
          nextDistance = distance;
          nextSection = section;
        }
      });

      if (nextDistance !== Number.POSITIVE_INFINITY) {
        setActiveSection(nextSection);
      }
    };

    window.addEventListener('scroll', selectActiveSection, { passive: true });
    window.addEventListener('resize', selectActiveSection);
    selectActiveSection();

    return () => {
      window.removeEventListener('scroll', selectActiveSection);
      window.removeEventListener('resize', selectActiveSection);
    };
  }, []);

  useEffect(() => {
    const mediaQuery = window.matchMedia('(max-width: 768px)');
    const syncLockState = (event: MediaQueryList | MediaQueryListEvent) => {
      setIsReorderLocked('matches' in event ? event.matches : mediaQuery.matches);
    };

    syncLockState(mediaQuery);
    mediaQuery.addEventListener('change', syncLockState);
    return () => {
      mediaQuery.removeEventListener('change', syncLockState);
    };
  }, []);

  async function refresh() {
    const [datasetsData, chartsData, dashboardsData] = await Promise.all([
      api<Dataset[]>('/api/datasets', { method: 'GET' }),
      api<Chart[]>('/api/charts', { method: 'GET' }),
      api<Dashboard[]>('/api/dashboards', { method: 'GET' }),
    ]);

    setDatasets(datasetsData);
    setCharts(chartsData);
    setDashboards(dashboardsData);

    if (!datasetId && datasetsData[0]) {
      setDatasetId(datasetsData[0].id);
    }
    if (!selectedChartId && chartsData[0]) {
      setSelectedChartId(chartsData[0].id);
      setCurrentChart(chartsData[0]);
    }

    if (!currentDashboardId && dashboardsData[0]) {
      await loadDashboard(dashboardsData[0].id);
      return;
    }
    if (currentDashboardId) {
      const stillExists = dashboardsData.some((dash) => dash.id === currentDashboardId);
      if (stillExists) {
        await loadDashboard(currentDashboardId);
      } else if (dashboardsData[0]) {
        await loadDashboard(dashboardsData[0].id);
      }
    }
  }

  async function loadDashboard(id: string) {
    if (!id) return;
    const detail = await api<Dashboard>(`/api/dashboards/${id}`, { method: 'GET' });
    setCurrentDashboardId(detail.id);
    setDashboardTitle(detail.title || dashboardTitle);
    setCurrentDashboard({
      ...detail,
      layout: detail.layout?.length ? detail.layout : layoutFromChartIDs(detail.chart_ids || []),
      filters: detail.filters || [],
    });
  }

  function selectChart(chartId: string) {
    const found = charts.find((item) => item.id === chartId);
    if (!found) {
      return;
    }
    setSelectedChartId(chartId);
    setCurrentChart(found);
    appendThread('user', `Inspecting chart ${found.title || found.id} from AI gallery.`);
  }

  function appendThread(role: 'user' | 'ai', text: string) {
    setThread((prev) => [...prev.slice(-18), { role, text }]);
  }

  async function uploadDataset(e: FormEvent) {
    e.preventDefault();
    const file = fileRef.current?.files?.[0];
    if (!file) {
      setStatus('Choose a data file first.');
      return;
    }

    const payload = new FormData();
    payload.append('file', file);
    payload.append('customer_id', customerID);
    payload.append('sheet', sheet);
    payload.append('header_row', headerRow || '1');

    const dataset = await api<Dataset>('/api/sources', {
      method: 'POST',
      body: payload,
    });

    await refresh();
    setDatasetId(dataset.id);
    setStatus(`Dataset ${dataset.id} imported successfully.`);
    setDatasetFileName(file.name);
  }

  async function importIntermediary(e: FormEvent) {
    e.preventDefault();
    let parsed: unknown;
    try {
      parsed = JSON.parse(intermediary);
    } catch {
      setStatus('Invalid intermediary payload JSON.');
      return;
    }

    const dataset = await api<Dataset>('/api/datasets/intermediary', {
      method: 'POST',
      body: JSON.stringify(parsed),
    });

    setDatasetId(dataset.id);
    await refresh();
    setStatus(`Intermediary dataset ${dataset.id} imported.`);
  }

  async function createChart() {
    if (!datasetId) {
      setStatus('Pick or create a dataset first.');
      return;
    }
    if (!chartPrompt.trim()) {
      setStatus('Add a chart prompt.');
      return;
    }

    appendThread('user', chartPrompt);
    const created = await api<Chart>('/api/charts', {
      method: 'POST',
      body: JSON.stringify({
        dataset_id: datasetId,
        prompt: chartPrompt,
      }),
    });

    setCurrentChart(created);
    setSelectedChartId(created.id);
    setCharts((prev) => [created, ...prev]);
    appendThread('ai', `Created chart ${created.id}`);
    setStatus(`Chart ${created.id} created.`);
  }

  async function refineCurrentChart() {
    if (!currentChart) {
      setStatus('Create a chart first.');
      return;
    }

    appendThread('user', chartPrompt);
    const updated = await api<Chart>(`/api/charts/${currentChart.id}`, {
      method: 'PUT',
      body: JSON.stringify({ prompt: chartPrompt }),
    });

    setCurrentChart(updated);
    setSelectedChartId(updated.id);
    setCharts((prev) => prev.map((item) => (item.id === updated.id ? updated : item)));
    appendThread('ai', `Refined chart ${updated.id}`);
    setStatus(`Chart ${updated.id} refined.`);
  }

  async function createDashboardFromCurrent() {
    if (!currentChart) {
      setStatus('Create a chart before creating a dashboard.');
      return;
    }

    const created = await api<Dashboard>('/api/dashboards', {
      method: 'POST',
      body: JSON.stringify({
        title: dashboardTitle || 'Analysis Dashboard',
        chart_ids: [currentChart.id],
      }),
    });

    setCurrentDashboardId(created.id);
    await loadDashboard(created.id);
    await refresh();
    setStatus(`Dashboard ${created.id} created from chart ${currentChart.id}.`);
  }

  async function addChartToCurrentDashboard() {
    if (!currentDashboardId) {
      setStatus('Pick a dashboard first.');
      return;
    }
    if (!currentChart) {
      setStatus('Create a chart before adding to dashboard.');
      return;
    }

    const updated = await api<Dashboard>(`/api/dashboards/${currentDashboardId}/charts`, {
      method: 'PUT',
      body: JSON.stringify({ chart_id: currentChart.id }),
    });

    setCurrentDashboard(updated);
    setCurrentDashboardId(updated.id);
    setStatus(`Added chart ${currentChart.id} to dashboard ${updated.id}.`);
    await loadDashboard(updated.id);
    await refresh();
  }

  async function removeCurrentDashboardTile(chartId: string) {
    if (!currentDashboard) {
      return;
    }
    const layout = currentDashboard.layout.filter((item) => item.chart_id !== chartId);
    setCurrentDashboard({
      ...currentDashboard,
      layout,
      chart_ids: layout.map((item) => item.chart_id),
    });
    setStatus(`Removed ${chartId} from dashboard layout (unsaved).`);
  }

  async function reorderChartOnDashboard(toIndex: number) {
    if (!currentDashboard || dragFrom === null) {
      return;
    }
    const layout = [...currentDashboard.layout];
    const source = layout.splice(dragFrom, 1);
    layout.splice(toIndex, 0, source[0]);
    const updatedLayout = normalizeLayout(layout);

    setCurrentDashboard({
      ...currentDashboard,
      layout: updatedLayout,
      chart_ids: updatedLayout.map((item) => item.chart_id),
    });
    setDragFrom(null);
    setStatus('Layout order changed locally. Save dashboard to persist.');
  }

  async function saveDashboard() {
    if (!currentDashboard) {
      setStatus('No dashboard selected.');
      return;
    }

    const next = {
      title: dashboardTitle,
      layout: normalizeLayout(currentDashboard.layout || []),
      filters: currentDashboard.filters || [],
    };

    const refreshed = await api<Dashboard>(`/api/dashboards/${currentDashboard.id}`, {
      method: 'PUT',
      body: JSON.stringify(next),
    });

    await loadDashboard(refreshed.id);
    await refresh();
    setStatus(`Dashboard ${refreshed.id} saved.`);
  }

  async function sendFilterMessage() {
    if (!currentDashboardId) {
      setStatus('No dashboard selected.');
      return;
    }
    const detail = await api<Dashboard>(`/api/dashboards/${currentDashboardId}/chat`, {
      method: 'POST',
      body: JSON.stringify({ message: filterPrompt }),
    });
    setCurrentDashboard(detail);
    setStatus(`Filter command applied on ${detail.id}.`);
  }

  const chartLookup = useMemo(() => {
    const rows = Object.fromEntries(charts.map((item) => [item.id, item]));
    return rows;
  }, [charts]);

  function onDashboardChoose(value: string) {
    setCurrentDashboardId(value);
    loadDashboard(value).catch(() => {
      setStatus('Unable to open selected dashboard.');
    });
  }

  function moveCurrentDashboardTile(index: number, delta: number) {
    if (!currentDashboard) {
      return;
    }
    const layout = [...currentDashboard.layout];
    const targetIndex = index + delta;
    if (targetIndex < 0 || targetIndex >= layout.length) {
      return;
    }

    const [item] = layout.splice(index, 1);
    if (!item) {
      return;
    }

    layout.splice(targetIndex, 0, item);
    const updatedLayout = normalizeLayout(layout);
    setCurrentDashboard({
      ...currentDashboard,
      layout: updatedLayout,
      chart_ids: updatedLayout.map((item) => item.chart_id),
    });
    setStatus(`Moved ${item.chart_id} ${delta < 0 ? 'up' : 'down'} in dashboard.`);
  }

  function onDragStart(index: number) {
    if (isReorderLocked) {
      return;
    }
    setDragFrom(index);
  }

  function onDragOver(event: DragEvent<HTMLDivElement>) {
    if (isReorderLocked) {
      return;
    }
    event.preventDefault();
  }

  function onDrop(index: number) {
    if (isReorderLocked) {
      return;
    }
    reorderChartOnDashboard(index);
  }

  function toggleReorderLock() {
    setIsReorderLocked((prev) => !prev);
    setDragFrom(null);
  }

  function scrollToSection(section: ActiveSection) {
    setActiveSection(section);
    sectionRefs.current[section]?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  const layoutToRender = currentDashboard?.layout || [];
  const hasDashboard = Boolean(currentDashboard);

  return (
    <div className="app-shell">
      <aside className="workspace-nav">
        <div className="workspace-logo">⚡ Jodata</div>
        <p className="nav-title">Workspace</p>
        <div className="section-nav">
          <button
            className={`nav-link ${activeSection === 'datasets' ? 'active' : ''}`}
            type="button"
            aria-current={activeSection === 'datasets' ? 'page' : undefined}
            onClick={() => scrollToSection('datasets')}
          >
            Datasets
          </button>
          <button
            className={`nav-link ${activeSection === 'charts' ? 'active' : ''}`}
            type="button"
            aria-current={activeSection === 'charts' ? 'page' : undefined}
            onClick={() => scrollToSection('charts')}
          >
            Charts
          </button>
          <button
            className={`nav-link ${activeSection === 'dashboards' ? 'active' : ''}`}
            type="button"
            aria-current={activeSection === 'dashboards' ? 'page' : undefined}
            onClick={() => scrollToSection('dashboards')}
          >
            Dashboards
          </button>
        </div>

        <p className="nav-title">AI Flow</p>
        <p className="text-xs text-slate-200/75">1) Ingest → 2) Generate chart → 3) Add to dashboard → 4) Filter → 5) Save</p>
      </aside>

      <div className="workspace-main">
        <header className="topbar">
          <div>
            <h1>AI BI Workbench</h1>
            <p className="topbar-kicker">Superset-inspired data exploration with AI charting.</p>
          </div>
          <Button variant="outline" type="button" onClick={refresh}>
            Reload workspace
          </Button>
        </header>

        <div className="workspace-main-scroll">
          <div className="flow-strip">
            <span className="flow-chip">Explore</span>
            <span className="flow-chip">Create Chart</span>
            <span className="flow-chip">Assemble Dashboard</span>
            <span className="flow-chip">Layout</span>
            <span className="flow-chip">Filter Chat</span>
            <span className="flow-chip">Save</span>
          </div>

          <main className="content-grid">
            <section className="left-column">
              <section ref={(node) => (sectionRefs.current.datasets = node)} className="workflow-section">
                <Card>
                <CardHeader>
                  <CardTitle>Datasets</CardTitle>
                  <CardDescription>Upload Excel/CSV or paste intermediary payload.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  <form className="space-y-2" onSubmit={uploadDataset}>
                    <Label htmlFor="apiKey">API Key</Label>
                    <Input
                      id="apiKey"
                      type="password"
                      value={apiKey}
                      onChange={(event) => setApiKey(event.target.value)}
                      placeholder="optional token for protected API"
                    />

                    <Label htmlFor="datasetFile">Data file</Label>
                    <Input
                      id="datasetFile"
                      type="file"
                      accept=".csv,.json,.xlsx,.xlsm"
                      onChange={(event) => setDatasetFileName(event.target.files?.[0]?.name || '')}
                      ref={fileRef}
                    />

                    <Label htmlFor="customer">Customer ID</Label>
                    <Input id="customer" value={customerID} onChange={(event) => setCustomerID(event.target.value)} />

                    <div className="form-two-cols">
                      <div>
                        <Label htmlFor="sheet">Sheet</Label>
                        <Input id="sheet" value={sheet} onChange={(event) => setSheet(event.target.value)} />
                      </div>
                      <div>
                        <Label htmlFor="headerRow">Header row</Label>
                        <Input
                          id="headerRow"
                          type="number"
                          min={1}
                          value={headerRow}
                          onChange={(event) => setHeaderRow(event.target.value)}
                        />
                      </div>
                    </div>

                    <Button type="submit" className="w-full">
                      Ingest Source
                    </Button>
                    {datasetFileName && <CardDescription>Last uploaded file: {datasetFileName}</CardDescription>}
                  </form>

                  <form className="space-y-2" onSubmit={importIntermediary}>
                    <Label htmlFor="intermediary">Intermediary payload</Label>
                    <Textarea
                      id="intermediary"
                      rows={7}
                      value={intermediary}
                      onChange={(event) => setIntermediary(event.target.value)}
                    />
                    <Button variant="outline" type="submit" className="w-full">
                      Import intermediary
                    </Button>
                  </form>
                </CardContent>
              </Card>
              </section>

              <section ref={(node) => (sectionRefs.current.charts = node)} className="workflow-section">
                <Card>
                <CardHeader>
                  <CardTitle>Explore / Chart</CardTitle>
                  <CardDescription>Use natural language prompts to build chart definitions.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-2">
                  <Label htmlFor="dataset-select">Select dataset</Label>
                  <select
                    id="dataset-select"
                    className="w-full rounded-md border border-slate-200 px-3 py-2"
                    value={datasetId}
                    onChange={(event) => setDatasetId(event.target.value)}
                  >
                    {datasets.map((item) => (
                      <option value={item.id} key={item.id}>
                        {item.name} ({item.id})
                      </option>
                    ))}
                  </select>

                  <Label htmlFor="chartPrompt">AI chart prompt</Label>
                  <Textarea
                    id="chartPrompt"
                    value={chartPrompt}
                    onChange={(event) => setChartPrompt(event.target.value)}
                  />

                  <div className="card-like-border p-2">
                    <Label className="mb-2 block text-xs">Chart library</Label>
                    <div className="space-y-2">
                      {charts.map((chart) => (
                        <Button
                          key={chart.id}
                          type="button"
                          variant={selectedChartId === chart.id ? 'default' : 'outline'}
                          className="w-full justify-start"
                          onClick={() => selectChart(chart.id)}
                        >
                          {chart.title || chart.id}
                        </Button>
                      ))}
                      {charts.length === 0 && <CardDescription>No charts yet.</CardDescription>}
                    </div>
                  </div>

                  <div className="chat-thread card-like-border">
                    {thread.length === 0 ? (
                      <p className="text-xs text-slate-500">Conversation history appears here as charts are created.</p>
                    ) : (
                      thread.map((message, idx) => (
                        <div key={`${message.role}-${idx}`} className={`chat-message ${message.role}`}>
                          <strong>{message.role === 'user' ? 'You' : 'AI'}:</strong> {message.text}
                        </div>
                      ))
                    )}
                  </div>

                  <div className="split-actions">
                    <Button type="button" onClick={createChart}>
                      Create chart
                    </Button>
                    <Button type="button" variant="outline" onClick={refineCurrentChart}>
                      Refine current
                    </Button>
                  </div>
                </CardContent>
              </Card>
              </section>

              <section ref={(node) => (sectionRefs.current.dashboards = node)} className="workflow-section">
                <Card>
                <CardHeader>
                  <CardTitle>Dashboard builder</CardTitle>
                  <CardDescription>Create, add charts, rearrange, filter, and save.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-2">
                  <Label htmlFor="dashboardId">Current dashboard</Label>
                  <select
                    id="dashboardId"
                    className="w-full rounded-md border border-slate-200 px-3 py-2"
                    value={currentDashboardId}
                    onChange={(event) => onDashboardChoose(event.target.value)}
                  >
                    {dashboards.map((dash) => (
                      <option value={dash.id} key={dash.id}>
                        {dash.title} ({dash.id})
                      </option>
                    ))}
                  </select>

                  <Label htmlFor="dashboardTitle">Dashboard title</Label>
                  <Input
                    id="dashboardTitle"
                    value={dashboardTitle}
                    onChange={(event) => setDashboardTitle(event.target.value)}
                  />

                  <div className="split-actions">
                    <Button type="button" onClick={createDashboardFromCurrent}>
                      New dashboard from chart
                    </Button>
                    <Button type="button" variant="outline" onClick={addChartToCurrentDashboard}>
                      Add current chart
                    </Button>
                  </div>

                  <Label htmlFor="filterPrompt">Dashboard filter command</Label>
                  <Textarea
                    id="filterPrompt"
                    value={filterPrompt}
                    onChange={(event) => setFilterPrompt(event.target.value)}
                  />
                  <div className="split-actions">
                    <Button type="button" variant="outline" onClick={sendFilterMessage}>
                      Add / update filters
                    </Button>
                    <Button type="button" variant="outline" onClick={saveDashboard}>
                      Save dashboard
                    </Button>
                  </div>
                </CardContent>
              </Card>
              </section>
            </section>

            <section className="right-column">
              <Card>
                <CardHeader className="flex-row items-center justify-between">
                  <div>
                    <CardTitle>{currentChart?.title || 'No chart selected'}</CardTitle>
                    <CardDescription>
                      {currentChart
                        ? `${currentChart.id} / ${currentChart.type}`
                        : 'Create a chart in Explore to preview it here'}
                    </CardDescription>
                  </div>
                </CardHeader>
                <CardContent>
                  <div ref={chartContainerRef} className="chart-surface" />
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>{hasDashboard ? `Dashboard: ${currentDashboard?.title}` : 'No dashboard loaded'}</CardTitle>
                  <CardDescription>
                    {hasDashboard ? `${currentDashboard?.id}` : 'Create one and drop charts into it'}
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <h3 className="section-heading">Canvas layout</h3>
                  <p className="text-xs text-slate-500 mb-3">
                    {isReorderLocked
                      ? 'Drag ordering is locked. Use ▲/▼ or enable drag reorder to move cards.'
                      : 'Drag cards to reorder, or use ▲/▼ on mobile, then Remove to detach.'}
                  </p>
                  <div className="split-actions mb-3">
                    <Button type="button" variant="outline" onClick={toggleReorderLock} aria-pressed={!isReorderLocked}>
                      {isReorderLocked ? 'Enable drag reorder' : 'Disable drag reorder'}
                    </Button>
                  </div>

                  <div className="canvas">
                    {layoutToRender.map((item, index) => {
                      const chart = chartLookup[item.chart_id];
                      return (
                        <div
                          key={item.chart_id || `slot-${index}`}
                          className="tile"
                          draggable={!isReorderLocked}
                          onDragStart={() => onDragStart(index)}
                          onDragOver={onDragOver}
                          onDrop={() => onDrop(index)}
                        >
                          <div className="tile-title-row">
                            <span className="drag-handle" aria-hidden="true">
                              ≡
                            </span>
                            <div className="tile-content">
                            <div className="font-medium">{chart?.title || item.chart_id}</div>
                            <p className="text-xs text-slate-500">{item.chart_id}</p>
                            </div>
                          </div>
                          <div className="tile-actions">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => moveCurrentDashboardTile(index, -1)}
                              disabled={index === 0}
                              type="button"
                              aria-label={`Move ${chart?.title || item.chart_id} earlier`}
                            >
                              ▲
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => moveCurrentDashboardTile(index, 1)}
                              disabled={index === layoutToRender.length - 1}
                              type="button"
                              aria-label={`Move ${chart?.title || item.chart_id} later`}
                            >
                              ▼
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => removeCurrentDashboardTile(item.chart_id)}
                              type="button"
                              aria-label={`Remove ${chart?.title || item.chart_id} from dashboard`}
                            >
                              Remove
                            </Button>
                          </div>
                        </div>
                      );
                    })}
                    {!layoutToRender.length && (
                      <p className="text-sm text-slate-500">Add a chart first, then arrange tiles here.</p>
                    )}
                  </div>

                  <div className="section-gap" />
                  <h3 className="section-heading">Filters</h3>
                  <ul className="space-y-2">
                    {currentDashboard?.filters?.map((item, index) => (
                      <li
                        key={`${item.expression}-${index}`}
                        className="text-xs rounded-md border border-slate-200 bg-slate-50 px-2 py-1"
                      >
                        {item.expression}
                      </li>
                    ))}
                    {(!currentDashboard?.filters || currentDashboard.filters.length === 0) && (
                      <li className="text-xs text-slate-500">No filters yet.</li>
                    )}
                  </ul>

                  <p className="status-pill mt-3">{status}</p>
                </CardContent>
              </Card>
            </section>
          </main>
        </div>
      </div>
    </div>
  );
}
