import { useEffect, type RefObject } from 'react'

// Tables marked ds-cards turn into cards on phones (styles.css). This copies
// each column heading onto its cells as data-label, and sets table roles
// explicitly, because changing a table's display drops its semantics.
function label(table: HTMLTableElement) {
  table.setAttribute('role', 'table')
  const head = table.tHead?.rows[table.tHead.rows.length - 1]
  const names: string[] = []
  if (head) for (const cell of Array.from(head.cells)) {
    for (let i = 0; i < Math.max(1, cell.colSpan); i++) names.push(cell.colSpan > 1 ? '' : (cell.textContent ?? '').trim())
    if (cell.getAttribute('role') !== 'columnheader') cell.setAttribute('role', 'columnheader')
  }
  for (const section of [table.tHead, ...Array.from(table.tBodies), table.tFoot]) {
    if (!section) continue
    if (section.getAttribute('role') !== 'rowgroup') section.setAttribute('role', 'rowgroup')
    for (const row of Array.from(section.rows)) {
      if (row.getAttribute('role') !== 'row') row.setAttribute('role', 'row')
      if (section === table.tHead) continue
      let col = 0
      for (const cell of Array.from(row.cells)) {
        const name = cell.colSpan > 1 ? '' : names[col] ?? ''
        if (cell.dataset.label !== name) cell.dataset.label = name
        const role = cell.tagName === 'TH' ? 'rowheader' : 'cell'
        if (cell.getAttribute('role') !== role) cell.setAttribute('role', role)
        col += Math.max(1, cell.colSpan)
      }
    }
  }
}

export function useCardTables(root: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const el = root.current
    if (!el) return
    let frame = 0
    const run = () => { frame = 0; el.querySelectorAll<HTMLTableElement>('table.ds-cards').forEach(label) }
    const schedule = () => { if (!frame) frame = requestAnimationFrame(run) }
    run()
    // Attribute changes are not observed, so labelling never re-triggers itself.
    const observer = new MutationObserver(schedule)
    observer.observe(el, { childList: true, subtree: true, characterData: true })
    return () => { observer.disconnect(); if (frame) cancelAnimationFrame(frame) }
  }, [root])
}
