import type { KeyboardEvent, ReactNode } from 'react'
import { Icon, type IconName } from '../components/icons'

// Shared pieces of the student and staff profiles (checkpoint 4).

const initials = (name: string) => name.split(/\s+/).filter(Boolean).slice(0, 2).map(part => [...part][0]).join('').toUpperCase()

// ProfilePhoto is the large portrait in a profile header: the photo when
// there is one, otherwise the initials on the module's colour.
export function ProfilePhoto({ url, name, size = 'h-28 w-28 sm:h-36 sm:w-36' }: { url?: string; name: string; size?: string }) {
  if (url) return <img src={url} alt="" className={`${size} shrink-0 rounded-full object-cover shadow-md ring-4 ring-white`} />
  return <span aria-hidden="true" className={`${size} ds-fill relative grid shrink-0 place-items-center overflow-hidden rounded-full text-3xl font-extrabold tracking-tight shadow-md ring-4 ring-white sm:text-4xl`}>
    <Icon name="user" className="absolute -bottom-3 -right-3 h-20 w-20 opacity-15" />
    <span className="relative">{initials(name) || '?'}</span>
  </span>
}

// Fact is one labelled value in a profile header or details list.
export function Fact({ label, children, icon }: { label: ReactNode; children: ReactNode; icon?: IconName }) {
  return <div className="min-w-0">
    <dt className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-slate-500">{icon && <Icon name={icon} className="h-3.5 w-3.5" />}{label}</dt>
    <dd className="mt-0.5 font-semibold text-slate-900" style={{ overflowWrap: 'anywhere' }}>{children}</dd>
  </div>
}

// Chip is a small rounded label, for subjects, classes and audiences.
export function Chip({ children }: { children: ReactNode }) {
  return <span className="ds-soft ds-ink inline-flex items-center rounded-full px-2.5 py-1 text-xs font-semibold">{children}</span>
}

export function ProfileTabs({ items, active, onChange, prefix, label }: { items: { id: string; label: string }[]; active: string; onChange: (id: string) => void; prefix: string; label: string }) {
  function key(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    let next = index
    if (event.key === 'ArrowRight') next = (index + 1) % items.length
    else if (event.key === 'ArrowLeft') next = (index + items.length - 1) % items.length
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = items.length - 1
    else return
    event.preventDefault()
    onChange(items[next].id)
    document.getElementById(`${prefix}-tab-${items[next].id}`)?.focus()
  }
  return <nav aria-label={label} className="pr-tabs overflow-x-auto" role="tablist">
    {items.map((item, index) => <button key={item.id} id={`${prefix}-tab-${item.id}`} type="button" role="tab" aria-selected={active === item.id}
      aria-controls={active === item.id ? `${prefix}-panel-${item.id}` : undefined} tabIndex={active === item.id ? 0 : -1} onClick={() => onChange(item.id)} onKeyDown={event => key(event, index)}
      className="pr-tab shrink-0">{item.label}</button>)}
  </nav>
}

// minutes → "09:30" (Bangla digits in Bangla).
export function clock(minutes: number, bn: boolean) {
  const text = `${String(Math.floor(minutes / 60)).padStart(2, '0')}:${String(minutes % 60).padStart(2, '0')}`
  return bn ? text.replace(/[0-9]/g, d => '০১২৩৪৫৬৭৮৯'[Number(d)]) : text
}
