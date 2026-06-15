<script lang="ts">
  import { onMount } from 'svelte'
  import type { InsightsRange, InsightsSection, InsightsResult, InsightsFilters } from '../lib/api/queryInsights'
  import { fetchInsights } from '../lib/api/queryInsights'
  import { openQueryTab } from '../lib/stores/tabs.svelte'
  import { formatNumber, formatBytes, formatElapsed } from '../lib/utils/format'
  import TrendChart from '../lib/components/common/TrendChart.svelte'
  import {
    Gauge, RefreshCw, AlertTriangle, Server, Timer, MemoryStick, Repeat,
    Users, Table2, Activity, ExternalLink, Search, X, Funnel,
  } from 'lucide-svelte'

  type SectionKey = Exclude<InsightsSection, 'summary' | 'volume' | 'latency'>

  interface ColumnSpec {
    key: string
    label: string
    mono?: boolean
    right?: boolean
    format?: (v: unknown) => string
  }

  interface SectionSpec {
    key: SectionKey
    label: string
    icon: typeof Timer
    columns: ColumnSpec[]
    openable?: boolean
    /** Clicking the funnel on a row applies this filter from this row field. */
    filterKey?: 'user' | 'table'
    filterField?: string
  }

  const num = (v: unknown) => Number(v ?? 0) || 0
  const fmtNum = (v: unknown) => formatNumber(num(v))
  const fmtBytes = (v: unknown) => formatBytes(num(v))
  const fmtMs = (v: unknown) => formatElapsed(num(v) / 1000)
  const fmtTime = (v: unknown) => String(v ?? '')

  const RANGES: { value: InsightsRange; label: string }[] = [
    { value: '1h', label: '1h' },
    { value: '6h', label: '6h' },
    { value: '24h', label: '24h' },
    { value: '7d', label: '7d' },
    { value: '30d', label: '30d' },
  ]

  const PATTERN_COLUMNS: ColumnSpec[] = [
    { key: 'sample_query', label: 'Query pattern', mono: true },
    { key: 'runs', label: 'Runs', right: true, format: fmtNum },
    { key: 'p50_ms', label: 'p50', right: true, format: fmtMs },
    { key: 'p95_ms', label: 'p95', right: true, format: fmtMs },
    { key: 'max_ms', label: 'Max', right: true, format: fmtMs },
    { key: 'max_memory', label: 'Peak mem', right: true, format: fmtBytes },
    { key: 'read_rows', label: 'Rows read', right: true, format: fmtNum },
    { key: 'last_seen', label: 'Last seen', right: true, format: fmtTime },
  ]

  const SECTIONS: SectionSpec[] = [
    { key: 'slow', label: 'Slow queries', icon: Timer, columns: PATTERN_COLUMNS, openable: true },
    { key: 'memory', label: 'Memory', icon: MemoryStick, columns: PATTERN_COLUMNS, openable: true },
    { key: 'frequent', label: 'Frequent', icon: Repeat, columns: PATTERN_COLUMNS, openable: true },
    {
      key: 'errors', label: 'Errors', icon: AlertTriangle, openable: true,
      columns: [
        { key: 'error_name', label: 'Error', mono: true },
        { key: 'exception_code', label: 'Code', right: true },
        { key: 'occurrences', label: 'Count', right: true, format: fmtNum },
        { key: 'sample_error', label: 'Last message' },
        { key: 'last_seen', label: 'Last seen', right: true, format: fmtTime },
      ],
    },
    {
      key: 'users', label: 'Users', icon: Users, filterKey: 'user', filterField: 'user',
      columns: [
        { key: 'user', label: 'User', mono: true },
        { key: 'runs', label: 'Runs', right: true, format: fmtNum },
        { key: 'failures', label: 'Failures', right: true, format: fmtNum },
        { key: 'p95_ms', label: 'p95', right: true, format: fmtMs },
        { key: 'read_bytes', label: 'Read', right: true, format: fmtBytes },
        { key: 'max_memory', label: 'Peak mem', right: true, format: fmtBytes },
      ],
    },
    {
      key: 'tables', label: 'Hot tables', icon: Table2, filterKey: 'table', filterField: 'table',
      columns: [
        { key: 'table', label: 'Table', mono: true },
        { key: 'reads', label: 'Queries', right: true, format: fmtNum },
        { key: 'read_rows', label: 'Rows read', right: true, format: fmtNum },
        { key: 'read_bytes', label: 'Bytes read', right: true, format: fmtBytes },
        { key: 'last_seen', label: 'Last read', right: true, format: fmtTime },
      ],
    },
  ]

  let range = $state<InsightsRange>('24h')
  let loading = $state(true)
  let refreshing = $state(false)
  let error = $state<string | null>(null)
  let unsupported = $state(false)

  // ── Filters (apply to the whole dashboard: tiles, charts, sections) ──
  let filters = $state<InsightsFilters>({})
  let searchInput = $state('')
  let searchTimer: ReturnType<typeof setTimeout> | null = null

  const KIND_OPTIONS = ['Select', 'Insert', 'Create', 'Alter', 'Drop', 'Other']
  const DURATION_OPTIONS: { value: number; label: string }[] = [
    { value: 0, label: 'Any duration' },
    { value: 100, label: '≥ 100ms' },
    { value: 1000, label: '≥ 1s' },
    { value: 10000, label: '≥ 10s' },
  ]

  const hasFilters = $derived(
    !!(filters.user || filters.kind || filters.search || filters.table || filters.minMs)
  )

  function applyFilters(patch: Partial<InsightsFilters>) {
    filters = { ...filters, ...patch }
    void loadAll(true)
  }

  function handleSearchInput() {
    if (searchTimer) clearTimeout(searchTimer)
    searchTimer = setTimeout(() => {
      applyFilters({ search: searchInput.trim() || undefined })
    }, 400)
  }

  function clearFilters() {
    searchInput = ''
    filters = {}
    void loadAll(true)
  }

  function filterFromRow(spec: SectionSpec, row: Record<string, unknown>) {
    if (!spec.filterKey || !spec.filterField) return
    const value = String(row[spec.filterField] ?? '')
    if (value) applyFilters({ [spec.filterKey]: value })
  }

  let summary = $state<Record<string, unknown> | null>(null)
  let meta = $state<{ cluster: string; is_cluster: boolean; degraded: boolean }>({ cluster: '', is_cluster: false, degraded: false })
  let volume = $state<Record<string, unknown>[]>([])
  let latency = $state<Record<string, unknown>[]>([])

  let activeSection = $state<SectionKey>('slow')
  let sectionResult = $state<InsightsResult | null>(null)
  let sectionLoading = $state(false)
  let sectionError = $state<string | null>(null)
  let loadSeq = 0

  // Backend caps sample_query at this many chars (queryinsights.SampleQueryCap);
  // samples at the cap are flagged when opened so broken SQL isn't run blindly.
  const SAMPLE_QUERY_CAP = 2000

  // Pass the already-resolved cluster on follow-up requests so the backend
  // can skip re-resolving it on every call.
  const knownCluster = () => (meta.is_cluster && meta.cluster ? meta.cluster : undefined)

  const activeSpec = $derived(SECTIONS.find((s) => s.key === activeSection) ?? SECTIONS[0])

  const failureCount = $derived(num(summary?.failed_queries))

  const volumeChart = $derived({
    x: volume.map((r) => num(r.t)),
    series: [
      { label: 'Queries', values: volume.map((r) => num(r.queries)), color: '#f97316', fill: 'rgba(249,115,22,0.14)' },
      { label: 'Failures', values: volume.map((r) => num(r.failures)), color: '#ef4444' },
    ],
  })
  const latencyChart = $derived({
    x: latency.map((r) => num(r.t)),
    series: [
      { label: 'p50', values: latency.map((r) => num(r.p50_ms)), color: '#7dd3fc' },
      { label: 'p95', values: latency.map((r) => num(r.p95_ms)), color: '#0ea5e9', fill: 'rgba(14,165,233,0.12)' },
    ],
  })

  async function loadAll(showSpinner = false) {
    const seq = ++loadSeq
    if (showSpinner) loading = true
    refreshing = true
    error = null
    try {
      const cluster = knownCluster()
      const [s, v, l] = await Promise.all([
        fetchInsights('summary', range, cluster, filters),
        fetchInsights('volume', range, cluster, filters),
        fetchInsights('latency', range, cluster, filters),
      ])
      if (seq !== loadSeq) return
      unsupported = !s.supported
      summary = s.data[0] ?? null
      meta = { cluster: s.cluster, is_cluster: s.is_cluster, degraded: !!s.degraded }
      volume = v.supported ? v.data : []
      latency = l.supported ? l.data : []
      void loadSection(activeSection, seq)
    } catch (e: any) {
      if (seq !== loadSeq) return
      error = e.message
    } finally {
      if (seq === loadSeq) {
        loading = false
        refreshing = false
      }
    }
  }

  async function loadSection(key: SectionKey, seq = loadSeq) {
    activeSection = key
    sectionLoading = true
    sectionError = null
    // Guard against both a newer loadAll (seq) and a newer section click
    // (activeSection): two rapid section clicks share the same seq, and the
    // slower response must not land under the other section's columns.
    const stale = () => seq !== loadSeq || key !== activeSection
    try {
      const res = await fetchInsights(key, range, knownCluster(), filters)
      if (stale()) return
      sectionResult = res
    } catch (e: any) {
      if (stale()) return
      sectionResult = null
      sectionError = e.message
    } finally {
      if (!stale()) sectionLoading = false
    }
  }

  function setRange(r: InsightsRange) {
    if (range === r) return
    range = r
    void loadAll(true)
  }

  function openPattern(row: Record<string, unknown>) {
    let sql = String(row.sample_query ?? '')
    if (!sql) return
    if (sql.length >= SAMPLE_QUERY_CAP) {
      sql = `-- ⚠ Query text truncated by Query Insights — fetch the full text from system.query_log by query hash.\n${sql}`
    }
    openQueryTab(sql)
  }

  onMount(() => {
    void loadAll(true)
    return () => {
      if (searchTimer) clearTimeout(searchTimer)
    }
  })
