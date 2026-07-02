<script lang="ts">
  import { onMount } from 'svelte'
  import uPlot from 'uplot'
  import 'uplot/dist/uPlot.min.css'
  import { getTheme } from '../../stores/theme.svelte'

  export interface TrendSeries {
    label: string
    /** Aligned to x; null renders as a gap in the line. */
    values: (number | null)[]
    color: string
    fill?: string
  }

  interface Props {
    /** Unix timestamps (seconds). */
    x: number[]
    series: TrendSeries[]
    height?: number
    /** Y-axis title, e.g. "queries" or "ms". */
    yLabel?: string
    /** Y value formatter for axis ticks and the hover legend. */
    formatY?: (v: number) => string
  }

  let { x, series, height = 150, yLabel = '', formatY = (v) => String(v) }: Props = $props()

  let container: HTMLDivElement
  let chart: uPlot | null = null

  function destroyChart() {
    if (chart) {
      chart.destroy()
      chart = null
    }
  }

  function themeColors() {
    const dark = getTheme() === 'dark'
    return {
      axis: dark ? '#9ca3af' : '#6b7280',
      grid: dark ? 'rgba(156,163,175,0.12)' : 'rgba(107,114,128,0.14)',
    }
  }

  function draw() {
    destroyChart()
    if (!container) return
    if (!x || x.length === 0 || series.length === 0) return

    const { axis, grid } = themeColors()
    const fmt = formatY

    const opts: uPlot.Options = {
      width: container.clientWidth || 480,
      height,
      // The legend doubles as the hover tooltip: it shows each series' value
      // at the cursor position.
      legend: { show: true, live: true },
      cursor: {
        points: { show: true, size: 6 },
        x: true,
        y: false,
      },
      scales: {
        // x values are unix seconds; time:true gives date/time tick labels.
        x: { time: true },
      },
      axes: [
        {
          stroke: axis,
          grid: { show: true, stroke: grid, width: 1 },
          ticks: { show: false },
          font: '10px var(--font-sans)',
          space: 80,
        },
        {
          label: yLabel || undefined,
          labelFont: '10px var(--font-sans)',
          labelSize: yLabel ? 14 : 0,
          stroke: axis,
          grid: { show: true, stroke: grid, width: 1 },
          ticks: { show: false },
          font: '10px var(--font-sans)',
          size: 56,
          values: (_u, ticks) => ticks.map((v) => fmt(v)),
        },
      ],
      series: [
        {
          label: 'Time',
          value: (_u, v) => (v == null ? '—' : new Date(v * 1000).toLocaleString()),
        },
        ...series.map((s) => ({
          label: s.label,
          stroke: s.color,
          width: 2,
          fill: s.fill,
          points: { show: false },
          value: (_u: uPlot, v: number | null) => (v == null ? '—' : fmt(v)),
        })),
      ],
      padding: [8, 8, 0, 0],
    }

    chart = new uPlot(opts, [x, ...series.map((s) => s.values)], container)
  }

  onMount(() => {
    draw()
    const observer = new ResizeObserver(() => {
      if (chart && container) chart.setSize({ width: container.clientWidth, height })
    })
    observer.observe(container)
    return () => {
      observer.disconnect()
      destroyChart()
    }
  })

  $effect(() => {
    x
    series
    getTheme()
    draw()
  })
</script>

<div bind:this={container} class="w-full trend-chart" style="min-height:{height}px"></div>

<style>
  /* Style uplot's built-in live legend as a compact inline readout. */
  .trend-chart :global(.u-legend) {
    font-size: 11px;
    text-align: left;
    padding-top: 4px;
  }
  .trend-chart :global(.u-legend .u-marker) {
    width: 8px;
    height: 8px;
  }
  .trend-chart :global(.u-legend th) {
    font-weight: 500;
    color: rgb(107 114 128);
  }
  .trend-chart :global(.u-legend .u-value) {
    color: rgb(107 114 128);
    font-variant-numeric: tabular-nums;
  }
  :global(.dark) .trend-chart :global(.u-legend th),
  :global(.dark) .trend-chart :global(.u-legend .u-value) {
    color: rgb(156 163 175);
  }
</style>
