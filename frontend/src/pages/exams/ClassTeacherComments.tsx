import { useEffect, useId, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../../api/client'
import { getCommentSections, getCommentSheet, getPresets, reviewComments, saveComment, savePreset, type CommentPreset, type CommentSheet, type StudentComment } from '../../api/marksheetAdmit'
import { CardTitle, EmptyState, Notice, PageHeader, Skeleton, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'

// Class teacher comments (marksheet v2): a class teacher writes one comment
// per student per term or annual result, from a preset or in their own
// words; the head teacher and the administrator review them and keep the
// presets. A comment never changes a published result.

type Draft = { mode: 'none' | 'preset' | 'own'; presetId: string; text: string }

const draftOf = (c: StudentComment): Draft =>
  c.preset_id ? { mode: 'preset', presetId: c.preset_id, text: '' } : c.custom_text ? { mode: 'own', presetId: '', text: c.custom_text } : { mode: 'none', presetId: '', text: '' }

function useExplain() {
  const { t } = useTranslation()
  return (cause: unknown) => cause instanceof ApiError && cause.status === 403 ? t('msErrForbidden') : t('msErrGeneric')
}

export function ClassTeacherComments({ reviewer }: { reviewer: boolean }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const uid = useId()
  const sections = useQuery({ queryKey: ['ms', 'comment-sections'], queryFn: () => authorized(getCommentSections), retry: false })
  const [sectionId, setSectionId] = useState('')
  const [scopeKey, setScopeKey] = useState('')
  const section = sections.data?.find(s => s.section_id === sectionId)
  useEffect(() => {
    if (!sectionId && sections.data?.length) setSectionId(sections.data[0].section_id)
  }, [sections.data, sectionId])
  useEffect(() => {
    if (section && !section.scopes.some(s => `${s.scope}:${s.scope_id}` === scopeKey)) {
      const first = section.scopes[0]
      setScopeKey(first ? `${first.scope}:${first.scope_id}` : '')
    }
  }, [section, scopeKey])
  const [scope, scopeId] = scopeKey.split(':')
  const scopeName = (s: { scope: string; name: string }) => s.scope === 'annual' ? t('msAnnualOf', { session: s.name }) : s.name
  const bn = i18n.language === 'bn'

  return <div className="space-y-5">
    <PageHeader eyebrow={t('m12Group_academics')} title={t('msCommentsTitle')} description={t(reviewer ? 'msCommentsIntroReviewer' : 'msCommentsIntro')} />
    {sections.isPending ? <Skeleton rows={3} /> : sections.isError ? <Notice tone="error">{t('msLoadError')}</Notice> : sections.data.length === 0 ?
      <EmptyState icon="users" title={t('msNoSections')}>{t('msNoSectionsHint')}</EmptyState> : <>
        <section className={`${ui.card} p-4 sm:p-5`} aria-label={t('msCommentsTitle')}>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <label className={ui.label} htmlFor={`${uid}-section`}>{t('msSection')}
              <select id={`${uid}-section`} className={ui.field} value={sectionId} onChange={e => setSectionId(e.target.value)}>
                {sections.data.map(s => <option key={s.section_id} value={s.section_id}>{s.class_name} · {s.section_name} ({s.session_name})</option>)}
              </select>
            </label>
            <label className={ui.label} htmlFor={`${uid}-scope`}>{t('msResult')}
              <select id={`${uid}-scope`} className={ui.field} value={scopeKey} onChange={e => setScopeKey(e.target.value)}>
                {section?.scopes.map(s => <option key={`${s.scope}:${s.scope_id}`} value={`${s.scope}:${s.scope_id}`}>{scopeName(s)}</option>)}
              </select>
            </label>
          </div>
          {section && <p className="mt-2 text-sm text-slate-600">{section.teacher_name ? t('msClassTeacherLine', { name: section.teacher_name }) : t('msNoClassTeacher')}</p>}
        </section>
        {section && scope && scopeId && <CommentTable key={`${sectionId}:${scopeKey}`} scope={scope as 'term' | 'annual'} scopeId={scopeId} sectionId={sectionId} bn={bn} />}
      </>}
    {reviewer && <PresetEditor />}
  </div>
}

function CommentTable({ scope, scopeId, sectionId, bn }: { scope: 'term' | 'annual'; scopeId: string; sectionId: string; bn: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const explain = useExplain()
  const key = ['ms', 'sheet', scope, scopeId, sectionId]
  const sheet = useQuery({ queryKey: key, queryFn: () => authorized(token => getCommentSheet(token, scope, scopeId, sectionId)), retry: false })
  const [drafts, setDrafts] = useState<Record<string, Draft>>({})
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  if (sheet.isPending) return <Skeleton rows={5} />
  if (sheet.isError) return <Notice tone="error">{explain(sheet.error)}</Notice>
  const data: CommentSheet = sheet.data
  const draft = (c: StudentComment) => drafts[c.student_id] ?? draftOf(c)
  const changed = (c: StudentComment) => JSON.stringify(draft(c)) !== JSON.stringify(draftOf(c))
  const presetText = (p: CommentPreset) => bn ? p.text_bn : p.text_en
  const nameOf = (c: StudentComment) => bn && c.name_bn ? c.name_bn : c.name_en
  const written = data.students.filter(c => c.status === 'written').length

  async function save(c: StudentComment) {
    const d = draft(c)
    setBusy(c.student_id); setMessage(null)
    try {
      await authorized(token => saveComment(token, { scope, scope_id: scopeId, student_id: c.student_id, preset_id: d.mode === 'preset' ? d.presetId || null : null, custom_text: d.mode === 'own' ? d.text.trim() : '' }))
      setDrafts(({ [c.student_id]: _, ...rest }) => rest)
      await client.invalidateQueries({ queryKey: key })
      setMessage({ tone: 'success', text: d.mode === 'none' ? t('msCleared') : t('msSaved') })
    } catch (cause) { setMessage({ tone: 'error', text: explain(cause) }) } finally { setBusy('') }
  }
  async function reviewAll() {
    setBusy('review'); setMessage(null)
    try {
      const out = await authorized(token => reviewComments(token, { scope, scope_id: scopeId, section_id: sectionId }))
      await client.invalidateQueries({ queryKey: key })
      setMessage({ tone: 'success', text: t('msReviewedCount', { n: out.reviewed }) })
    } catch (cause) { setMessage({ tone: 'error', text: explain(cause) }) } finally { setBusy('') }
  }

  return <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="ms-comments-table">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <CardTitle id="ms-comments-table" icon="notebook">{data.class_name} · {data.section_name}</CardTitle>
      {data.can_review && <button type="button" className={ui.secondary} disabled={!!busy || written === 0} onClick={() => void reviewAll()}>{written === 0 ? t('msNothingToReview') : t('msReviewAll')}</button>}
    </div>
    {message && <div className="mt-3"><Notice tone={message.tone}>{message.text}</Notice></div>}
    <ul className="mt-4 divide-y divide-slate-100" data-testid="comment-rows">{data.students.map(c => {
      const d = draft(c)
      return <li key={c.student_id} className="grid grid-cols-1 gap-3 py-3 md:grid-cols-[minmax(0,14rem)_minmax(0,1fr)_auto] md:items-start">
        <div className="min-w-0">
          <p className="font-semibold text-slate-900">{c.roll_no != null && <span className="mr-1.5 tabular-nums text-slate-500">{c.roll_no}.</span>}{nameOf(c)}</p>
          <p className="text-xs text-slate-600">{c.admission_no}{c.gpa ? ` · ${t('msGpa')} ${c.gpa}` : ''}</p>
          <p className="mt-1"><StatusChip status={c.status} />{c.written_by && <span className="ml-2 text-xs text-slate-500">{t('msWrittenBy', { name: c.written_by })}</span>}</p>
        </div>
        <div className="min-w-0 space-y-2">
          <select aria-label={t('msCommentFor', { name: nameOf(c) })} className={ui.field} disabled={!data.can_write}
            value={d.mode === 'preset' ? d.presetId : d.mode === 'own' ? '__own' : ''}
            onChange={e => setDrafts({ ...drafts, [c.student_id]: e.target.value === '' ? { mode: 'none', presetId: '', text: '' } : e.target.value === '__own' ? { mode: 'own', presetId: '', text: d.text } : { mode: 'preset', presetId: e.target.value, text: '' } })}>
            <option value="">{t('msNoComment')}</option>
            {data.presets.map(p => <option key={p.id} value={p.id}>{presetText(p)}</option>)}
            <option value="__own">{t('msOwnWords')}</option>
          </select>
          {d.mode === 'own' && <textarea aria-label={t('msOwnFor', { name: nameOf(c) })} className={`${ui.field} min-h-20`} maxLength={400} placeholder={t('msOwnPlaceholder')}
            value={d.text} disabled={!data.can_write} onChange={e => setDrafts({ ...drafts, [c.student_id]: { ...d, text: e.target.value } })} />}
        </div>
        {data.can_write && <button type="button" className={ui.primary} disabled={!!busy || !changed(c) || (d.mode === 'own' && !d.text.trim())} aria-busy={busy === c.student_id} onClick={() => void save(c)}>{t('msSave')}</button>}
      </li>
    })}</ul>
  </section>
}

function StatusChip({ status }: { status: StudentComment['status'] }) {
  const { t } = useTranslation()
  const tone = status === 'reviewed' ? 'bg-emerald-50 text-emerald-800 ring-emerald-600/30' : status === 'written' ? 'bg-sky-50 text-sky-900 ring-sky-600/30' : 'bg-slate-100 text-slate-700 ring-slate-400/30'
  return <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ring-inset ${tone}`}>{t(`msStatus_${status || 'none'}`)}</span>
}

function PresetEditor() {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const explain = useExplain()
  const presets = useQuery({ queryKey: ['ms', 'presets', 'all'], queryFn: () => authorized(token => getPresets(token, true)), retry: false })
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  async function save(input: { text_en: string; text_bn: string; display_order: number; is_active: boolean }, id?: string) {
    setMessage(null)
    try {
      await authorized(token => savePreset(token, input, id))
      await client.invalidateQueries({ queryKey: ['ms'] })
      setMessage({ tone: 'success', text: t('msSaved') })
      return true
    } catch (cause) { setMessage({ tone: 'error', text: explain(cause) }); return false }
  }
  return <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="ms-presets">
    <CardTitle id="ms-presets" icon="settings">{t('msPresetsTitle')}</CardTitle>
    <p className="mt-1 text-sm text-slate-600">{t('msPresetsIntro')}</p>
    {message && <div className="mt-3"><Notice tone={message.tone}>{message.text}</Notice></div>}
    {presets.isPending ? <Skeleton rows={3} /> : presets.isError ? <Notice tone="error">{t('msLoadError')}</Notice> :
      <ul className="mt-4 space-y-3">
        {presets.data.map(p => <li key={p.id}><PresetRow preset={p} onSave={(input) => save(input, p.id)} /></li>)}
        <li><PresetRow onSave={input => save(input)} /></li>
      </ul>}
  </section>
}

function PresetRow({ preset, onSave }: { preset?: CommentPreset; onSave: (input: { text_en: string; text_bn: string; display_order: number; is_active: boolean }) => Promise<boolean> }) {
  const { t } = useTranslation()
  const uid = useId()
  const blank = { text_en: '', text_bn: '', display_order: 100, is_active: true }
  const [form, setForm] = useState(preset ? { text_en: preset.text_en, text_bn: preset.text_bn, display_order: preset.display_order, is_active: preset.is_active } : blank)
  const [busy, setBusy] = useState(false)
  return <form className={`grid grid-cols-1 gap-3 rounded-xl border p-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_6rem_auto] md:items-end ${preset ? 'border-slate-200' : 'border-dashed border-slate-300'}`}
    onSubmit={async e => { e.preventDefault(); setBusy(true); const ok = await onSave(form); setBusy(false); if (ok && !preset) setForm(blank) }}>
    {!preset && <p className="text-sm font-semibold text-slate-800 md:col-span-4">{t('msPresetNew')}</p>}
    <label className={ui.label} htmlFor={`${uid}-en`}>{t('msPresetEN')}<input id={`${uid}-en`} required maxLength={300} className={ui.field} value={form.text_en} onChange={e => setForm({ ...form, text_en: e.target.value })} /></label>
    <label className={ui.label} htmlFor={`${uid}-bn`}>{t('msPresetBN')}<input id={`${uid}-bn`} required maxLength={300} className={ui.field} value={form.text_bn} onChange={e => setForm({ ...form, text_bn: e.target.value })} /></label>
    <label className={ui.label} htmlFor={`${uid}-order`}>{t('msPresetOrder')}<input id={`${uid}-order`} type="number" min={0} max={10000} className={ui.field} value={form.display_order} onChange={e => setForm({ ...form, display_order: Number(e.target.value) })} /></label>
    <div className="flex flex-wrap items-center gap-3">
      <label className="flex min-h-10 items-center gap-2 text-sm font-semibold text-slate-800"><input type="checkbox" checked={form.is_active} onChange={e => setForm({ ...form, is_active: e.target.checked })} />{t('msPresetActive')}</label>
      <button className={ui.secondary} disabled={busy}>{t(preset ? 'msPresetSave' : 'msPresetAdd')}</button>
    </div>
  </form>
}
