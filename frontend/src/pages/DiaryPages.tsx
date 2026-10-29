import { confirmAction } from '../components/ConfirmDialog'
import { useEffect, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router'
import { deleteDiary, getAllDiaries, getDiary, getDiaryAssignments, getDiaryCompliance, getDiaryFeed, getDiarySettings, getMySubmission, getSubmissions, publishDiary, reviewSubmission, saveDiary, saveDiarySettings, submitHomework, type Attachment, type Diary, type DiaryAssignment, type DiaryInput, type Submission } from '../api/communication'
import { ApiError } from '../api/client'
import { CardTitle, EmptyState, Notice as Banner, PageHeader, Pager, Skeleton, StatusBadge, ui } from './attendanceUi'
import { AttachmentList, Field, FileUploader, isoToLocal, localToIso, shiftDay, todayDhaka, useApi, useDhakaFormat, useErrorText, useLocal } from './commShared'
import { Icon, type IconName } from '../components/icons'
import type { Tone } from './attendanceUi'

const sections: { key: keyof Pick<Diary, 'lesson_topics' | 'class_notes' | 'homework' | 'instructions'>; label: string }[] = [
  { key: 'lesson_topics', label: 'm9LessonTopics' }, { key: 'class_notes', label: 'm9ClassNotes' }, { key: 'homework', label: 'm9Homework' }, { key: 'instructions', label: 'm9Instructions' },
]

// Each diary section gets its own tinted panel so lessons, notes, homework and
// instructions read as distinct blocks; the tints have dark-theme mappings.
const sectionLook: Record<(typeof sections)[number]['key'], { icon: IconName; box: string; title: string }> = {
  lesson_topics: { icon: 'book', box: 'border-[color-mix(in_srgb,var(--brand-primary)_18%,transparent)] bg-[var(--brand-soft)]', title: 'text-[var(--brand-ink)]' },
  class_notes: { icon: 'notebook', box: 'border-slate-200 bg-slate-50', title: 'text-slate-600' },
  homework: { icon: 'clipboard', box: 'border-amber-200 bg-amber-50', title: 'text-amber-800' },
  instructions: { icon: 'info', box: 'border-sky-200 bg-sky-50', title: 'text-sky-800' },
}

function DiaryBody({ diary, filePath }: { diary: Diary; filePath: (f: Attachment) => string }) {
  const { t } = useTranslation()
  const fmt = useDhakaFormat()
  const present = sections.filter(s => diary[s.key]?.trim())
  const lesson = present.filter(s => s.key === 'lesson_topics' || s.key === 'class_notes')
  const block = (s: (typeof sections)[number]) => <div key={s.key} className={`min-w-0 rounded-xl border p-4 ${sectionLook[s.key].box}`}>
    <h4 className={`flex items-center gap-2 text-xs font-bold uppercase tracking-wide ${sectionLook[s.key].title}`}><Icon name={sectionLook[s.key].icon} className="h-4 w-4 shrink-0" />{t(s.label)}</h4>
    <p className="mt-2 whitespace-pre-line break-words text-[0.9375rem] leading-7 text-slate-800">{diary[s.key]}</p>
  </div>
  return <div className="space-y-3">
    {lesson.length > 0 && <div className={`grid gap-3 ${lesson.length === 2 ? 'md:grid-cols-2' : ''}`}>{lesson.map(block)}</div>}
    {present.filter(s => !lesson.includes(s)).map(block)}
    {diary.attachments.length > 0 && <div>
      <h4 className="mb-2 flex items-center gap-2 text-xs font-bold uppercase tracking-wide text-slate-500"><Icon name="file" className="h-4 w-4" />{t('m9Attachments')} <span className="rounded-full bg-slate-100 px-2 py-0.5 text-[0.7rem] text-slate-600">{fmt.number(diary.attachments.length)}</span></h4>
      <AttachmentList variant="cards" items={diary.attachments} pathFor={filePath} /></div>}
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
        <div><CardTitle id="diary-day" icon="notebook">{fmt.date(date)}</CardTitle>{assignments.data && <p className="text-sm text-slate-600">{t('m9DiaryProgress', { published: fmt.number(published), total: fmt.number(assignments.data.length) })}</p>}</div>
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
  const [uploading, setUploading] = useState(false)
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
      <div className="flex flex-wrap items-center justify-between gap-2"><div><CardTitle id="diary-editor" icon="notebook">{local(assignment.subject_name_en, assignment.subject_name_bn)} · {assignment.class_name} {assignment.section_name}</CardTitle><p className="text-sm text-slate-600">{assignment.session_name}</p></div><DiaryStatus status={diary?.status ?? null} /></div>
      {message && <Banner tone={message.tone}>{message.text}</Banner>}
      <div className="grid gap-4 md:grid-cols-2">
        {sections.map(s => <Field key={s.key} label={t(s.label)}><textarea rows={s.key === 'class_notes' ? 5 : 3} className={ui.field} value={form[s.key]} placeholder={t(`${s.label}Hint`)} onChange={e => setForm({ ...form, [s.key]: e.target.value })} /></Field>)}
      </div>
      <div><p className={ui.label}>{t('m9Attachments')}</p><AttachmentList items={files} pathFor={f => diaryId && diary?.attachments.some(a => a.id === f.id) ? `/diary/${diaryId}/files/${f.id}` : ''} onRemove={id => setFiles(files.filter(f => f.id !== id))} />
        <div className="mt-2"><FileUploader path="/diary/files" max={10 - files.length} onBusy={setUploading} onUploaded={f => setFiles(prev => [...prev, f])} /></div></div>
      <fieldset className="rounded-xl border border-slate-200 p-4"><legend className="px-1 text-sm font-semibold">{t('m9HomeworkSubmission')}</legend>
        <label className="flex items-start gap-3 text-sm"><input type="checkbox" className="mt-1" checked={form.submission_enabled} onChange={e => setForm({ ...form, submission_enabled: e.target.checked })} /><span><span className="font-medium">{t('m9AllowSubmission')}</span><span className="block text-xs text-slate-500">{t('m9AllowSubmissionHint')}</span></span></label>
        {form.submission_enabled && <div className="mt-3 max-w-xs"><Field label={t('m9DueAt')}><input type="datetime-local" className={ui.field} value={due} onChange={e => setDue(e.target.value)} /></Field></div>}
      </fieldset>
      <div className="flex flex-wrap gap-2 border-t border-slate-100 pt-4">
        <button type="submit" className={ui.secondary} disabled={busy || uploading}>{diary?.status === 'published' ? t('m9SaveChanges') : t('m9SaveDraft')}</button>
        {diary?.status !== 'published' && <button type="button" className={ui.primary} disabled={busy || uploading || empty} onClick={() => void publish()}>{t('m9PublishDiary')}</button>}
        {diary?.status === 'draft' && <button type="button" className={`${ui.secondary} text-rose-700`} disabled={busy} onClick={async () => { if ((await confirmAction(t('m9ConfirmDelete')))) void run(async () => { await authorized(token => deleteDiary(token, diary.id)); setDiaryId(null); setForm(emptyInput); setFiles([]) }, 'm9DeletedOk') }}>{t('m9Delete')}</button>}
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
    <CardTitle id="subs-title" icon="clipboard">{t('m9Submissions')}</CardTitle>
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
  const today = todayDhaka()
  const defaultFrom = shiftDay(today, -13)
  const filtered = from !== defaultFrom || to !== today || subject !== '' || search !== ''
  function clearFilters() { setFrom(defaultFrom); setTo(today); setSubject(''); setSearch(''); setPage(1) }
  const searchField = ui.field.replace('mt-1.5 ', '').replace('px-3', 'pl-10 pr-3')
  return <div className="space-y-6">
    <PageHeader eyebrow={t('m9DiaryEyebrow')} title={t(mode === 'student' ? 'm9MyDiary' : 'm9ChildDiary')} description={t('m9DiaryFeedIntro')} />
    <section className={`${ui.card} p-4 sm:p-5`} aria-label={t('m9Filters')}>
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-[minmax(0,1.7fr)_minmax(0,1.1fr)_minmax(0,1fr)_minmax(0,1fr)]">
        <Field label={t('m9Search')} className="sm:col-span-2 xl:col-span-1"><span className="relative mt-1.5 block">
          <Icon name="search" className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
          <input type="search" className={searchField} placeholder={t('m9DiarySearchHint')} value={search} onChange={e => { setSearch(e.target.value); setPage(1) }} /></span></Field>
        <Field label={t('m9Subject')}><select className={ui.field} value={subject} onChange={e => { setSubject(e.target.value); setPage(1) }}><option value="">{t('m9AllSubjects')}</option>{Object.entries(subjects).map(([id, name]) => <option key={id} value={id}>{name}</option>)}</select></Field>
        <div className="grid grid-cols-2 gap-3 sm:col-span-1 xl:col-span-2">
          <Field label={t('m9From')}><input type="date" className={ui.field} value={from} max={to} onChange={e => { setFrom(e.target.value); setPage(1) }} /></Field>
          <Field label={t('m9To')}><input type="date" className={ui.field} value={to} min={from} onChange={e => { setTo(e.target.value); setPage(1) }} /></Field>
        </div>
      </div>
      <div className="mt-4 flex min-h-9 flex-wrap items-center justify-between gap-2 border-t border-slate-100 pt-3">
        <p className="text-sm font-medium text-slate-600" aria-live="polite">{feed.data ? t('m9DiaryEntries', { count: feed.data.total, n: fmt.number(feed.data.total) }) : ''}</p>
        {filtered && <button type="button" className={ui.ghost} onClick={clearFilters}><Icon name="close" className="h-4 w-4" />{t('m9DiaryClearFilters')}</button>}
      </div>
    </section>
    {feed.isPending ? <Skeleton rows={4} /> : feed.isError ? <Banner tone="error">{feed.error instanceof ApiError && feed.error.status === 404 ? t('m9ErrNotFound') : t('m9ErrGeneric')}</Banner> : feed.data.items.length === 0 ? <EmptyState title={t('m9NoDiaries')} /> : <>
      {[...byDate.entries()].map(([day, list]) => <section key={day} aria-labelledby={`day-${day}`} className="space-y-4">
        <div className="flex items-center gap-3">
          <h2 id={`day-${day}`} className="flex shrink-0 items-center gap-2 text-sm font-bold text-slate-700"><Icon name="calendar" className="h-4 w-4 text-slate-400" />{fmt.date(day)}</h2>
          {(day === today || day === shiftDay(today, -1)) && <span className="shrink-0 rounded-full bg-[var(--brand-soft)] px-2.5 py-0.5 text-xs font-semibold text-[var(--brand-ink)]">{t(day === today ? 'm9DiaryToday' : 'm9DiaryYesterday')}</span>}
          <span aria-hidden="true" className="flex-1 border-t border-slate-200" />
        </div>
        {list.map(d => <DiaryCard key={d.id} diary={d} mode={mode} filePath={filePath(d)} />)}
      </section>)}
      <Pager page={page} total={feed.data.total} size={20} onPage={setPage} previous={t('m9Previous')} next={t('m9Next')} label={t('m9MyDiary')} />
    </>}
  </div>
}

// The badge says where the homework stands for this student: due, submitted,
// reviewed, closed, set without online submission, or none at all.
function homeworkStatus(d: Diary): { tone: Tone; label: string } {
  if (d.submission_enabled) {
    if (d.my_submission_status === 'reviewed') return { tone: 'present', label: 'm9Sub_reviewed' }
    if (d.my_submission_status === 'submitted') return { tone: 'leave', label: 'm9Sub_submitted' }
    return d.submission_open ? { tone: 'late', label: 'm9SubmissionDue' } : { tone: 'unmarked', label: 'm9SubmissionClosed' }
  }
  return d.homework.trim() ? { tone: 'leave', label: 'm9DiaryHomeworkSet' } : { tone: 'unmarked', label: 'm9DiaryNoHomework' }
}

function DiaryCard({ diary: d, mode, filePath }: { diary: Diary; mode: 'student' | 'guardian'; filePath: (f: Attachment) => string }) {
  const { t } = useTranslation()
  const local = useLocal()
  const fmt = useDhakaFormat()
  const status = homeworkStatus(d)
  const meta = 'flex min-w-0 items-center gap-1.5'
  return <article className={`${ui.card} overflow-hidden`} aria-labelledby={`diary-${d.id}`}>
    <header className="flex flex-wrap items-start gap-x-4 gap-y-3 px-4 pb-3 pt-4 sm:px-6 sm:pt-5">
      <span aria-hidden="true" className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-[var(--brand-soft)] text-[var(--brand-ink)]"><Icon name="book" className="h-5 w-5" /></span>
      <div className="min-w-0 flex-1">
        <h3 id={`diary-${d.id}`} className="text-lg font-bold leading-snug text-slate-900">{local(d.subject_name_en, d.subject_name_bn)}</h3>
        <dl className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-sm text-slate-600">
          <div className={meta}><dt><Icon name="user" className="h-4 w-4 text-slate-400" /><span className="sr-only">{t('m9DiaryTeacher')}</span></dt><dd className="truncate">{d.teacher_name}</dd></div>
          <div className={meta}><dt><Icon name="school" className="h-4 w-4 text-slate-400" /><span className="sr-only">{t('m9DiaryClassSection')}</span></dt><dd>{d.class_name} · {d.section_name}</dd></div>
          <div className={meta}><dt><Icon name="calendar" className="h-4 w-4 text-slate-400" /><span className="sr-only">{t('m9Date')}</span></dt><dd>{fmt.date(d.diary_date)}</dd></div>
        </dl>
      </div>
      <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto sm:flex-col sm:items-end">
        <StatusBadge tone={status.tone}>{t(status.label)}</StatusBadge>
        {d.submission_enabled && d.submission_due_at && <span className="flex items-center gap-1 text-xs font-medium text-slate-600"><Icon name="clock" className="h-3.5 w-3.5" />{t('m9DiaryDueOn', { date: fmt.dateTime(d.submission_due_at) })}</span>}
      </div>
    </header>
    <div className="px-4 pb-4 sm:px-6 sm:pb-5"><DiaryBody diary={d} filePath={filePath} /></div>
    {d.submission_enabled && mode === 'student' && <HomeworkPanel diary={d} />}
  </article>
}

function HomeworkPanel({ diary }: { diary: Diary }) {
  const { t } = useTranslation()
  const authorized = useApi()
  const client = useQueryClient()
  const errorText = useErrorText()
  const fmt = useDhakaFormat()
  const mine = useQuery({ queryKey: ['m9', 'diary', 'mine', diary.id], enabled: !!diary.my_submission_status, queryFn: () => authorized(token => getMySubmission(token, diary.id)), retry: false })
  const [open, setOpen] = useState(false)
  const [text, setText] = useState('')
  const [files, setFiles] = useState<Attachment[]>([])
  const [busy, setBusy] = useState(false)
  const [uploading, setUploading] = useState(false)
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
  const submissionPath = (f: Attachment) => sub ? `/diary/submissions/${sub.id}/files/${f.id}` : ''
  return <div className="border-t border-slate-100 bg-slate-50 px-4 py-4 sm:px-6 sm:py-5">
    {message && <div className="mb-3"><Banner tone={message.tone}>{message.text}</Banner></div>}
    {open ? <form onSubmit={submit} className="space-y-4 rounded-xl border border-[color-mix(in_srgb,var(--brand-primary)_22%,transparent)] bg-white p-4 shadow-sm sm:p-5" aria-labelledby={`hw-form-${diary.id}`}>
      <h4 id={`hw-form-${diary.id}`} className="flex items-center gap-2 text-base font-bold text-slate-900"><Icon name="send" className="h-5 w-5 text-[var(--brand-ink)]" />{sub ? t('m9EditSubmission') : t('m9SubmitHomework')}</h4>
      <Field label={t('m9YourAnswer')}><textarea rows={5} className={ui.field} maxLength={10000} value={text} onChange={e => setText(e.target.value)} /></Field>
      <AttachmentList variant="cards" items={files} pathFor={f => sub?.files.some(x => x.id === f.id) ? submissionPath(f) : ''} onRemove={id => setFiles(files.filter(f => f.id !== id))} />
      <FileUploader path="/diary/homework/files" max={5 - files.length} onBusy={setUploading} onUploaded={f => setFiles(prev => [...prev, f])} />
      <div className="flex flex-col-reverse gap-2 border-t border-slate-100 pt-4 sm:flex-row sm:justify-end">
        <button type="button" className={`${ui.secondary} min-h-12`} onClick={() => setOpen(false)}>{t('m9Cancel')}</button>
        <button type="submit" className={cta} disabled={busy || uploading || (!text.trim() && files.length === 0)}><Icon name="send" className="h-5 w-5" />{busy ? t('m9Submitting') : t('m9SubmitHomework')}</button>
      </div>
    </form> : sub ? <div className="rounded-xl border border-emerald-200 bg-white p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3"><span aria-hidden="true" className="grid h-10 w-10 place-items-center rounded-full bg-emerald-50 text-emerald-700"><Icon name="check" className="h-5 w-5" /></span>
          <div><p className="font-bold text-slate-900">{t('m9YourSubmission')}</p>{sub.submitted_at && <p className="text-xs text-slate-500">{t('m9SubmittedOn', { date: fmt.dateTime(sub.submitted_at) })}</p>}</div></div>
        {canEdit && <button type="button" className={ui.secondary} onClick={start}>{t('m9EditSubmission')}</button>}
      </div>
      {sub.answer_text && <p className="mt-3 whitespace-pre-line break-words rounded-lg bg-slate-50 p-3 text-sm leading-6 text-slate-800">{sub.answer_text}</p>}
      {sub.files.length > 0 && <div className="mt-3"><AttachmentList variant="cards" items={sub.files} pathFor={submissionPath} /></div>}
      {sub.review_note && <div className="mt-3 rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm"><p className="text-xs font-bold uppercase tracking-wide text-emerald-800">{t('m9TeacherNote')}</p><p className="mt-1 whitespace-pre-line text-emerald-900">{sub.review_note}</p></div>}
    </div> : diary.my_submission_status && mine.isPending ? <Skeleton rows={1} /> : canEdit ? <div className="flex flex-col gap-4 rounded-xl border border-[color-mix(in_srgb,var(--brand-primary)_22%,transparent)] bg-[var(--brand-soft)] p-4 sm:flex-row sm:items-center sm:p-5">
      <span aria-hidden="true" className="hidden h-12 w-12 shrink-0 place-items-center rounded-xl bg-white text-[var(--brand-ink)] shadow-sm sm:grid"><Icon name="clipboard" className="h-6 w-6" /></span>
      <div className="min-w-0 flex-1"><p className="font-bold text-slate-900">{t('m9SubmitCtaTitle')}</p><p className="mt-0.5 text-sm text-slate-600">{t('m9SubmitCtaHint')}</p></div>
      <button type="button" className={cta} onClick={start}><Icon name="send" className="h-5 w-5" />{t('m9SubmitHomework')}</button>
    </div> : <p className="flex items-center gap-2 text-sm text-slate-600"><Icon name="lock" className="h-4 w-4 text-slate-400" />{t('m9SubmissionClosedHint')}</p>}
  </div>
}

const cta = 'inline-flex min-h-12 w-full items-center justify-center gap-2 rounded-xl bg-[var(--brand-primary)] px-6 text-base font-semibold text-[var(--brand-on)] shadow-md shadow-[color-mix(in_srgb,var(--brand-primary)_30%,transparent)] transition hover:brightness-110 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)] disabled:cursor-not-allowed disabled:opacity-50 disabled:shadow-none sm:w-auto'

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
      <CardTitle id="diary-settings" icon="settings">{t('m9DiarySettings')}</CardTitle>
      {message && <div className="mt-2"><Banner tone={message.tone}>{message.text}</Banner></div>}
      <div className="mt-3 flex flex-col gap-4 sm:flex-row sm:items-end">
        <label className="flex items-start gap-3 text-sm"><input type="checkbox" className="mt-1" checked={current.daily_required} onChange={e => setDraft({ ...current, daily_required: e.target.checked })} /><span><span className="font-medium">{t('m9DailyRequired')}</span><span className="block text-xs text-slate-500">{t('m9DailyRequiredHint')}</span></span></label>
        <Field label={t('m9ReminderTime')}><input type="time" className={ui.field} value={current.reminder_time} onChange={e => setDraft({ ...current, reminder_time: e.target.value })} /></Field>
        <button type="button" className={ui.primary} disabled={!draft} onClick={() => void saveSettings()}>{t('m9Save')}</button>
      </div></section>}
    <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="compliance-title">
      <CardTitle id="compliance-title" icon="check">{t('m9Compliance')}</CardTitle>
      {compliance.data && <p className="text-sm text-slate-600">{t('m9DiaryProgress', { published: fmt.number(done), total: fmt.number(compliance.data.length) })}</p>}
      {compliance.isPending ? <div className="mt-3"><Skeleton rows={3} /></div> : compliance.isError ? <Banner tone="error">{t('m9ErrGeneric')}</Banner> : compliance.data.length === 0 ? <div className="mt-3"><EmptyState title={t('m9NoAssignments')} /></div> :
        <div className="mt-3 overflow-x-auto"><table className="ds-cards w-full min-w-[32rem] text-left text-sm"><thead><tr className="border-b text-xs uppercase tracking-wide text-slate-500"><th className="py-2">{t('m9ClassSection')}</th><th>{t('m9Subject')}</th><th>{t('m9Teachers')}</th><th>{t('m9Status')}</th></tr></thead>
          <tbody>{compliance.data.map((r, i) => <tr key={i} className="border-b border-slate-100"><td className="py-2.5">{r.class_name} · {r.section_name}</td><td>{local(r.subject_name_en, r.subject_name_bn)}</td><td className="text-slate-600">{r.teachers}</td><td><DiaryStatus status={r.diary_status} /></td></tr>)}</tbody></table></div>}
    </section>
    <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="all-title">
      <CardTitle id="all-title" icon="notebook">{t('m9DiariesForDay')}</CardTitle>
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