</script>

<div class="flex flex-col h-full overflow-hidden">
  <!-- Header -->
  <div class="ds-page-header">
    <!-- w-full: ds-page-header is itself flex, so this wrapper must stretch
         for justify-between to push the controls to the far right. -->
    <div class="w-full flex items-center justify-between gap-4">
      <div class="flex items-center gap-3 min-w-0">
        <Gauge size={20} class="text-ch-orange shrink-0" />
        <div class="min-w-0">
          <h1 class="ds-page-title">Query Insights</h1>
          <p class="ds-page-subtitle">Latency, load and failure analytics from system.query_log</p>
        </div>
      </div>
      <div class="flex items-center gap-2 shrink-0">
        {#if meta.is_cluster}
          <span class="ds-badge ds-badge-neutral inline-flex items-center gap-1.5">
            <Server size={12} /> {meta.cluster}
          </span>
        {/if}
        {#if meta.degraded}
          <span class="ds-badge ds-badge-warn inline-flex items-center gap-1.5" title="Some nodes could not be reached; showing local node only">
            <AlertTriangle size={12} /> Degraded
          </span>
        {/if}
        <div class="ds-segment">
          {#each RANGES as r}
            <button
              class="ds-segment-btn {range === r.value ? 'ds-segment-btn-active' : ''}"
              onclick={() => setRange(r.value)}
            >{r.label}</button>
          {/each}
        </div>
        <button class="ds-icon-btn" onclick={() => loadAll()} title="Refresh" aria-label="Refresh">
          <RefreshCw size={15} class={refreshing ? 'animate-spin' : ''} />
        </button>
      </div>
    </div>
  </div>

  <!-- Filter bar: applies to the whole dashboard (tiles, charts, sections) -->
  <div class="flex items-center gap-2 flex-wrap px-4 py-2 border-b border-gray-200 dark:border-gray-800 bg-gray-50/60 dark:bg-gray-900/40">
    <div class="relative">
      <Search size={13} class="absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-400" />
      <input
        class="w-64 pl-8 pr-7 py-1.5 text-xs rounded-md border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-950 text-gray-700 dark:text-gray-200 focus:outline-none focus:border-ch-blue"
        placeholder="Filter by query text..."
        bind:value={searchInput}
        oninput={handleSearchInput}
        spellcheck="false"
      />
      {#if searchInput}
        <button
          class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
          onclick={() => { searchInput = ''; applyFilters({ search: undefined }) }}
          aria-label="Clear search"
        >
          <X size={13} />
        </button>
      {/if}
    </div>

    <select
      class="px-2 py-1.5 text-xs rounded-md border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-950 text-gray-700 dark:text-gray-200 focus:outline-none focus:border-ch-blue"
      value={filters.kind ?? ''}
      onchange={(e) => applyFilters({ kind: e.currentTarget.value || undefined })}
    >
      <option value="">All kinds</option>
      {#each KIND_OPTIONS as k}
        <option value={k}>{k}</option>
      {/each}
    </select>

    <select
      class="px-2 py-1.5 text-xs rounded-md border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-950 text-gray-700 dark:text-gray-200 focus:outline-none focus:border-ch-blue"
      value={String(filters.minMs ?? 0)}
      onchange={(e) => applyFilters({ minMs: parseInt(e.currentTarget.value) || undefined })}
    >
      {#each DURATION_OPTIONS as d}
        <option value={String(d.value)}>{d.label}</option>
      {/each}
    </select>

    {#if filters.user}
      <span class="inline-flex items-center gap-1 pl-1.5 pr-0.5 py-0.5 text-[11px] rounded-md border border-orange-200/70 dark:border-orange-500/25 bg-orange-100/60 dark:bg-orange-500/10 text-orange-800 dark:text-orange-300">
        <Users size={10} class="shrink-0" />
        <span class="font-mono">{filters.user}</span>
        <button class="p-0.5 rounded hover:bg-orange-200/70 dark:hover:bg-orange-500/20" onclick={() => applyFilters({ user: undefined })} aria-label="Remove user filter">
          <X size={10} />
        </button>
      </span>
    {/if}
    {#if filters.table}
      <span class="inline-flex items-center gap-1 pl-1.5 pr-0.5 py-0.5 text-[11px] rounded-md border border-orange-200/70 dark:border-orange-500/25 bg-orange-100/60 dark:bg-orange-500/10 text-orange-800 dark:text-orange-300">
        <Table2 size={10} class="shrink-0" />
        <span class="font-mono">{filters.table}</span>
        <button class="p-0.5 rounded hover:bg-orange-200/70 dark:hover:bg-orange-500/20" onclick={() => applyFilters({ table: undefined })} aria-label="Remove table filter">
          <X size={10} />
        </button>
      </span>
    {/if}

    {#if hasFilters}
      <button
        class="px-1.5 py-0.5 text-[11px] text-gray-500 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-200 rounded hover:bg-gray-200 dark:hover:bg-gray-800"
        onclick={clearFilters}
      >Clear all</button>
    {/if}

    <div class="flex-1"></div>
    <span class="text-[11px] text-gray-400 hidden lg:inline">Tip: use the funnel on Users / Hot tables rows to cross-filter</span>
  </div>

  <div class="flex-1 overflow-auto p-4 space-y-5">
    {#if loading}
      <div class="ds-empty">Loading query insights…</div>
    {:else if error}
      <div class="ds-panel p-6 flex items-start gap-3 text-sm">
        <AlertTriangle size={18} class="text-red-500 shrink-0 mt-0.5" />
        <div>
          <div class="font-semibold text-gray-900 dark:text-gray-100">Couldn't load query insights</div>
          <div class="text-gray-500 mt-1">{error}</div>
          <button class="ds-btn-outline px-2.5 py-1.5 mt-3" onclick={() => loadAll(true)}>Retry</button>
        </div>
      </div>
    {:else if unsupported}
      <div class="ds-panel p-6 flex items-start gap-3 text-sm">
        <AlertTriangle size={18} class="text-amber-500 shrink-0 mt-0.5" />
        <div>
          <div class="font-semibold text-gray-900 dark:text-gray-100">system.query_log is not available</div>
          <div class="text-gray-500 mt-1">
            Query Insights needs ClickHouse's query log. Enable it with
            <code class="px-1 py-0.5 rounded bg-gray-200/70 dark:bg-gray-800 font-mono text-[11px]">&lt;query_log&gt;</code>
            in the server config (it is on by default in most deployments), then run a few queries and refresh.
          </div>
        </div>
      </div>
    {:else}
      <!-- Headline tiles -->
      <div class="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-6 gap-3">
        <div class="ds-stat-card border border-gray-200 dark:border-gray-800">
          <div class="flex items-center gap-2 text-gray-500 text-xs mb-1"><Activity size={14} /> Queries</div>
          <div class="text-2xl font-bold text-gray-900 dark:text-gray-100">{fmtNum(summary?.total_queries)}</div>
          <div class="text-[11px] text-gray-400 mt-0.5">{fmtNum(summary?.active_users)} active users</div>
        </div>
        <div class="ds-stat-card border {failureCount > 0 ? 'border-red-300/70 dark:border-red-800/70' : 'border-gray-200 dark:border-gray-800'}">
          <div class="flex items-center gap-2 text-gray-500 text-xs mb-1"><AlertTriangle size={14} /> Failures</div>
          <div class="text-2xl font-bold {failureCount > 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-900 dark:text-gray-100'}">{fmtNum(failureCount)}</div>
        </div>
        <div class="ds-stat-card border border-gray-200 dark:border-gray-800">
          <div class="flex items-center gap-2 text-gray-500 text-xs mb-1"><Timer size={14} /> p50 latency</div>
          <div class="text-2xl font-bold text-gray-900 dark:text-gray-100">{fmtMs(summary?.p50_ms)}</div>
        </div>
        <div class="ds-stat-card border border-gray-200 dark:border-gray-800">
          <div class="flex items-center gap-2 text-gray-500 text-xs mb-1"><Timer size={14} /> p95 latency</div>
          <div class="text-2xl font-bold text-gray-900 dark:text-gray-100">{fmtMs(summary?.p95_ms)}</div>
          <div class="text-[11px] text-gray-400 mt-0.5">max {fmtMs(summary?.max_ms)}</div>
        </div>
        <div class="ds-stat-card border border-gray-200 dark:border-gray-800">
          <div class="flex items-center gap-2 text-gray-500 text-xs mb-1"><Table2 size={14} /> Data read</div>
          <div class="text-2xl font-bold text-gray-900 dark:text-gray-100">{fmtBytes(summary?.read_bytes)}</div>
        </div>
        <div class="ds-stat-card border border-gray-200 dark:border-gray-800">
          <div class="flex items-center gap-2 text-gray-500 text-xs mb-1"><MemoryStick size={14} /> Peak memory</div>
          <div class="text-2xl font-bold text-gray-900 dark:text-gray-100">{fmtBytes(summary?.peak_memory)}</div>
        </div>
      </div>

      <!-- Trends -->
      <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div class="ds-card p-3">
          <div class="text-xs text-gray-500 mb-2 flex items-center gap-2"><Activity size={13} /> Query volume · {range}</div>
          {#if volumeChart.x.length > 1}
            <TrendChart x={volumeChart.x} series={volumeChart.series} height={150} yLabel="queries" formatY={(v) => formatNumber(Math.round(v))} />
          {:else}
            <div class="h-[150px] grid place-items-center text-xs text-gray-400">Not enough data for this range yet</div>
          {/if}
        </div>
        <div class="ds-card p-3">
          <div class="text-xs text-gray-500 mb-2 flex items-center gap-2"><Timer size={13} /> Latency · {range}</div>
          {#if latencyChart.x.length > 1}
            <TrendChart x={latencyChart.x} series={latencyChart.series} height={150} yLabel="duration" formatY={(v) => formatElapsed(v / 1000)} />
          {:else}
            <div class="h-[150px] grid place-items-center text-xs text-gray-400">Not enough data for this range yet</div>
          {/if}
        </div>
      </div>

      <!-- Drill-down sections -->
      <div>
        <div class="ds-segment flex-wrap mb-3">
          {#each SECTIONS as s}
            <button
              class="ds-segment-btn {activeSection === s.key ? 'ds-segment-btn-active' : ''} inline-flex items-center gap-1.5"
              onclick={() => loadSection(s.key)}
            >
              <s.icon size={13} /> {s.label}
            </button>
          {/each}
        </div>

        {#if sectionLoading}
          <div class="ds-empty">Loading {activeSpec.label.toLowerCase()}…</div>
        {:else if sectionError}
          <div class="ds-panel p-4 text-sm flex items-start gap-2">
            <AlertTriangle size={15} class="text-red-500 shrink-0 mt-0.5" />
            <div>
              <div class="text-gray-700 dark:text-gray-200">Couldn't load {activeSpec.label.toLowerCase()}: <span class="text-gray-500">{sectionError}</span></div>
              <button class="ds-btn-outline px-2.5 py-1 mt-2" onclick={() => loadSection(activeSection)}>Retry</button>
            </div>
          </div>
        {:else if sectionResult && !sectionResult.supported}
          <div class="ds-panel-muted p-4 text-sm text-gray-500 flex items-center gap-2">
            <AlertTriangle size={15} /> {activeSpec.label} is not available on this ClickHouse version.
          </div>
        {:else if sectionResult && sectionResult.data.length === 0}
          <div class="ds-panel-muted p-4 text-sm text-gray-500">
            Nothing recorded for this range.
          </div>
        {:else if sectionResult}
          {#if sectionResult.degraded}
            <div class="mb-2 text-[11px] text-amber-600 dark:text-amber-400 flex items-center gap-1.5">
              <AlertTriangle size={12} /> Cluster-wide query failed — showing the connected node only.
            </div>
          {/if}
          <div class="ds-table-wrap">
            <table class="ds-table">
              <thead>
                <tr class="ds-table-head-row">
                  {#each activeSpec.columns as c}
                    <th class={c.right ? 'ds-table-th-right' : 'ds-table-th'}>{c.label}</th>
                  {/each}
                  {#if activeSpec.openable || activeSpec.filterKey}<th class="ds-table-th-right"></th>{/if}
                </tr>
              </thead>
              <tbody>
                {#each sectionResult.data as row}
                  <tr class="ds-table-row">
                    {#each activeSpec.columns as c}
                      <td class="{c.right ? 'ds-td-right' : c.mono ? 'ds-td-mono' : 'ds-td'} {c.key === 'sample_query' || c.key === 'sample_error' ? 'max-w-md' : ''}">
                        {#if c.key === 'sample_query' || c.key === 'sample_error'}
                          <span class="block truncate font-mono text-[11px]" title={String(row[c.key] ?? '')}>{String(row[c.key] ?? '')}</span>
                        {:else}
                          {c.format ? c.format(row[c.key]) : String(row[c.key] ?? '')}
                        {/if}
                      </td>
                    {/each}
                    {#if activeSpec.openable || activeSpec.filterKey}
                      <td class="ds-td-right">
                        <span class="inline-flex items-center gap-0.5">
                          {#if activeSpec.filterKey}
                            <button
                              class="ds-icon-btn"
                              onclick={() => filterFromRow(activeSpec, row)}
                              title={`Filter the dashboard by this ${activeSpec.filterKey}`}
                              aria-label={`Filter by this ${activeSpec.filterKey}`}
                            >
                              <Funnel size={13} />
                            </button>
                          {/if}
                          {#if activeSpec.openable}
                            <button
                              class="ds-icon-btn"
                              onclick={() => openPattern(row)}
                              title="Open in a new query tab"
                              aria-label="Open query in a new tab"
                            >
                              <ExternalLink size={13} />
                            </button>
                          {/if}
                        </span>
                      </td>
                    {/if}
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
