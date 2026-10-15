import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError, getPermissions, getRolePermissions, getRoles, putRolePermissions } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

export function PermissionPage({ canManage }: { canManage: boolean }) {
  const { t } = useTranslation()
  const { accessToken, renew, expire } = useAuth()
  const queryClient = useQueryClient()
  const [selectedRole, setSelectedRole] = useState('admin')
  const [draft, setDraft] = useState<string[]>([])
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

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

  const roles = useQuery({ queryKey: ['rbac', 'roles'], queryFn: () => authorized(getRoles), retry: false })
  const permissions = useQuery({ queryKey: ['rbac', 'permissions'], queryFn: () => authorized(getPermissions), retry: false })
  const grants = useQuery({ queryKey: ['rbac', 'grants', selectedRole], queryFn: () => authorized((token) => getRolePermissions(token, selectedRole)), enabled: !!selectedRole, retry: false })

  useEffect(() => { setDraft(grants.data ?? []) }, [grants.data, selectedRole])
  useEffect(() => { setMessage(''); setError('') }, [selectedRole])

  const save = useMutation({
    mutationFn: () => authorized((token) => putRolePermissions(token, selectedRole, draft)),
    onSuccess: async () => {
      setMessage(t('permissionsSaved'))
      setError('')
      await queryClient.invalidateQueries({ queryKey: ['rbac', 'grants', selectedRole] })
      await queryClient.invalidateQueries({ queryKey: ['my-permissions'] })
    },
    onError: (cause) => {
      setMessage('')
      setError(cause instanceof ApiError && cause.code === 'last_admin' ? t('lastAdminError') : t('savePermissionsError'))
    },
  })

  const isLoading = roles.isPending || permissions.isPending || grants.isPending
  const isError = roles.isError || permissions.isError || grants.isError
  const canSave = canManage && !save.isPending

  return <section className="space-y-6">
    <div>
      <p className="text-sm font-bold uppercase tracking-[0.2em] text-teal-700">{t('administration')}</p>
      <h1 className="mt-2 text-3xl font-bold text-slate-900">{t('permissionEditor')}</h1>
      <p className="mt-2 max-w-2xl text-slate-600">{t('permissionEditorDescription')}</p>
    </div>
    {isLoading && <p role="status" className="text-slate-600">{t('loadingPermissions')}</p>}
    {isError && <div role="alert" className="rounded-xl border border-rose-200 bg-rose-50 p-4 text-rose-800">{t('loadPermissionsError')} <button type="button" className="font-semibold underline" onClick={() => { void roles.refetch(); void permissions.refetch(); void grants.refetch() }}>{t('reload')}</button></div>}
    {!isLoading && !isError && roles.data && permissions.data && <div className="grid gap-6 lg:grid-cols-[15rem_minmax(0,1fr)]">
      <nav aria-label={t('roles')} className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
        <h2 className="px-2 text-lg font-bold text-slate-900">{t('roles')}</h2>
        {roles.data.items.length === 0 && <p className="px-2 py-4 text-slate-600">{t('noRoles')}</p>}
        <div className="mt-3 grid gap-1 sm:grid-cols-2 lg:grid-cols-1">
          {roles.data.items.map((role) => <button key={role.id} type="button" onClick={() => setSelectedRole(role.id)} aria-current={selectedRole === role.id ? 'page' : undefined} className={`rounded-lg px-3 py-2 text-left text-sm font-semibold ${selectedRole === role.id ? 'bg-teal-100 text-teal-900' : 'text-slate-700 hover:bg-slate-100'}`}>{role.name}</button>)}
        </div>
      </nav>
      <div className="min-w-0 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-7">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div><h2 className="text-xl font-bold text-slate-900">{t('permissionMatrix')}</h2><p className="mt-1 text-sm text-slate-600">{selectedRole}</p></div>
          {canManage && <button type="button" disabled={!canSave} onClick={() => { setMessage(''); setError(''); save.mutate() }} className="rounded-lg bg-teal-700 px-5 py-2.5 font-semibold text-white hover:bg-teal-800 disabled:cursor-not-allowed disabled:opacity-50">{save.isPending ? t('saving') : t('saveChanges')}</button>}
        </div>
        {message && <p role="status" className="mt-4 rounded-lg bg-emerald-50 p-3 text-emerald-800">{message}</p>}
        {error && <p role="alert" className="mt-4 rounded-lg bg-rose-50 p-3 text-rose-800">{error}</p>}
        {permissions.data.items.length === 0 ? <p className="mt-6 text-slate-600">{t('noPermissions')}</p> :
          <div className="mt-6 grid gap-3 sm:grid-cols-2">
            {permissions.data.items.map((permission) => <label key={permission.id} className="flex min-w-0 cursor-pointer items-start gap-3 rounded-xl border border-slate-200 p-4 hover:border-teal-400">
              <input type="checkbox" disabled={!canManage} checked={draft.includes(permission.id)} onChange={(event) => setDraft((current) => event.target.checked ? [...current, permission.id] : current.filter((id) => id !== permission.id))} className="mt-1 h-4 w-4 accent-teal-700" />
              <span className="min-w-0"><span className="block break-all text-sm font-semibold text-slate-900">{permission.id}</span><span className="mt-1 block text-sm text-slate-600">{permission.description}</span></span>
            </label>)}
          </div>}
      </div>
    </div>}
  </section>
}
