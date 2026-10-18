import { useEffect, useRef, useState, type ReactNode } from 'react'

// Small dependency-free SVG charts. Colours come from the --viz-* custom
// properties in styles.css (with dark-theme steps); text always uses text
// tokens, never the series colour. Every chart also renders a visually hidden
// table so the numbers are available without the picture.

function useWidth<T extends HTMLElement>() {
  const ref = useRef<T>(null)
  const [width, setWidth] = useState(0)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const observer = new ResizeObserver(entries => setWidth(Math.floor(entries[0].contentRect.width)))
    observer.observe(el)
    setWidth(Math.floor(el.getBoundingClientRect().width))
    return () => observer.disconnect()
  }, [])
  return [ref, width] as const
}

function SrTable({ caption, columns, rows }: { caption: string; columns: string[]; rows: (string | number)[][] }) {
  return <table className="sr-only"><caption>{caption}</caption><thead><tr>{columns.map(c => <th key={c} scope="col">{c}</th>)}</tr></thead>
    <tbody>{rows.map((r, i) => <tr key={i}>{r.map((v, j) => j === 0 ? <th key={j} scope="row">{v}</th> : <td key={j}>{v}</td>)}</tr>)}</tbody></table>
}

function Tooltip({ x, y, width, children }: { x: number; y: number; width: number; children: ReactNode }) {
  const left = Math.min(Math.max(x - 80, 0), Math.max(width - 160, 0))
  return <div role="presentation" className="pointer-events-none absolute z-10 w-40 rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs text-slate-700 shadow-lg" style={{ left, top: Math.max(y - 64, 0) }}>{children}</div>
}

// showLabel thins axis labels to every nth point, always keeps the last one,
// and drops a regular label that would crowd the last.
function showLabel(i: number, count: number, every: number) {
  if (i === count - 1) return true
  return i % every === 0 && count - 1 - i >= Math.ceil(every * 0.75)
}

export type Point = { label: string; value: number | null; detail?: string }

