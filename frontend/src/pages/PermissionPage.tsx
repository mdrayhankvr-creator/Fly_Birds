import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { getPermissionHistory, getPermissionMatrix, saveRolePermissions, type MatrixPermission } from '../api/admin'
import { EmptyState, Notice, PageHeader, Pager, Skeleton, Tabs, ui } from './attendanceUi'
import { useApi, useDhakaFormat } from './commShared'

type View = 'edit' | 'matrix' | 'history'

function groupPermissions(items: MatrixPermission[], search: string) {
  const needle = search.trim().toLowerCase()
  const groups = new Map<string, MatrixPermission[]>()
  for (const p of items) {
    if (needle && !p.id.toLowerCase().includes(needle) && !p.description.toLowerCase().includes(needle)) continue
    groups.set(p.group, [...(groups.get(p.group) ?? []), p])
  }
  return [...groups.entries()].sort(([a], [b]) => a.localeCompare(b))
}

export function PermissionPage({ canManage }: { canManage: boolean }) {
  const { t } = useTranslation()
  const api = useApi()
  const f = useDhakaFormat()
  const client = useQueryClient()
  const [view, setView] = useState<View>('edit')
  const [role, setRole] = useState('admin')
  const [search, setSearch] = useState('')
  const [draft, setDraft] = useState<Set<string>>(new Set())
  const [reviewing, setReviewing] = useState(false)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string; stale?: boolean } | null>(null)
  const [historyPage, setHistoryPage] = useState(1)
  const matrix = useQuery({ queryKey: ['rbac', 'matrix'], queryFn: () => api(getPermissionMatrix), retry: false })
  const history = useQuery({ queryKey: ['rbac', 'history', role, historyPage], queryFn: () => api(token => getPermissionHistory(token, role, historyPage)), enabled: view === 'history', retry: false })
  const base = useMemo(() => matrix.data?.grants[role] ?? [], [matrix.data, role])
  const protectedIds = matrix.data?.protected[role] ?? []
  useEffect(() => { setDraft(new Set(base)); setReviewing(false) }, [base])
  useEffect(() => { setHistoryPage(1) }, [role])
  const added = [...draft].filter(id => !base.includes(id)).sort()
  const removed = base.filter(id => !draft.has(id)).sort()
  const dirty = added.length + removed.length > 0
  const roleName = (id: string) => t(`m10Role_${id}`, { defaultValue: matrix.data?.roles.find(r => r.id === id)?.name ?? id })
  const groupName = (g: string) => t(`m10Group_${g}`, { defaultValue: g })
  const describe = (id: string) => matrix.data?.permissions.find(p => p.id === id)?.description ?? id

  const save = useMutation({
    mutationFn: () => api(token => saveRolePermissions(token, role, [...draft].sort(), base)),
    onMutate: () => setMessage(null),
    onSuccess: async () => {
      setMessage({ tone: 'success', text: t('m10Saved', { role: roleName(role) }) })
      await Promise.all([client.invalidateQueries({ queryKey: ['rbac'] }), client.invalidateQueries({ queryKey: ['my-permissions'] })])
    },
    onError: cause => {
      setReviewing(false)
      const code = cause instanceof ApiError ? cause.code : ''
      setMessage({ tone: 'error', stale: code === 'permissions_changed', text: t(code === 'permissions_changed' ? 'm10Stale' : code === 'last_admin' ? 'm10LastAdmin' : cause instanceof ApiError && cause.status === 403 ? 'm10ErrForbidden' : 'm10SaveError') })
    },
  })

  function toggle(id: string, on: boolean) {
    if (protectedIds.includes(id) && !on) return
    setMessage(null)
    setDraft(current => { const next = new Set(current); if (on) next.add(id); else next.delete(id); return next })
  }
  function selectRole(next: string) {
    setRole(next); setMessage(null); setSearch('')
  }
  const groups = matrix.data ? groupPermissions(matrix.data.permissions, search) : []

  return <section className="space-y-6">
    <PageHeader eyebrow={t('m10Eyebrow')} title={t('m10PermTitle')} description={t('m10PermIntro')} />
    {!canManage && <Notice tone="info">{t('m10ReadOnlyPerms')}</Notice>}
    {matrix.isPending && <Skeleton rows={5} />}
    {matrix.isError && <Notice tone="error" action={<button type="button" className={ui.ghost} onClick={() => void matrix.refetch()}>{t('m10Retry')}</button>}>{t(matrix.error instanceof ApiError && matrix.error.status === 403 ? 'm10ErrForbidden' : 'm10ErrGeneric')}</Notice>}
    {matrix.data && <>
      <Tabs label={t('m10PermTitle')} value={view} onChange={setView} items={[{ value: 'edit', label: t('m10TabEdit') }, { value: 'matrix', label: t('m10TabMatrix') }, { value: 'history', label: t('m10TabHistory') }]} />
      {view !== 'matrix' && <div role="radiogroup" aria-label={t('roles')} className="flex flex-wrap gap-2">
        {matrix.data.roles.map(r => <button key={r.id} type="button" role="radio" aria-checked={role === r.id} onClick={() => selectRole(r.id)}
          className={`rounded-xl px-3.5 py-2 text-left text-sm transition focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] ${role === r.id ? 'bg-[var(--brand-primary)] text-[var(--brand-on)] shadow-sm' : 'bg-white text-slate-700 ring-1 ring-slate-200 hover:bg-slate-50'}`}>
          <span className="block font-semibold">{roleName(r.id)}</span>
          <span className={`block text-xs ${role === r.id ? 'text-white/85' : 'text-slate-500'}`}>{t('m10ActiveUsers', { count: r.active_users, n: f.number(r.active_users) })} · {f.number(matrix.data.grants[r.id]?.length ?? 0)}</span></button>)}
      </div>}
      {message && <Notice tone={message.tone} action={message.stale ? <button type="button" className={ui.secondary} onClick={() => { setMessage(null); void matrix.refetch() }}>{t('m10Reload')}</button> : undefined}>{message.text}</Notice>}

      {view === 'edit' && <div className="space-y-4">
        <div className={`${ui.card} sticky top-2 z-10 flex flex-wrap items-center gap-3 p-3`}>
          <label className="min-w-[12rem] flex-1"><span className="sr-only">{t('m10SearchPermissions')}</span>
            <input type="search" className={`${ui.field} mt-0`} placeholder={t('m10SearchPermissions')} value={search} onChange={e => setSearch(e.target.value)} /></label>
          <span className="text-sm tabular-nums text-slate-600" aria-live="polite">{dirty ? t('m10Unsaved', { added: f.number(added.length), removed: f.number(removed.length) }) : t('m10NoChanges')}</span>
          {canManage && <div className="flex gap-2">
            <button type="button" className={ui.secondary} disabled={!dirty || save.isPending} onClick={() => { setDraft(new Set(base)); setReviewing(false); setMessage(null) }}>{t('m10Discard')}</button>
            <button type="button" className={ui.primary} disabled={!dirty || save.isPending} onClick={() => setReviewing(true)}>{t('m10Review')}</button></div>}
        </div>
        {reviewing && dirty && <div role="alertdialog" aria-labelledby="perm-review-title" className="rounded-2xl border border-amber-300 bg-amber-50 p-4 text-amber-950">
          <h2 id="perm-review-title" className="font-bold">{t('m10ReviewTitle', { role: roleName(role) })}</h2>
          <div className="mt-2 grid gap-3 sm:grid-cols-2">
            {added.length > 0 && <div><p className="text-sm font-semibold text-emerald-800">{t('m10Adding')}</p><ul className="mt-1 space-y-0.5 text-sm">{added.map(id => <li key={id}><code className="font-mono text-xs">+ {id}</code> <span className="text-slate-600">{describe(id)}</span></li>)}</ul></div>}
            {removed.length > 0 && <div><p className="text-sm font-semibold text-rose-800">{t('m10Removing')}</p><ul className="mt-1 space-y-0.5 text-sm">{removed.map(id => <li key={id}><code className="font-mono text-xs">− {id}</code> <span className="text-slate-600">{describe(id)}</span></li>)}</ul></div>}
          </div>
          <div className="mt-3 flex flex-wrap gap-2">
            <button type="button" className={ui.primary} disabled={save.isPending} onClick={() => save.mutate()}>{save.isPending ? t('m10Saving') : t('m10SaveNow')}</button>
            <button type="button" className={ui.secondary} onClick={() => setReviewing(false)}>{t('m10Cancel')}</button></div>
        </div>}
        {groups.length === 0 && <EmptyState title={t('m10NoMatch')} />}
        <div className="grid gap-4 lg:grid-cols-2">{groups.map(([group, items]) => {
          const granted = items.filter(p => draft.has(p.id)).length
          return <fieldset key={group} className={`${ui.card} p-4`}>
            <legend className="sr-only">{groupName(group)}</legend>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h2 className="font-bold text-slate-900">{groupName(group)}</h2>
              <span className="text-xs tabular-nums text-slate-500">{t('m10Granted', { granted: f.number(granted), total: f.number(items.length) })}</span>
              {canManage && <span className="flex gap-1">
                <button type="button" className={ui.ghost} onClick={() => items.forEach(p => toggle(p.id, true))}>{t('m10SelectAll')}</button>
                <button type="button" className={ui.ghost} onClick={() => items.forEach(p => toggle(p.id, false))}>{t('m10ClearGroup')}</button></span>}
            </div>
            <ul className="mt-2 space-y-1">{items.map(p => {
              const locked = protectedIds.includes(p.id)
              const changed = draft.has(p.id) !== base.includes(p.id)
              return <li key={p.id}><label className={`flex cursor-pointer items-start gap-3 rounded-lg px-2 py-1.5 ${changed ? 'bg-amber-50' : 'hover:bg-slate-50'}`}>
                <input type="checkbox" className="mt-1 h-4 w-4 accent-[var(--brand-primary)]" checked={draft.has(p.id)} disabled={!canManage || locked || save.isPending} onChange={e => toggle(p.id, e.target.checked)} />
                <span className="min-w-0"><code className="break-all font-mono text-sm text-slate-900">{p.id}</code>
                  {locked && <span title={t('m10ProtectedHint')} className="ml-2 rounded bg-slate-100 px-1.5 py-0.5 text-xs font-semibold text-slate-700">🔒 {t('m10Protected')}</span>}
                  <span className="block text-sm text-slate-600">{p.description}</span></span>
              </label></li>
            })}</ul>
          </fieldset>
        })}</div>
      </div>}

      {view === 'matrix' && <div className={`${ui.card} overflow-x-auto`}>
        <table className="w-full min-w-[44rem] text-left text-sm">
          <caption className="sr-only">{t('m10TabMatrix')}</caption>
          <thead className="sticky top-0 bg-white"><tr className="border-b border-slate-200">
            <th scope="col" className="p-3 font-semibold text-slate-700">{t('m10Permission')}</th>
            {matrix.data.roles.map(r => <th key={r.id} scope="col" className="p-3 text-center font-semibold text-slate-700">{roleName(r.id)}</th>)}</tr></thead>
          {groupPermissions(matrix.data.permissions, '').map(([group, items]) => <tbody key={group}>
            <tr><th colSpan={matrix.data.roles.length + 1} scope="colgroup" className="bg-slate-50 px-3 py-2 text-xs font-bold tracking-wide text-slate-600">{groupName(group)}</th></tr>
            {items.map(p => <tr key={p.id} className="border-b border-slate-100">
              <th scope="row" className="p-2 pl-3 font-normal"><code className="font-mono text-xs text-slate-900">{p.id}</code></th>
              {matrix.data.roles.map(r => {
                const has = matrix.data.grants[r.id]?.includes(p.id)
                return <td key={r.id} className="p-2 text-center">{has ? <span className="font-bold text-emerald-700" aria-label={`${roleName(r.id)}: ${p.id}`}>✓</span> : <span className="text-slate-300" aria-hidden="true">·</span>}</td>
              })}</tr>)}
          </tbody>)}
        </table>
      </div>}

      {view === 'history' && <div className="space-y-3">
        {history.isPending && <Skeleton rows={3} />}
        {history.isError && <Notice tone="error">{t('m10ErrGeneric')}</Notice>}
        {history.data && (history.data.items.length === 0 ? <EmptyState title={t('m10NoHistory')} /> : <>
          <ol className={`${ui.card} divide-y divide-slate-100`}>{history.data.items.map(h => <li key={h.id} className="px-4 py-3">
            <p className="text-sm"><span className="font-semibold text-slate-900">{h.actor_name || t('m10System')}</span> <span className="text-slate-500">· {f.dateTime(h.created_at)}</span></p>
            <div className="mt-1 flex flex-wrap gap-1">
              {h.added.map(id => <code key={'+' + id} className="rounded bg-emerald-50 px-1.5 py-0.5 font-mono text-xs text-emerald-800">+ {id}</code>)}
              {h.removed.map(id => <code key={'-' + id} className="rounded bg-rose-50 px-1.5 py-0.5 font-mono text-xs text-rose-800">− {id}</code>)}
              {h.added.length + h.removed.length === 0 && <span className="text-sm text-slate-500">{t('m10None')}</span>}
            </div></li>)}</ol>
          <Pager page={historyPage} total={history.data.total} size={10} onPage={setHistoryPage} previous={t('m10Previous')} next={t('m10Next')} label={t('m10TabHistory')} />
        </>)}
      </div>}
    </>}
  </section>
}
