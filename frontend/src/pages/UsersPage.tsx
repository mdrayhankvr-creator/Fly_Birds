import { confirmAction } from '../components/ConfirmDialog'
import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError, createUser, getUser, getUsers, setUserRole, setUserStatus, updateUser, type ManagedUser, type NewUser } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

const roles = ['admin', 'head_teacher', 'teacher', 'student', 'guardian', 'accountant']
const inputClass = 'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900 focus:border-[var(--brand-primary)] focus:outline-none'

export function UsersPage({ permissions }: { permissions: string[] }) {
  const { t } = useTranslation()
  const { accessToken, renew, expire } = useAuth()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [roleFilter, setRoleFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [page, setPage] = useState(1)
  const [selectedId, setSelectedId] = useState('')
  const [newUser, setNewUser] = useState<NewUser>({ email: '', display_name: '', role: 'teacher', password: '' })
  const [edit, setEdit] = useState<{ email: string; display_name: string } | null>(null)
  const [chosenRole, setChosenRole] = useState('')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const canCreate = permissions.includes('users:create') && permissions.includes('permissions:manage')
  const canUpdate = permissions.includes('users:update')
  const canAssign = permissions.includes('permissions:manage')

  async function authorized<T>(request: (token: string) => Promise<T>): Promise<T> {
    if (!accessToken) throw new ApiError(401, 'unauthorized')
    try { return await request(accessToken) }
    catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        try { return await request(await renew()) }
        catch (retryCause) { if (retryCause instanceof ApiError && retryCause.status === 401) expire(); throw retryCause }
      }
      throw cause
    }
  }

  const params = new URLSearchParams({ page: String(page), page_size: '10', search })
  if (roleFilter) params.set('role', roleFilter)
  if (statusFilter) params.set('status', statusFilter)
  const users = useQuery({ queryKey: ['users', params.toString()], queryFn: () => authorized((token) => getUsers(token, params)), retry: false })
  const selected = useQuery({ queryKey: ['user', selectedId], enabled: !!selectedId, queryFn: () => authorized((token) => getUser(token, selectedId)), retry: false })

  function selectUser(user: ManagedUser) { setSelectedId(user.id); setEdit({ email: user.email, display_name: user.display_name }); setChosenRole(user.role); setError(''); setMessage('') }
  async function run(action: () => Promise<unknown>) {
    setBusy(true); setError(''); setMessage('')
    try {
      await action()
      setMessage(t('userSaved'))
      await Promise.all([queryClient.invalidateQueries({ queryKey: ['users'] }), queryClient.invalidateQueries({ queryKey: ['user'] }), queryClient.invalidateQueries({ queryKey: ['dashboard'] })])
    } catch (cause) {
      setError(cause instanceof ApiError && cause.code === 'last_admin' ? t('lastAdminError') : cause instanceof ApiError && cause.code === 'duplicate_email' ? t('duplicateEmail') : t('userSaveError'))
    } finally { setBusy(false) }
  }
  function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const input = { ...newUser }
    if (!input.display_name.trim() || new TextEncoder().encode(input.password).length < 12 || new TextEncoder().encode(input.password).length > 72) { setError(t('invalidUserForm')); return }
    void run(async () => { const created = await authorized((token) => createUser(token, input)); setNewUser({ email: '', display_name: '', role: 'teacher', password: '' }); selectUser(created) })
  }
  function update(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selectedId || !edit) return
    void run(() => authorized((token) => updateUser(token, selectedId, edit)))
  }
  async function changeStatus(user: ManagedUser) {
    if (!(await confirmAction(t(user.is_active ? 'confirmDeactivate' : 'confirmActivate', { name: user.display_name })))) return
    void run(() => authorized((token) => setUserStatus(token, user.id, !user.is_active)))
  }
  async function changeRole(user: ManagedUser) {
    if (!(await confirmAction(t('confirmRole', { name: user.display_name, role: chosenRole })))) return
    void run(() => authorized((token) => setUserRole(token, user.id, chosenRole)))
  }

  return <div className="space-y-8">
    <section><h1 className="text-3xl font-bold text-slate-900">{t('usersHeading')}</h1><p className="mt-2 text-slate-600">{t('usersDescription')}</p></section>
    {message && <p role="status" className="rounded-lg bg-emerald-50 p-3 text-emerald-900">{message}</p>}
    {error && <p role="alert" className="rounded-lg bg-rose-50 p-3 text-rose-800">{error}</p>}
    {canCreate && <form onSubmit={create} className="grid gap-4 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:grid-cols-2">
      <h2 className="text-xl font-semibold text-slate-900 sm:col-span-2">{t('createUser')}</h2>
      <label className="text-sm font-semibold text-slate-700">{t('displayName')}<input className={inputClass} required maxLength={160} value={newUser.display_name} onChange={(e) => setNewUser({ ...newUser, display_name: e.target.value })} /></label>
      <label className="text-sm font-semibold text-slate-700">{t('email')}<input className={inputClass} type="email" required maxLength={254} value={newUser.email} onChange={(e) => setNewUser({ ...newUser, email: e.target.value })} /></label>
      <label className="text-sm font-semibold text-slate-700">{t('role')}<select className={inputClass} value={newUser.role} onChange={(e) => setNewUser({ ...newUser, role: e.target.value })}>{roles.map((role) => <option key={role} value={role}>{t(`role_${role}`)}</option>)}</select></label>
      <label className="text-sm font-semibold text-slate-700">{t('initialPassword')}<input className={inputClass} type="password" minLength={12} required autoComplete="new-password" value={newUser.password} onChange={(e) => setNewUser({ ...newUser, password: e.target.value })} /></label>
      <p className="text-sm text-slate-600 sm:col-span-2">{t('provisioningLimit')}</p>
      <button disabled={busy} className="brand-bg rounded-lg px-4 py-2 font-semibold text-white disabled:opacity-50 sm:justify-self-start">{t('createUser')}</button>
    </form>}
    <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
      <h2 className="text-xl font-semibold text-slate-900">{t('userDirectory')}</h2>
      <div className="mt-4 grid gap-3 sm:grid-cols-3">
        <label className="text-sm font-semibold text-slate-700">{t('search')}<input className={inputClass} value={search} maxLength={100} onChange={(e) => { setPage(1); setSearch(e.target.value) }} /></label>
        <label className="text-sm font-semibold text-slate-700">{t('role')}<select className={inputClass} value={roleFilter} onChange={(e) => { setPage(1); setRoleFilter(e.target.value) }}><option value="">{t('allRoles')}</option>{roles.map((role) => <option key={role} value={role}>{t(`role_${role}`)}</option>)}</select></label>
        <label className="text-sm font-semibold text-slate-700">{t('status')}<select className={inputClass} value={statusFilter} onChange={(e) => { setPage(1); setStatusFilter(e.target.value) }}><option value="">{t('allStatuses')}</option><option value="active">{t('active')}</option><option value="inactive">{t('inactive')}</option></select></label>
      </div>
      {users.isPending ? <p role="status" className="mt-5">{t('loadingUsers')}</p> : users.isError ? <p role="alert" className="mt-5 text-rose-700">{t('loadUsersError')}</p> : users.data.items.length === 0 ? <p className="mt-5 text-slate-600">{t('noUsers')}</p> : <div className="mt-5 w-full overflow-x-auto"><table className="min-w-[38rem] w-full text-left text-sm"><thead className="border-b text-slate-600"><tr><th className="p-2">{t('displayName')}</th><th className="p-2">{t('email')}</th><th className="p-2">{t('role')}</th><th className="p-2">{t('status')}</th><th className="p-2">{t('details')}</th></tr></thead><tbody>{users.data.items.map((user) => <tr key={user.id} className="border-b last:border-0"><td className="p-2">{user.display_name}</td><td className="p-2">{user.email}</td><td className="p-2">{t(`role_${user.role}`)}</td><td className="p-2">{t(user.is_active ? 'active' : 'inactive')}</td><td className="p-2"><button className="font-semibold text-[var(--brand-ink)] underline" onClick={() => selectUser(user)}>{t('view')}</button></td></tr>)}</tbody></table></div>}
      <div className="mt-5 flex items-center justify-between gap-2 text-sm"><button disabled={page <= 1 || users.isPending} onClick={() => setPage(page - 1)} className="rounded border px-3 py-2 disabled:opacity-40">{t('previous')}</button><span>{t('pageOf', { page, total: Math.max(1, Math.ceil((users.data?.total || 0) / 10)) })}</span><button disabled={users.isPending || !users.data || page * 10 >= users.data.total} onClick={() => setPage(page + 1)} className="rounded border px-3 py-2 disabled:opacity-40">{t('next')}</button></div>
    </section>
    {selectedId && <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><h2 className="text-xl font-semibold text-slate-900">{t('userDetails')}</h2>
      {selected.isPending ? <p role="status">{t('loadingUsers')}</p> : selected.isError ? <p role="alert">{t('loadUsersError')}</p> : <div className="mt-4 space-y-4"><p className="break-all text-sm text-slate-600">{selected.data.id}</p><p>{t('status')}: {t(selected.data.is_active ? 'active' : 'inactive')}</p>
        {canUpdate && edit && <form onSubmit={update} className="grid gap-3 sm:grid-cols-2"><label>{t('displayName')}<input className={inputClass} required maxLength={160} value={edit.display_name} onChange={(e) => setEdit({ ...edit, display_name: e.target.value })} /></label><label>{t('email')}<input className={inputClass} type="email" required maxLength={254} value={edit.email} onChange={(e) => setEdit({ ...edit, email: e.target.value })} /></label><button disabled={busy} className="brand-bg rounded-lg px-4 py-2 font-semibold text-white disabled:opacity-50 sm:justify-self-start">{t('saveChanges')}</button></form>}
        <div className="flex flex-wrap items-end gap-3">{canAssign && <label>{t('role')}<select className={inputClass} value={chosenRole} onChange={(e) => setChosenRole(e.target.value)}>{roles.map((role) => <option key={role} value={role}>{t(`role_${role}`)}</option>)}</select></label>}{canAssign && chosenRole !== selected.data.role && <button disabled={busy} onClick={() => changeRole(selected.data)} className="rounded-lg border px-4 py-2 font-semibold disabled:opacity-50">{t('assignRole')}</button>}{canUpdate && <button disabled={busy} onClick={() => changeStatus(selected.data)} className="rounded-lg border px-4 py-2 font-semibold disabled:opacity-50">{t(selected.data.is_active ? 'deactivate' : 'activate')}</button>}</div>
      </div>}
    </section>}
  </div>
}