// LineChart plots one percentage series (0–100) with a crosshair tooltip.
export function LineChart({ title, points, format, empty, height = 200 }: { title: string; points: Point[]; format: (v: number) => string; empty: string; height?: number }) {
  const [ref, width] = useWidth<HTMLDivElement>()
  const [hover, setHover] = useState<number | null>(null)
  const valid = points.filter(p => p.value !== null)
  const pad = { l: 36, r: 12, t: 12, b: 28 }
  const w = Math.max(width, 240), plotW = w - pad.l - pad.r, plotH = height - pad.t - pad.b
  const x = (i: number) => pad.l + (points.length <= 1 ? plotW / 2 : (i / (points.length - 1)) * plotW)
  const y = (v: number) => pad.t + plotH - (v / 100) * plotH
  let path = '', open = false
  points.forEach((p, i) => {
    if (p.value === null) { open = false; return }
    path += `${open ? 'L' : 'M'}${x(i).toFixed(1)},${y(p.value).toFixed(1)}`
    open = true
  })
  const labelEvery = Math.max(1, Math.ceil(points.length / Math.max(2, Math.floor(plotW / 72))))
  function move(clientX: number, rect: DOMRect) {
    if (!points.length) return
    const rel = clientX - rect.left - pad.l
    setHover(Math.min(points.length - 1, Math.max(0, Math.round(points.length <= 1 ? 0 : (rel / plotW) * (points.length - 1)))))
  }
  const h = hover !== null ? points[hover] : null
  return <figure className="viz relative min-w-0" ref={ref}>
    <figcaption className="sr-only">{title}</figcaption>
    {valid.length === 0 ? <p className="py-10 text-center text-sm text-slate-500">{empty}</p> : <>
      <svg width={w} height={height} role="img" aria-label={title} className="block max-w-full touch-pan-y"
        onPointerMove={e => move(e.clientX, e.currentTarget.getBoundingClientRect())} onPointerLeave={() => setHover(null)}>
        {[0, 25, 50, 75, 100].map(t => <g key={t}><line x1={pad.l} x2={w - pad.r} y1={y(t)} y2={y(t)} stroke="var(--viz-grid)" strokeWidth={1} />
          <text x={pad.l - 6} y={y(t) + 4} textAnchor="end" className="fill-slate-500 text-[10px] tabular-nums">{t}%</text></g>)}
        {points.map((p, i) => showLabel(i, points.length, labelEvery) ? <text key={i} x={x(i)} y={height - 8} textAnchor={i === 0 ? 'start' : i === points.length - 1 ? 'end' : 'middle'} className="fill-slate-500 text-[10px]">{p.label}</text> : null)}
        <path d={path} fill="none" stroke="var(--viz-1)" strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
        {valid.length === 1 && points.map((p, i) => p.value !== null && <circle key={i} cx={x(i)} cy={y(p.value)} r={4} fill="var(--viz-1)" />)}
        {h && hover !== null && <line x1={x(hover)} x2={x(hover)} y1={pad.t} y2={pad.t + plotH} stroke="var(--viz-axis)" strokeWidth={1} strokeDasharray="3 3" />}
        {h && hover !== null && h.value !== null && <circle cx={x(hover)} cy={y(h.value)} r={5} fill="var(--viz-1)" stroke="var(--viz-surface)" strokeWidth={2} />}
      </svg>
      {h && hover !== null && <Tooltip x={x(hover)} y={h.value !== null ? y(h.value) : pad.t} width={w}><p className="font-semibold text-slate-900">{h.label}</p><p className="tabular-nums">{h.value === null ? '—' : format(h.value)}</p>{h.detail && <p className="text-slate-500">{h.detail}</p>}</Tooltip>}
    </>}
    <SrTable caption={title} columns={['', title]} rows={points.map(p => [p.label, p.value === null ? '—' : format(p.value)])} />
  </figure>
}

export type Series = { key: string; label: string; values: number[] }

