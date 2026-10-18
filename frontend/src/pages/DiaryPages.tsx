import { useEffect, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router'
import { deleteDiary, getAllDiaries, getDiary, getDiaryAssignments, getDiaryCompliance, getDiaryFeed, getDiarySettings, getMySubmission, getSubmissions, publishDiary, reviewSubmission, saveDiary, saveDiarySettings, submitHomework, type Attachment, type Diary, type DiaryAssignment, type DiaryInput, type Submission } from '../api/communication'
import { ApiError } from '../api/client'
import { EmptyState, Notice as Banner, PageHeader, Pager, Skeleton, StatusBadge, ui } from './attendanceUi'
import { AttachmentList, Field, FileUploader, isoToLocal, localToIso, shiftDay, todayDhaka, useApi, useDhakaFormat, useErrorText, useLocal } from './commShared'

const sections: { key: keyof Pick<Diary, 'lesson_topics' | 'class_notes' | 'homework' | 'instructions'>; label: string }[] = [
  { key: 'lesson_topics', label: 'm9LessonTopics' }, { key: 'class_notes', label: 'm9ClassNotes' }, { key: 'homework', label: 'm9Homework' }, { key: 'instructions', label: 'm9Instructions' },
]

function DiaryBody({ diary, filePath }: { diary: Diary; filePath: (f: Attachment) => string }) {
  const { t } = useTranslation()
  return <div className="space-y-3">
    {sections.map(s => diary[s.key] ? <div key={s.key}><h4 className="text-xs font-bold uppercase tracking-wide text-slate-500">{t(s.label)}</h4><p className={`mt-1 whitespace-pre-line text-sm leading-relaxed ${s.key === 'homework' ? 'rounded-lg bg-amber-50 p-3 text-amber-950' : 'text-slate-800'}`}>{diary[s.key]}</p></div> : null)}
    <AttachmentList items={diary.attachments} pathFor={filePath} />
  </div>
}

function DiaryStatus({ status }: { status: 'draft' | 'published' | null }) {
  const { t } = useTranslation()
  return <StatusBadge tone={status === 'published' ? 'present' : status === 'draft' ? 'late' : 'unmarked'}>{t(status ? `m9Status_${status}` : 'm9Missing')}</StatusBadge>
}

export function TeacherDiaryPage() {
  const { t } = useTranslation()
  const authorized = useApi()
  const local = useLocal()
  const fmt = useDhakaFormat()
  const [date, setDate] = useState(todayDhaka())
  const [selected, setSelected] = useState<DiaryAssignment | null>(null)
  const assignments = useQuery({ queryKey: ['m9', 'diary-assignments', date], queryFn: () => authorized(token => getDiaryAssignments(token, date)), retry: false })
  const published = assignments.data?.filter(a => a.diary_status === 'published').length ?? 0
  return <div className="space-y-5">
    <PageHeader eyebrow={t('m9DiaryEyebrow')} title={t('m9TeacherDiary')} description={t('m9TeacherDiaryIntro')} />
    <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="diary-day">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div><h2 id="diary-day" className="text-lg font-bold">{fmt.date(date)}</h2>{assignments.data && <p className="text-sm text-slate-600">{t('m9DiaryProgress', { published: fmt.number(published), total: fmt.number(assignments.data.length) })}</p>}</div>
        <div className="flex gap-2">
          <button type="button" className={ui.secondary} aria-label={t('m9PreviousDay')} onClick={() => { setDate(shiftDay(date, -1)); setSelected(null) }}>‹</button>
          <label><span className="sr-only">{t('m9Date')}</span><input type="date" className={`${ui.field} mt-0`} max={shiftDay(todayDhaka(), 7)} value={date} onChange={e => { if (e.target.value) { setDate(e.target.value); setSelected(null) } }} /></label>
          <button type="button" className={ui.secondary} aria-label={t('m9NextDay')} disabled={date >= shiftDay(todayDhaka(), 7)} onClick={() => { setDate(shiftDay(date, 1)); setSelected(null) }}>›</button>
          <button type="button" className={ui.secondary} onClick={() => { setDate(todayDhaka()); setSelected(null) }}>{t('m9Today')}</button>
        </div>
      </div>
      {assignments.isPending ? <div className="mt-4"><Skeleton rows={3} /></div> : assignments.isError ? <div className="mt-4"><Banner tone="error">{t('m9ErrGeneric')}</Banner></div> : assignments.data.length === 0 ? <div className="mt-4"><EmptyState title={t('m9NoAssignments')}>{t('m9NoAssignmentsHint')}</EmptyState></div> :
        <ul className="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">{assignments.data.map(a => {
          const active = selected?.section_id === a.section_id && selected.subject_id === a.subject_id
          return <li key={`${a.section_id}-${a.subject_id}`}><button type="button" aria-pressed={active} onClick={() => setSelected(a)} className={`w-full rounded-xl border p-3 text-left transition ${active ? 'border-[var(--brand-primary)] ring-2 ring-[color-mix(in_srgb,var(--brand-primary)_25%,transparent)]' : 'border-slate-200 hover:border-slate-300'}`}>
            <span className="flex items-start justify-between gap-2"><span className="font-semibold text-slate-900">{local(a.subject_name_en, a.subject_name_bn)}</span><DiaryStatus status={a.diary_status} /></span>
            <span className="mt-1 block text-sm text-slate-600">{a.class_name} · {a.section_name}</span></button></li>
        })}</ul>}
    </section>
    {selected && <DiaryEditor key={`${selected.section_id}-${selected.subject_id}-${date}`} assignment={selected} date={date} onSaved={() => void assignments.refetch()} />}
  </div>
}

const emptyInput: DiaryInput = { lesson_topics: '', class_notes: '', homework: '', instructions: '', submission_enabled: false, submission_due_at: null, file_ids: [] }

function DiaryEditor({ assignment, date, onSaved }: { assignment: DiaryAssignment; date: string; onSaved: () => void }) {
  const { t } = useTranslation()
  const authorized = useApi()
  const client = useQueryClient()
  const local = useLocal()
  const errorText = useErrorText()
  const [diaryId, setDiaryId] = useState(assignment.diary_id)
  const existing = useQuery({ queryKey: ['m9', 'diary', diaryId], enabled: !!diaryId, queryFn: () => authorized(token => getDiary(token, diaryId!)), retry: false })
  const [form, setForm] = useState<DiaryInput>(emptyInput)
  const [files, setFiles] = useState<Attachment[]>([])
  const [due, setDue] = useState('')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const diary = existing.data
  useEffect(() => {
    if (!diary) return
    setForm({ lesson_topics: diary.lesson_topics, class_notes: diary.class_notes, homework: diary.homework, instructions: diary.instructions, submission_enabled: diary.submission_enabled, submission_due_at: diary.submission_due_at, file_ids: [] })
    setFiles(diary.attachments); setDue(isoToLocal(diary.submission_due_at))
  }, [diary])
  const payload = (): DiaryInput => ({ ...form, submission_due_at: form.submission_enabled ? localToIso(due) : null, file_ids: files.map(f => f.id), ...(diaryId ? {} : { section_id: assignment.section_id, subject_id: assignment.subject_id, diary_date: date }) })
  const empty = !form.lesson_topics.trim() && !form.class_notes.trim() && !form.homework.trim() && !form.instructions.trim()
  async function run(task: () => Promise<Diary | unknown>, success: string) {
    setBusy(true); setMessage(null)
    try {
      const result = await task()
      if (result && typeof result === 'object' && 'id' in result) setDiaryId((result as Diary).id)
      await client.invalidateQueries({ queryKey: ['m9', 'diary'] })
      setMessage({ tone: 'success', text: t(success) }); onSaved()
    } catch (cause) { setMessage({ tone: 'error', text: errorText(cause) }) } finally { setBusy(false) }
  }
  function save(event: FormEvent) { event.preventDefault(); void run(() => authorized(token => saveDiary(token, payload(), diaryId ?? undefined)), 'm9Saved') }
  const publish = () => run(async () => { const saved = await authorized(token => saveDiary(token, payload(), diaryId ?? undefined)); return authorized(token => publishDiary(token, saved.id)) }, 'm9DiaryPublished')
  if (diaryId && existing.isPending) return <Skeleton rows={4} />
  return <>
    <form onSubmit={save} className={`${ui.card} space-y-4 p-4 sm:p-6`} aria-labelledby="diary-editor">
      <div className="flex flex-wrap items-center justify-between gap-2"><div><h2 id="diary-editor" className="text-lg font-bold">{local(assignment.subject_name_en, assignment.subject_name_bn)} · {assignment.class_name} {assignment.section_name}</h2><p className="text-sm text-slate-600">{assignment.session_name}</p></div><DiaryStatus status={diary?.status ?? null} /></div>
      {message && <Banner tone={message.tone}>{message.text}</Banner>}
      <div className="grid gap-4 md:grid-cols-2">
        {sections.map(s => <Field key={s.key} label={t(s.label)}><textarea rows={s.key === 'class_notes' ? 5 : 3} className={ui.field} value={form[s.key]} placeholder={t(`${s.label}Hint`)} onChange={e => setForm({ ...form, [s.key]: e.target.value })} /></Field>)}
      </div>
      <div><p className={ui.label}>{t('m9Attachments')}</p><AttachmentList items={files} pathFor={f => diaryId && diary?.attachments.some(a => a.id === f.id) ? `/diary/${diaryId}/files/${f.id}` : ''} onRemove={id => setFiles(files.filter(f => f.id !== id))} />
        <div className="mt-2"><FileUploader path="/diary/files" max={10 - files.length} onUploaded={f => setFiles(prev => [...prev, f])} /></div></div>
      <fieldset className="rounded-xl border border-slate-200 p-4"><legend className="px-1 text-sm font-semibold">{t('m9HomeworkSubmission')}</legend>
        <label className="flex items-start gap-3 text-sm"><input type="checkbox" className="mt-1" checked={form.submission_enabled} onChange={e => setForm({ ...form, submission_enabled: e.target.checked })} /><span><span className="font-medium">{t('m9AllowSubmission')}</span><span className="block text-xs text-slate-500">{t('m9AllowSubmissionHint')}</span></span></label>
        {form.submission_enabled && <div className="mt-3 max-w-xs"><Field label={t('m9DueAt')}><input type="datetime-local" className={ui.field} value={due} onChange={e => setDue(e.target.value)} /></Field></div>}
      </fieldset>
      <div className="flex flex-wrap gap-2 border-t border-slate-100 pt-4">
        <button type="submit" className={ui.secondary} disabled={busy}>{diary?.status === 'published' ? t('m9SaveChanges') : t('m9SaveDraft')}</button>
        {diary?.status !== 'published' && <button type="button" className={ui.primary} disabled={busy || empty} onClick={() => void publish()}>{t('m9PublishDiary')}</button>}
        {diary?.status === 'draft' && <button type="button" className={`${ui.secondary} text-rose-700`} disabled={busy} onClick={() => { if (window.confirm(t('m9ConfirmDelete'))) void run(async () => { await authorized(token => deleteDiary(token, diary.id)); setDiaryId(null); setForm(emptyInput); setFiles([]) }, 'm9DeletedOk') }}>{t('m9Delete')}</button>}
      </div>
      {empty && diary?.status !== 'published' && <p className="text-xs text-slate-500">{t('m9PublishNeedsContent')}</p>}
    </form>
    {diary && (diary.submission_enabled || diary.submission_count > 0) && diary.status === 'published' && <SubmissionsPanel diaryId={diary.id} />}
  </>
}

function SubmissionsPanel({ diaryId }: { diaryId: string }) {
  const { t } = useTranslation()
  const authorized = useApi()
  const local = useLocal()
  const fmt = useDhakaFormat()
  const errorText = useErrorText()
  const list = useQuery({ queryKey: ['m9', 'diary', 'submissions', diaryId], queryFn: () => authorized(token => getSubmissions(token, diaryId)), retry: false })
  const [notes, setNotes] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const submitted = list.data?.filter(s => s.status !== 'missing').length ?? 0
  async function review(s: Submission) {
    setError('')
    try { await authorized(token => reviewSubmission(token, s.id, notes[s.id] ?? s.review_note)); await list.refetch() } catch (cause) { setError(errorText(cause)) }
  }
  return <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="subs-title">
    <h2 id="subs-title" className="text-lg font-bold">{t('m9Submissions')}</h2>
    {list.data && <p className="text-sm text-slate-600">{t('m9SubmittedCount', { submitted: fmt.number(submitted), total: fmt.number(list.data.length) })}</p>}
    {error && <div className="mt-2"><Banner tone="error">{error}</Banner></div>}
    {list.isPending ? <div className="mt-3"><Skeleton rows={3} /></div> : list.isError ? <Banner tone="error">{t('m9ErrGeneric')}</Banner> :
      <ul className="mt-3 divide-y divide-slate-100">{list.data.map(s => <li key={s.student_id} className="py-3">
        <div className="flex flex-wrap items-center justify-between gap-2"><p className="font-semibold">{s.roll_no != null && <span className="mr-2 font-mono text-slate-400">{fmt.number(s.roll_no)}</span>}{local(s.student_name, s.student_name_bn)}</p>
          <StatusBadge tone={s.status === 'reviewed' ? 'present' : s.status === 'submitted' ? 'late' : 'unmarked'}>{t(`m9Sub_${s.status}`)}{s.submitted_at ? ` · ${fmt.dateTime(s.submitted_at)}` : ''}</StatusBadge></div>
        {s.status !== 'missing' && <>
          {s.answer_text && <p className="mt-2 whitespace-pre-line rounded-lg bg-slate-50 p-3 text-sm">{s.answer_text}</p>}
          <AttachmentList items={s.files} pathFor={f => `/diary/submissions/${s.id}/files/${f.id}`} />
          <div className="mt-2 flex flex-col gap-2 sm:flex-row"><label className="flex-1"><span className="sr-only">{t('m9ReviewNote')}</span><input className={`${ui.field} mt-0`} placeholder={t('m9ReviewNote')} maxLength={2000} value={notes[s.id] ?? s.review_note} onChange={e => setNotes({ ...notes, [s.id]: e.target.value })} /></label>
            <button type="button" className={ui.secondary} onClick={() => void review(s)}>{s.status === 'reviewed' ? t('m9UpdateReview') : t('m9MarkReviewed')}</button></div>
        </>}
      </li>)}</ul>}
  </section>
}

export function DiaryFeedPage({ mode }: { mode: 'student' | 'guardian' }) {
  const { t } = useTranslation()
  const { id: childId = '' } = useParams()
  const authorized = useApi()
  const local = useLocal()
  const fmt = useDhakaFormat()
  const [from, setFrom] = useState(shiftDay(todayDhaka(), -13))
  const [to, setTo] = useState(todayDhaka())
  const [subject, setSubject] = useState('')
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const params = new URLSearchParams({ page: String(page), page_size: '20', from, to, search, ...(subject ? { subject_id: subject } : {}) })
  const feed = useQuery({ queryKey: ['m9', 'diary-feed', mode, childId, params.toString()], queryFn: () => authorized(token => getDiaryFeed(token, mode === 'guardian' ? childId : null, params)), retry: false })
  const [subjects, setSubjects] = useState<Record<string, string>>({})
  useEffect(() => { if (feed.data) setSubjects(prev => { const next = { ...prev }; feed.data.items.forEach(d => { next[d.subject_id] = local(d.subject_name_en, d.subject_name_bn) }); return next }) }, [feed.data]) // eslint-disable-line react-hooks/exhaustive-deps
  const byDate = new Map<string, Diary[]>()
  feed.data?.items.forEach(d => byDate.set(d.diary_date, [...(byDate.get(d.diary_date) ?? []), d]))
  const filePath = (d: Diary) => (f: Attachment) => `/diary/${d.id}/files/${f.id}${mode === 'guardian' ? `?student_id=${childId}` : ''}`
  return <div className="space-y-5">
    <PageHeader eyebrow={t('m9DiaryEyebrow')} title={t(mode === 'student' ? 'm9MyDiary' : 'm9ChildDiary')} description={t('m9DiaryFeedIntro')} />
    <section className={`${ui.card} grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-4`} aria-label={t('m9Filters')}>
      <Field label={t('m9From')}><input type="date" className={ui.field} value={from} max={to} onChange={e => { setFrom(e.target.value); setPage(1) }} /></Field>
      <Field label={t('m9To')}><input type="date" className={ui.field} value={to} min={from} onChange={e => { setTo(e.target.value); setPage(1) }} /></Field>
      <Field label={t('m9Subject')}><select className={ui.field} value={subject} onChange={e => { setSubject(e.target.value); setPage(1) }}><option value="">{t('m9AllSubjects')}</option>{Object.entries(subjects).map(([id, name]) => <option key={id} value={id}>{name}</option>)}</select></Field>
      <Field label={t('m9Search')}><input type="search" className={ui.field} value={search} onChange={e => { setSearch(e.target.value); setPage(1) }} /></Field>
    </section>
    {feed.isPending ? <Skeleton rows={4} /> : feed.isError ? <Banner tone="error">{feed.error instanceof ApiError && feed.error.status === 404 ? t('m9ErrNotFound') : t('m9ErrGeneric')}</Banner> : feed.data.items.length === 0 ? <EmptyState title={t('m9NoDiaries')} /> : <>
      {[...byDate.entries()].map(([day, list]) => <section key={day} aria-labelledby={`day-${day}`} className="space-y-3">
        <h2 id={`day-${day}`} className="text-sm font-bold uppercase tracking-wide text-slate-500">{fmt.date(day)}</h2>
        {list.map(d => <article key={d.id} className={`${ui.card} p-4 sm:p-5`}>
          <div className="flex flex-wrap items-start justify-between gap-2"><div><h3 className="text-lg font-bold">{local(d.subject_name_en, d.subject_name_bn)}</h3><p className="text-xs text-slate-500">{d.teacher_name} · {d.class_name} {d.section_name}</p></div>
            {d.submission_enabled && <StatusBadge tone={d.my_submission_status === 'reviewed' ? 'present' : d.my_submission_status ? 'late' : d.submission_open ? 'absent' : 'unmarked'}>{t(d.my_submission_status ? `m9Sub_${d.my_submission_status}` : d.submission_open ? 'm9SubmissionDue' : 'm9SubmissionClosed')}</StatusBadge>}</div>
          <div className="mt-3"><DiaryBody diary={d} filePath={filePath(d)} /></div>
          {d.submission_enabled && mode === 'student' && <HomeworkPanel diary={d} />}
          {d.submission_enabled && d.submission_due_at && <p className="mt-2 text-xs text-slate-500">{t('m9DueAt')}: {fmt.dateTime(d.submission_due_at)}</p>}
        </article>)}
      </section>)}
      <Pager page={page} total={feed.data.total} size={20} onPage={setPage} previous={t('m9Previous')} next={t('m9Next')} label={t('m9MyDiary')} />
    </>}
  </div>
}

function HomeworkPanel({ diary }: { diary: Diary }) {
  const { t } = useTranslation()
  const authorized = useApi()
  const client = useQueryClient()
  const errorText = useErrorText()
  const mine = useQuery({ queryKey: ['m9', 'diary', 'mine', diary.id], enabled: !!diary.my_submission_status, queryFn: () => authorized(token => getMySubmission(token, diary.id)), retry: false })
  const [open, setOpen] = useState(false)
  const [text, setText] = useState('')
  const [files, setFiles] = useState<Attachment[]>([])
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const sub = mine.data
  const canEdit = diary.submission_open && sub?.status !== 'reviewed'
  function start() { setText(sub?.answer_text ?? ''); setFiles(sub?.files ?? []); setOpen(true); setMessage(null) }
  async function submit(event: FormEvent) {
    event.preventDefault(); setBusy(true); setMessage(null)
    try {
      await authorized(token => submitHomework(token, diary.id, { answer_text: text, file_ids: files.map(f => f.id) }))
      await client.invalidateQueries({ queryKey: ['m9', 'diary'] }); await client.invalidateQueries({ queryKey: ['m9', 'diary-feed'] })
      setOpen(false); setMessage({ tone: 'success', text: t('m9HomeworkSubmitted') })
    } catch (cause) { setMessage({ tone: 'error', text: errorText(cause) }) } finally { setBusy(false) }
  }
  return <div className="mt-4 rounded-xl border border-dashed border-slate-300 p-3">
    {message && <div className="mb-2"><Banner tone={message.tone}>{message.text}</Banner></div>}
    {sub && !open && <div className="text-sm"><p className="font-semibold">{t('m9YourSubmission')}</p>{sub.answer_text && <p className="mt-1 whitespace-pre-line">{sub.answer_text}</p>}
      <AttachmentList items={sub.files} pathFor={f => `/diary/submissions/${sub.id}/files/${f.id}`} />{sub.review_note && <p className="mt-2 rounded-lg bg-emerald-50 p-2 text-emerald-900">{t('m9TeacherNote')}: {sub.review_note}</p>}</div>}
    {open ? <form onSubmit={submit} className="space-y-3">
      <Field label={t('m9YourAnswer')}><textarea rows={4} className={ui.field} maxLength={10000} value={text} onChange={e => setText(e.target.value)} /></Field>
      <AttachmentList items={files} pathFor={f => sub?.files.some(x => x.id === f.id) ? `/diary/submissions/${sub.id}/files/${f.id}` : ''} onRemove={id => setFiles(files.filter(f => f.id !== id))} />
      <FileUploader path="/diary/homework/files" max={5 - files.length} onUploaded={f => setFiles(prev => [...prev, f])} />
      <div className="flex gap-2"><button type="submit" className={ui.primary} disabled={busy || (!text.trim() && files.length === 0)}>{busy ? t('m9Submitting') : t('m9SubmitHomework')}</button><button type="button" className={ui.secondary} onClick={() => setOpen(false)}>{t('m9Cancel')}</button></div>
    </form> : canEdit ? <button type="button" className={`${ui.secondary} ${sub ? 'mt-2' : ''}`} onClick={start}>{sub ? t('m9EditSubmission') : t('m9SubmitHomework')}</button> : !sub && <p className="text-sm text-slate-500">{t('m9SubmissionClosedHint')}</p>}
  </div>
}

export function AdminDiaryPage({ canManage }: { canManage: boolean }) {
  const { t } = useTranslation()
  const authorized = useApi()
  const local = useLocal()
  const fmt = useDhakaFormat()
  const errorText = useErrorText()
  const [date, setDate] = useState(todayDhaka())
  const [page, setPage] = useState(1)
  const [openId, setOpenId] = useState('')
  const settings = useQuery({ queryKey: ['m9', 'diary-settings'], enabled: canManage, queryFn: () => authorized(token => getDiarySettings(token)), retry: false })
  const [draft, setDraft] = useState<{ daily_required: boolean; reminder_time: string } | null>(null)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const compliance = useQuery({ queryKey: ['m9', 'compliance', date], queryFn: () => authorized(token => getDiaryCompliance(token, date)), retry: false })
  const params = new URLSearchParams({ page: String(page), page_size: '15', from: date, to: date })
  const all = useQuery({ queryKey: ['m9', 'diary-all', params.toString()], queryFn: () => authorized(token => getAllDiaries(token, params)), retry: false })
  const current = draft ?? settings.data
  const done = compliance.data?.filter(r => r.diary_status === 'published').length ?? 0
  async function saveSettings() {
    if (!current) return
    try { setDraft(await authorized(token => saveDiarySettings(token, current))); setMessage({ tone: 'success', text: t('m9Saved') }) } catch (cause) { setMessage({ tone: 'error', text: errorText(cause) }) }
  }
  return <div className="space-y-5">
    <PageHeader eyebrow={t('m9DiaryEyebrow')} title={t('m9DiaryOversight')} description={t('m9DiaryOversightIntro')} actions={<label className={ui.label}>{t('m9Date')}<input type="date" className={ui.field} value={date} onChange={e => { if (e.target.value) { setDate(e.target.value); setPage(1) } }} /></label>} />
    {canManage && current && <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="diary-settings">
      <h2 id="diary-settings" className="text-lg font-bold">{t('m9DiarySettings')}</h2>
      {message && <div className="mt-2"><Banner tone={message.tone}>{message.text}</Banner></div>}
      <div className="mt-3 flex flex-col gap-4 sm:flex-row sm:items-end">
        <label className="flex items-start gap-3 text-sm"><input type="checkbox" className="mt-1" checked={current.daily_required} onChange={e => setDraft({ ...current, daily_required: e.target.checked })} /><span><span className="font-medium">{t('m9DailyRequired')}</span><span className="block text-xs text-slate-500">{t('m9DailyRequiredHint')}</span></span></label>
        <Field label={t('m9ReminderTime')}><input type="time" className={ui.field} value={current.reminder_time} onChange={e => setDraft({ ...current, reminder_time: e.target.value })} /></Field>
        <button type="button" className={ui.primary} disabled={!draft} onClick={() => void saveSettings()}>{t('m9Save')}</button>
      </div></section>}
    <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="compliance-title">
      <h2 id="compliance-title" className="text-lg font-bold">{t('m9Compliance')}</h2>
      {compliance.data && <p className="text-sm text-slate-600">{t('m9DiaryProgress', { published: fmt.number(done), total: fmt.number(compliance.data.length) })}</p>}
      {compliance.isPending ? <div className="mt-3"><Skeleton rows={3} /></div> : compliance.isError ? <Banner tone="error">{t('m9ErrGeneric')}</Banner> : compliance.data.length === 0 ? <div className="mt-3"><EmptyState title={t('m9NoAssignments')} /></div> :
        <div className="mt-3 overflow-x-auto"><table className="w-full min-w-[32rem] text-left text-sm"><thead><tr className="border-b text-xs uppercase tracking-wide text-slate-500"><th className="py-2">{t('m9ClassSection')}</th><th>{t('m9Subject')}</th><th>{t('m9Teachers')}</th><th>{t('m9Status')}</th></tr></thead>
          <tbody>{compliance.data.map((r, i) => <tr key={i} className="border-b border-slate-100"><td className="py-2.5">{r.class_name} · {r.section_name}</td><td>{local(r.subject_name_en, r.subject_name_bn)}</td><td className="text-slate-600">{r.teachers}</td><td><DiaryStatus status={r.diary_status} /></td></tr>)}</tbody></table></div>}
    </section>
    <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="all-title">
      <h2 id="all-title" className="text-lg font-bold">{t('m9DiariesForDay')}</h2>
      {all.isPending ? <div className="mt-3"><Skeleton rows={3} /></div> : all.isError ? <Banner tone="error">{t('m9ErrGeneric')}</Banner> : all.data.items.length === 0 ? <div className="mt-3"><EmptyState title={t('m9NoDiaries')} /></div> : <>
        <ul className="mt-3 divide-y divide-slate-100">{all.data.items.map(d => <li key={d.id} className="py-3">
          <button type="button" className="flex w-full flex-wrap items-center justify-between gap-2 text-left" aria-expanded={openId === d.id} onClick={() => setOpenId(openId === d.id ? '' : d.id)}>
            <span><span className="font-semibold">{local(d.subject_name_en, d.subject_name_bn)}</span><span className="block text-xs text-slate-500">{d.class_name} · {d.section_name} · {d.teacher_name}</span></span>
            <span className="flex items-center gap-2"><DiaryStatus status={d.status} />{d.submission_enabled && <span className="text-xs text-slate-500">{t('m9SubmittedCount', { submitted: fmt.number(d.submission_count), total: '—' })}</span>}</span></button>
          {openId === d.id && <div className="mt-3"><DiaryBody diary={d} filePath={f => `/diary/${d.id}/files/${f.id}`} /></div>}
        </li>)}</ul>
        <Pager page={page} total={all.data.total} size={15} onPage={setPage} previous={t('m9Previous')} next={t('m9Next')} label={t('m9DiariesForDay')} />
      </>}
    </section>
  </div>
}
