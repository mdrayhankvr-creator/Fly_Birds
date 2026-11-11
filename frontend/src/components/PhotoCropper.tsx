import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { useTranslation } from 'react-i18next'

// PhotoCropper: crops a photo to a square face shot in the browser. Drag the
// photo (or use the arrow keys) to place the face inside the oval, zoom with
// the slider (or + and -), then the square is resized to OUTPUT pixels and
// compressed as JPEG. The server crops and re-encodes again; this keeps the
// upload small on slow connections and lets the student choose the framing.

const OUTPUT = 600
const VIEW = 280 // the on-screen frame, CSS pixels

type Box = { zoom: number; cx: number; cy: number }

function clamp(box: Box, w: number, h: number): Box {
  const side = Math.min(w, h) / box.zoom
  const half = side / 2
  return { zoom: box.zoom, cx: Math.min(Math.max(box.cx, half), w - half), cy: Math.min(Math.max(box.cy, half), h - half) }
}

export async function cropToBlob(bitmap: ImageBitmap, box: Box, size = OUTPUT): Promise<Blob> {
  const side = Math.min(bitmap.width, bitmap.height) / box.zoom
  const canvas = document.createElement('canvas')
  canvas.width = size
  canvas.height = size
  const context = canvas.getContext('2d')
  if (!context) throw new Error('canvas unavailable')
  context.fillStyle = '#ffffff'
  context.fillRect(0, 0, size, size)
  context.imageSmoothingQuality = 'high'
  context.drawImage(bitmap, box.cx - side / 2, box.cy - side / 2, side, side, 0, 0, size, size)
  const blob = await new Promise<Blob | null>(resolve => canvas.toBlob(resolve, 'image/jpeg', 0.85))
  if (!blob) throw new Error('encode failed')
  return blob
}

export function PhotoCropper({ file, busy, onCancel, onDone }: { file: File; busy: boolean; onCancel: () => void; onDone: (image: Blob) => void }) {
  const { t } = useTranslation()
  const canvas = useRef<HTMLCanvasElement>(null)
  const [bitmap, setBitmap] = useState<ImageBitmap | null>(null)
  const [failed, setFailed] = useState(false)
  const [box, setBox] = useState<Box>({ zoom: 1, cx: 0, cy: 0 })
  const drag = useRef<{ x: number; y: number; box: Box } | null>(null)

  useEffect(() => {
    let live = true
    let made: ImageBitmap | null = null
    createImageBitmap(file, { imageOrientation: 'from-image' }).then(b => {
      made = b
      if (!live) { b.close(); return }
      setBitmap(b); setBox({ zoom: 1, cx: b.width / 2, cy: b.height / 2 })
    }).catch(() => { if (live) setFailed(true) })
    return () => { live = false; made?.close() }
  }, [file])

  useEffect(() => {
    const c = canvas.current
    if (!c || !bitmap) return
    const ratio = window.devicePixelRatio || 1
    c.width = VIEW * ratio; c.height = VIEW * ratio
    const context = c.getContext('2d')
    if (!context) return
    const side = Math.min(bitmap.width, bitmap.height) / box.zoom
    context.fillStyle = '#ffffff'
    context.fillRect(0, 0, c.width, c.height)
    context.drawImage(bitmap, box.cx - side / 2, box.cy - side / 2, side, side, 0, 0, c.width, c.height)
  }, [bitmap, box])

  if (failed) return <p role="alert" className="text-sm font-semibold text-rose-700">{t('uxPhotoUnreadable')}</p>
  if (!bitmap) return <p className="text-sm text-slate-600">{t('uxPhotoLoading')}</p>
  const b = bitmap
  const perPixel = Math.min(b.width, b.height) / box.zoom / VIEW // source pixels per frame pixel
  const set = (next: Box) => setBox(clamp(next, b.width, b.height))
  const onPointerDown = (e: PointerEvent<HTMLDivElement>) => { e.currentTarget.setPointerCapture(e.pointerId); drag.current = { x: e.clientX, y: e.clientY, box } }
  const onPointerMove = (e: PointerEvent<HTMLDivElement>) => {
    const d = drag.current
    if (!d) return
    const scale = e.currentTarget.getBoundingClientRect().width / VIEW
    set({ ...d.box, cx: d.box.cx - (e.clientX - d.x) * perPixel / scale, cy: d.box.cy - (e.clientY - d.y) * perPixel / scale })
  }
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const step = 12 * perPixel
    const moves: Record<string, [number, number]> = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] }
    if (moves[e.key]) { e.preventDefault(); set({ ...box, cx: box.cx + moves[e.key][0], cy: box.cy + moves[e.key][1] }) }
    else if (e.key === '+' || e.key === '=') { e.preventDefault(); set({ ...box, zoom: Math.min(4, box.zoom + 0.2) }) }
    else if (e.key === '-') { e.preventDefault(); set({ ...box, zoom: Math.max(1, box.zoom - 0.2) }) }
  }
  return <div className="grid grid-cols-1 gap-4 sm:grid-cols-[auto_1fr] sm:items-start" data-photo-cropper>
    <div className="mx-auto w-full max-w-[280px]">
      <div role="application" tabIndex={0} aria-label={t('uxPhotoFrame')} onKeyDown={onKey} onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={() => { drag.current = null }} onPointerCancel={() => { drag.current = null }}
        className="relative aspect-square w-full cursor-grab touch-none select-none overflow-hidden rounded-2xl bg-slate-100 ring-1 ring-slate-300 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)] active:cursor-grabbing">
        <canvas ref={canvas} className="block h-full w-full" />
        <svg aria-hidden="true" viewBox="0 0 100 100" className="pointer-events-none absolute inset-0 h-full w-full"><ellipse cx="50" cy="44" rx="25" ry="32" fill="none" stroke="#ffffff" strokeWidth="1" strokeDasharray="3 2" opacity="0.9" /></svg>
      </div>
      <label className="mt-3 block text-xs font-semibold text-slate-700">{t('uxPhotoZoom')}
        <input type="range" min={1} max={4} step={0.05} value={box.zoom} onChange={e => set({ ...box, zoom: Number(e.target.value) })} className="mt-1 block w-full accent-[var(--brand-primary)]" /></label>
      <p className="mt-1 text-xs text-slate-600">{t('uxPhotoDragHint')}</p>
    </div>
    <div className="min-w-0 space-y-3">
      <PhotoGuidance />
      <div className="flex flex-wrap gap-2">
        <button type="button" className="btn btn-primary" disabled={busy} onClick={() => void cropToBlob(b, box).then(onDone)} data-photo-submit>{t('uxPhotoSubmit')}</button>
        <button type="button" className="btn btn-secondary" disabled={busy} onClick={onCancel}>{t('cancel')}</button>
      </div>
    </div>
  </div>
}

// PhotoGuidance: what makes a usable ID photo.
export function PhotoGuidance() {
  const { t } = useTranslation()
  return <div className="rounded-xl border border-sky-200 bg-sky-50 p-3 text-sm text-sky-950">
    <p className="font-bold">{t('uxPhotoGuideTitle')}</p>
    <ul className="mt-1 list-disc space-y-0.5 ps-5">
      {['uxPhotoGuide1', 'uxPhotoGuide2', 'uxPhotoGuide3', 'uxPhotoGuide4', 'uxPhotoGuide5'].map(k => <li key={k}>{t(k)}</li>)}
    </ul>
  </div>
}
