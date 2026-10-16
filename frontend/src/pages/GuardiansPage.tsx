import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, deleteGuardian, getGuardianStudents, getGuardians, getUsers, saveGuardian, type Guardian, type GuardianInput } from '../api/client'
import { peopleField, usePeopleApi } from './peopleShared'

const blank: GuardianInput = { name_en: '', name_bn: '', phone: '', email: '', address: '', user_id: '' }

export function GuardiansPage({ canManage }: { canManage: boolean }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<Guardian | null>(null)
  const [editing, setEditing] = useState('')
  const [form, setForm] = useState<GuardianInput>(blank)
  const [accountSearch, setAccountSearch] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const params = new URLSearchParams({ page: String(page), page_size: '10', search })
  const listing = useQuery({ queryKey: ['people', 'guardians', params.toString()], queryFn: () => authorized(token => getGuardians(token, params)), retry: false })
  const children = useQuery({ queryKey: ['people', 'guardian-students', selected?.id], enabled: !!selected, queryFn: () => authorized(token => getGuardianStudents(token, selected!.id)), retry: false })
  const accounts = useQuery({ queryKey: ['people', 'guardian-accounts', accountSearch], enabled: canManage, queryFn: () => authorized(token => getUsers(token, new URLSearchParams({ role: 'guardian', search: accountSearch, page_size: '100' }))), retry: false })
  function edit(item: Guardian) { setSelected(item); setEditing(item.id); setForm({ name_en: item.name_en, name_bn: item.name_bn, phone: item.phone, email: item.email, address: item.address, user_id: item.user_id ?? '' }); setError(''); setMessage('') }
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError(''); setMessage('')
    try { const value = await authorized(token => saveGuardian(token, form, editing || undefined)); setSelected(value); setEditing(''); setForm(blank); await cache.invalidateQueries({ queryKey: ['people', 'guardians'] }); setMessage(t('peopleSaved')) }
    catch (cause) { setError(cause instanceof ApiError && cause.status === 409 ? t('peopleDuplicate') : t('peopleSaveError')) }
    finally { setBusy(false) }
  }
  async function remove() {
    if (!editing || !window.confirm(t('deleteGuardianConfirm', { name: form.name_en }))) return
    setBusy(true); setError(''); setMessage('')
    try { await authorized(token => deleteGuardian(token, editing)); setSelected(null); setEditing(''); setForm(blank); if (listing.data?.items.length === 1) setPage(Math.max(1, page - 1)); await cache.invalidateQueries({ queryKey: ['people', 'guardians'] }); setMessage(t('peopleDeleted')) }
    catch { setError(t('peopleSaveError')) }
    finally { setBusy(false) }
  }
  const pages = Math.max(1, Math.ceil((listing.data?.total ?? 0) / 10))
  return <div className="space-y-6"><header className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm"><p className="text-sm font-bold uppercase tracking-widest text-teal-700">M3</p><h1 className="mt-2 text-3xl font-bold">{t('guardiansHeading')}</h1><p className="mt-2 text-slate-600">{t('guardianDescription')}</p><Link to="/admin/students" className="mt-4 inline-block rounded-lg bg-teal-50 px-4 py-2 font-semibold text-teal-800">{t('studentsHeading')}</Link></header>
    {error && <p role="alert" className="rounded-lg bg-rose-50 p-3 text-rose-800">{error}</p>}{message && <p role="status" className="rounded-lg bg-emerald-50 p-3 text-emerald-800">{message}</p>}
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1.4fr)_minmax(19rem,1fr)]"><section className="min-w-0 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><label className="block text-sm font-semibold">{t('search')}<input className={peopleField} value={search} onChange={e => { setSearch(e.target.value); setPage(1) }} /></label>
      {listing.isPending ? <p role="status" className="mt-4">{t('academicLoading')}</p> : listing.isError ? <p role="alert" className="mt-4 text-rose-700">{t('peopleLoadError')}</p> : listing.data.items.length === 0 ? <p className="mt-4 text-slate-600">{t('noGuardians')}</p> : <ul className="mt-4 space-y-2">{listing.data.items.map(item => <li key={item.id} className="rounded-xl border border-slate-200 p-3"><div className="flex flex-wrap justify-between gap-2"><div><p className="font-semibold">{i18n.language === 'bn' && item.name_bn ? item.name_bn : item.name_en}</p><p className="text-sm text-slate-600">{item.phone || item.email}</p></div><div className="flex gap-2"><button className="rounded-lg px-3 py-2 font-semibold text-teal-800" onClick={() => setSelected(item)}>{t('viewProfile')}</button>{canManage && <button className="rounded-lg px-3 py-2 font-semibold text-teal-800" onClick={() => edit(item)}>{t('edit')}</button>}</div></div></li>)}</ul>}
      <div className="mt-5 flex items-center justify-between gap-2 text-sm"><button className="rounded-lg border px-3 py-2 disabled:opacity-40" disabled={page <= 1} onClick={() => setPage(page - 1)}>{t('previous')}</button><span>{t('pageOf', { page, total: pages })}</span><button className="rounded-lg border px-3 py-2 disabled:opacity-40" disabled={page >= pages} onClick={() => setPage(page + 1)}>{t('next')}</button></div>
      {selected && <div className="mt-6 border-t pt-5"><h2 className="text-xl font-bold">{i18n.language === 'bn' && selected.name_bn ? selected.name_bn : selected.name_en}</h2><p className="mt-2 text-sm text-slate-600">{selected.phone} {selected.email}</p><p className="mt-1 text-sm text-slate-600">{selected.address}</p><h3 className="mt-5 font-semibold">{t('guardianChildren')}</h3>{children.isPending ? <p role="status">{t('academicLoading')}</p> : children.isError ? <p role="alert">{t('peopleLoadError')}</p> : children.data.length === 0 ? <p className="text-sm text-slate-600">{t('noStudents')}</p> : <ul className="mt-2 space-y-2">{children.data.map(child => <li key={child.id}><Link className="font-semibold text-teal-800 underline" to={`/admin/students/${child.id}`}>{child.name_en} · {child.admission_no}</Link></li>)}</ul>}</div>}
    </section>{canManage && <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><h2 className="text-xl font-bold">{t(editing ? 'editGuardian' : 'createGuardian')}</h2><form className="mt-4 space-y-3" onSubmit={e => void save(e)}>
      <label className="block text-sm font-semibold">{t('guardianNameEN')}<input required maxLength={160} className={peopleField} value={form.name_en} onChange={e => setForm({ ...form, name_en: e.target.value })} /></label>
      <label className="block text-sm font-semibold">{t('guardianNameBN')}<input maxLength={160} className={peopleField} value={form.name_bn} onChange={e => setForm({ ...form, name_bn: e.target.value })} /></label>
      <label className="block text-sm font-semibold">{t('phoneLabel')}<input type="tel" maxLength={32} className={peopleField} value={form.phone} onChange={e => setForm({ ...form, phone: e.target.value })} /></label>
      <label className="block text-sm font-semibold">{t('email')}<input type="email" maxLength={254} className={peopleField} value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} /></label>
      <label className="block text-sm font-semibold">{t('addressLabel')}<textarea maxLength={500} className={peopleField} value={form.address} onChange={e => setForm({ ...form, address: e.target.value })} /></label>
      <label className="block text-sm font-semibold">{t('guardianAccount')}<input className={peopleField} placeholder={t('search')} value={accountSearch} onChange={e => setAccountSearch(e.target.value)} /><select className={peopleField} value={form.user_id ?? ''} onChange={e => setForm({ ...form, user_id: e.target.value })}><option value="">{t('optionalAccount')}</option>{accounts.data?.items.map(u => <option key={u.id} value={u.id}>{u.display_name || u.email} ({u.email})</option>)}{form.user_id && !accounts.data?.items.some(u => u.id === form.user_id) && <option value={form.user_id}>{form.user_id}</option>}</select></label>
      <div className="flex flex-wrap gap-2"><button disabled={busy} className="rounded-lg bg-teal-700 px-4 py-2 font-semibold text-white disabled:opacity-50">{t('saveChanges')}</button>{editing && <><button type="button" className="rounded-lg border px-4 py-2" onClick={() => { setEditing(''); setForm(blank) }}>{t('cancel')}</button><button type="button" disabled={busy} className="rounded-lg border border-rose-300 px-4 py-2 font-semibold text-rose-700" onClick={() => void remove()}>{t('deleteGuardian')}</button></>}</div>
    </form></section>}</div>
  </div>
}