// ColumnChart draws one or two series as vertical bars on a zero baseline.
export function ColumnChart({ title, labels, series, format, empty, height = 200 }: { title: string; labels: string[]; series: Series[]; format: (v: number) => string; empty: string; height?: number }) {
  const [ref, width] = useWidth<HTMLDivElement>()
  const [hover, setHover] = useState<number | null>(null)
  const max = Math.max(0, ...series.flatMap(s => s.values))
  const pad = { l: 8, r: 8, t: 12, b: 28 }
  const w = Math.max(width, 240), plotW = w - pad.l - pad.r, plotH = height - pad.t - pad.b
  const band = labels.length ? plotW / labels.length : plotW
  const gap = 2, inner = Math.max(4, Math.min(48, band * 0.7))
  const barW = Math.max(3, (inner - gap * (series.length - 1)) / series.length)
  const y = (v: number) => pad.t + plotH - (max ? (v / max) * plotH : 0)
  const labelEvery = Math.max(1, Math.ceil(labels.length / Math.max(2, Math.floor(plotW / 56))))
  const colours = ['var(--viz-1)', 'var(--viz-2)']
  return <figure className="viz relative min-w-0" ref={ref}>
    <figcaption className="sr-only">{title}</figcaption>
    {series.length > 1 && <ul className="mb-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-600" aria-hidden="true">
      {series.map((s, i) => <li key={s.key} className="flex items-center gap-1.5"><span className="h-2.5 w-2.5 rounded-sm" style={{ background: colours[i] }} />{s.label}</li>)}</ul>}
    {max === 0 ? <p className="py-10 text-center text-sm text-slate-500">{empty}</p> :
      <svg width={w} height={height} role="img" aria-label={title} className="block max-w-full" onPointerLeave={() => setHover(null)}>
        {[0, 0.5, 1].map(f => <line key={f} x1={pad.l} x2={w - pad.r} y1={y(max * f)} y2={y(max * f)} stroke="var(--viz-grid)" strokeWidth={1} />)}
        {labels.map((label, i) => {
          const start = pad.l + band * i + (band - inner) / 2
          return <g key={label + i} onPointerEnter={() => setHover(i)}>
            <rect x={pad.l + band * i} y={pad.t} width={band} height={plotH} fill="transparent" />
            {series.map((s, j) => {
              const v = s.values[i] ?? 0, top = y(v), hgt = Math.max(0, pad.t + plotH - top)
              const r = Math.min(4, barW / 2, hgt)
              const bx = start + j * (barW + gap)
              return hgt > 0 && <path key={s.key} fill={colours[j]} opacity={hover === null || hover === i ? 1 : 0.55}
                d={`M${bx},${pad.t + plotH}V${top + r}Q${bx},${top} ${bx + r},${top}H${bx + barW - r}Q${bx + barW},${top} ${bx + barW},${top + r}V${pad.t + plotH}Z`} />
            })}
            {showLabel(i, labels.length, labelEvery) && <text x={pad.l + band * i + band / 2} y={height - 8} textAnchor="middle" className="fill-slate-500 text-[10px]">{label}</text>}
          </g>
        })}
        <line x1={pad.l} x2={w - pad.r} y1={pad.t + plotH} y2={pad.t + plotH} stroke="var(--viz-axis)" strokeWidth={1} />
      </svg>}
    {hover !== null && max > 0 && <Tooltip x={pad.l + band * hover + band / 2} y={pad.t + 24} width={w}><p className="font-semibold text-slate-900">{labels[hover]}</p>
      {series.map((s, i) => <p key={s.key} className="flex items-center gap-1.5 tabular-nums"><span aria-hidden="true" className="h-2 w-2 rounded-sm" style={{ background: colours[i] }} />{s.label}: {format(s.values[hover] ?? 0)}</p>)}</Tooltip>}
    <SrTable caption={title} columns={['', ...series.map(s => s.label)]} rows={labels.map((l, i) => [l, ...series.map(s => format(s.values[i] ?? 0))])} />
  </figure>
}

export type BarItem = { key: string; label: ReactNode; value: number | null; display: string; hint?: string; href?: string }

// BarList is a labelled horizontal bar per row: readable on phones, with the
// value printed beside each bar so colour is never needed to read it.
export function BarList({ title, items, max, empty }: { title: string; items: BarItem[]; max?: number; empty: string }) {
  const top = max ?? Math.max(0, ...items.map(i => i.value ?? 0))
  if (!items.length) return <p className="py-6 text-center text-sm text-slate-500">{empty}</p>
  return <ul className="viz space-y-2.5" aria-label={title}>
    {items.map(item => <li key={item.key} title={item.hint}>
      <div className="flex items-baseline justify-between gap-3 text-sm"><span className="min-w-0 truncate text-slate-700">{item.label}</span><span className="shrink-0 font-semibold tabular-nums text-slate-900">{item.display}</span></div>
      <div className="mt-1 h-2 overflow-hidden rounded-full" style={{ background: 'var(--viz-track)' }}>
        <div className="h-full rounded-full" style={{ width: `${top && item.value ? Math.max(1.5, (item.value / top) * 100) : 0}%`, background: 'var(--viz-1)' }} /></div>
      {item.hint && <p className="mt-0.5 text-xs text-slate-500">{item.hint}</p>}
    </li>)}
  </ul>
}

// Meter is a single 0–100% bar for a headline rate.
export function Meter({ value, label }: { value: number | null; label: string }) {
  return <div className="mt-2 h-1.5 overflow-hidden rounded-full" style={{ background: 'var(--viz-track)' }} role="img" aria-label={label}>
    <div className="h-full rounded-full" style={{ width: `${value ?? 0}%`, background: 'var(--viz-1)' }} /></div>
}
