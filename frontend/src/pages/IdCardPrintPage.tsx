import { useEffect, useMemo, useState, type CSSProperties, type FormEvent } from 'react'
import { createPortal } from 'react-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { confirmAction } from '../components/ConfirmDialog'
import { CARD_HEIGHT_MM, CARD_WIDTH_MM, CardBack, CardFront, type CardLanguage } from '../components/IdCard'
import { Icon } from '../components/icons'
import { StateScreen } from '../components/StateScreen'
import { ApiError, deleteCardSignature, getAcademicClasses, getAcademicSections, getAcademicSessions, getCardSettings, getCardSignature, getSectionCards, getStudentCard, getStudentPhoto, reissueStudentCard, saveCardSettings, saveCardSignature, type CardData, type CardSchool, type CardSettings, type CardSettingsInput } from '../api/client'
import { usePeopleApi } from './peopleShared'
import { EmptyState, Notice, PageHeader, SectionCard, Skeleton, ui } from './attendanceUi'

type Layout = 'cr80' | 'a4'
type Deck = { cards: CardData[]; school: CardSchool; settings: CardSettings }

// A4 duplex sheet: 3 × 3 portrait cards with 6 mm gutters, centred.
const A4 = { width: 210, height: 297, columns: 3, rows: 3, gap: 6 }
const sheetLeft = (A4.width - (A4.columns * CARD_WIDTH_MM + (A4.columns - 1) * A4.gap)) / 2
const sheetTop = (A4.height - (A4.rows * CARD_HEIGHT_MM + (A4.rows - 1) * A4.gap)) / 2
const perSheet = A4.columns * A4.rows
// Backs are mirrored horizontally so each lands behind its front when the
// sheet is turned over on its long edge.
export function slotPosition(index: number, back: boolean) {
  const row = Math.floor(index / A4.columns), column = index % A4.columns
  const placed = back ? A4.columns - 1 - column : column
  return { left: sheetLeft + placed * (CARD_WIDTH_MM + A4.gap), top: sheetTop + row * (CARD_HEIGHT_MM + A4.gap) }
}

export function IdCardPrintPage({ mode }: { mode: 'single' | 'bulk' }) {
  const { id } = useParams()
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const [lang, setLang] = useState<CardLanguage>(i18n.language === 'bn' ? 'bn' : 'en')
  const [layout, setLayout] = useState<Layout>(mode === 'single' ? 'cr80' : 'a4')
  const [cropMarks, setCropMarks] = useState(true)
  const [sessionId, setSessionId] = useState('')
  const [classId, setClassId] = useState('')
  const [sectionId, setSectionId] = useState('')
  const [notice, setNotice] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)

  const sessions = useQuery({ queryKey: ['cards', 'sessions'], enabled: mode === 'bulk', queryFn: () => authorized(token => getAcademicSessions(token, new URLSearchParams({ page: '1', page_size: '100' }))), retry: false })
  const classes = useQuery({ queryKey: ['cards', 'classes'], enabled: mode === 'bulk', queryFn: () => authorized(token => getAcademicClasses(token, new URLSearchParams({ page: '1', page_size: '100' }))), retry: false })
  const sections = useQuery({ queryKey: ['cards', 'sections', sessionId, classId], enabled: mode === 'bulk' && !!sessionId && !!classId, queryFn: () => authorized(token => getAcademicSections(token, new URLSearchParams({ page: '1', page_size: '100', session_id: sessionId, class_id: classId }))), retry: false })
  const deck = useQuery<Deck>({
    queryKey: ['cards', 'deck', mode, id, sectionId],
    enabled: mode === 'single' ? !!id : !!sectionId,
    retry: false,
    queryFn: async () => {
      if (mode === 'single') { const card = await authorized(token => getStudentCard(token, id!)); return { cards: [card], school: card.school, settings: card.settings } }
      return authorized(token => getSectionCards(token, sectionId))
    },
  })
  const assets = useCardAssets(deck.data, authorized)

  async function reissue() {
    const student = deck.data?.cards[0]?.student
    if (!student || !(await confirmAction(t('idCardReissueConfirm', { name: student.name_en })))) return
    try { await authorized(token => reissueStudentCard(token, student.id)); await cache.invalidateQueries({ queryKey: ['cards', 'deck'] }); setNotice({ tone: 'success', text: t('idCardReissued') }) }
    catch { setNotice({ tone: 'error', text: t('peopleSaveError') }) }
  }

  const cards = deck.data?.cards ?? []
  const ready = !!deck.data && assets.ready && cards.length > 0
  return <div className="space-y-6">
    <PageHeader eyebrow={t('studentsHeading')} title={t(mode === 'single' ? 'idCardPrintOne' : 'idCardPrintBulk')} description={t('idCardPrintIntro')}
      actions={<Link to={mode === 'single' && id ? `/admin/students/${id}` : '/admin/students'} className={ui.secondary}><Icon name="chevronLeft" className="h-4 w-4" />{t(mode === 'single' ? 'profileHeading' : 'studentsHeading')}</Link>} />
    {notice && <Notice tone={notice.tone}>{notice.text}</Notice>}
    <SectionCard title={t('idCardPrintOptions')} id="card-options">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {mode === 'bulk' && <>
          <label className={ui.label}>{t('sessionLabel')}<select className={ui.field} value={sessionId} onChange={e => { setSessionId(e.target.value); setSectionId('') }}><option value="">{t('selectOption')}</option>{sessions.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
          <label className={ui.label}>{t('classLabel')}<select className={ui.field} value={classId} onChange={e => { setClassId(e.target.value); setSectionId('') }}><option value="">{t('selectOption')}</option>{classes.data?.items.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
          <label className={ui.label}>{t('sectionLabel')}<select className={ui.field} value={sectionId} disabled={!sessionId || !classId} onChange={e => setSectionId(e.target.value)}><option value="">{t('selectOption')}</option>{sections.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        </>}
        <label className={ui.label}>{t('idCardLanguage')}<select className={ui.field} value={lang} onChange={e => setLang(e.target.value as CardLanguage)}><option value="en">English</option><option value="bn">বাংলা</option></select></label>
      </div>
      <fieldset className="mt-5"><legend className={ui.label}>{t('idCardLayout')}</legend>
        <div className="mt-2 grid gap-3 sm:grid-cols-2">
          {(['cr80', 'a4'] as const).map(value => <label key={value} className={`flex cursor-pointer gap-3 rounded-xl border p-3 ${layout === value ? 'border-[var(--brand-primary)] bg-[var(--brand-soft)]' : 'border-slate-200'}`}>
            <input type="radio" name="card-layout" className="mt-1 accent-[var(--brand-primary)]" checked={layout === value} onChange={() => setLayout(value)} />
            <span><span className="block text-sm font-semibold text-slate-900">{t(value === 'cr80' ? 'idCardLayoutCr80' : 'idCardLayoutA4')}</span><span className="block text-sm text-slate-600">{t(value === 'cr80' ? 'idCardLayoutCr80Help' : 'idCardLayoutA4Help')}</span></span>
          </label>)}
        </div>
      </fieldset>
      {layout === 'a4' && <label className="mt-4 flex items-center gap-2 text-sm font-semibold text-slate-700"><input type="checkbox" className="h-4 w-4 accent-[var(--brand-primary)]" checked={cropMarks} onChange={e => setCropMarks(e.target.checked)} />{t('idCardCropMarks')}</label>}
      <div className="mt-5 flex flex-wrap items-center gap-3">
        <button type="button" className={ui.primary} disabled={!ready} onClick={() => window.print()} data-ready={ready ? 'true' : 'false'}><Icon name="print" className="h-4 w-4" />{t('idCardPrint')}</button>
        {mode === 'single' && deck.data && <button type="button" className={ui.secondary} onClick={() => void reissue()}>{t('idCardReissue')}</button>}
        <p className="text-sm text-slate-600" aria-live="polite">{deck.data ? (assets.ready ? t('idCardCount', { count: cards.length }) : t('idCardPreparing')) : ''}</p>
      </div>
      <p className="mt-3 text-sm text-slate-600">{t(layout === 'cr80' ? 'idCardDuplexCr80' : 'idCardDuplexA4')}</p>
    </SectionCard>

    <SectionCard title={t('idCardPreview')} id="card-preview">
      {mode === 'bulk' && !sectionId ? <EmptyState icon="users" title={t('idCardChooseSection')} />
        : deck.isPending ? <Skeleton rows={4} />
        : deck.isError ? <Notice tone="error">{deck.error instanceof ApiError && deck.error.status === 403 ? t('permissionDenied') : t('peopleLoadError')}</Notice>
        : cards.length === 0 ? <EmptyState icon="users" title={t('idCardNoStudents')} />
        : <div className="flex flex-wrap gap-8">{cards.map(card => <figure key={card.student.id} className="m-0">
            <div className="flex gap-3" style={{ zoom: 1.3 }}>
              <div className="rounded-[3mm] shadow-[0_8px_24px_rgb(15_23_42/0.18)]"><CardFront card={card} school={deck.data.school} settings={deck.data.settings} lang={lang} photoUrl={assets.photos[card.student.id]} signatureUrl={assets.signature} /></div>
              <div className="rounded-[3mm] shadow-[0_8px_24px_rgb(15_23_42/0.18)]"><CardBack card={card} school={deck.data.school} settings={deck.data.settings} lang={lang} /></div>
            </div>
            <figcaption className="mt-2 text-sm text-slate-600">{card.student.name_en} · {card.student.student_code || card.student.admission_no}</figcaption>
          </figure>)}</div>}
    </SectionCard>

    <CardSettingsPanel authorized={authorized} onSaved={() => void cache.invalidateQueries({ queryKey: ['cards'] })} />
    {ready && createPortal(<PrintSheets deck={deck.data!} lang={lang} layout={layout} cropMarks={cropMarks} photos={assets.photos} signature={assets.signature} />, document.body)}
  </div>
}

type Authorized = ReturnType<typeof usePeopleApi>

// Loads protected photos and the signature as object URLs; printing waits for them.
function useCardAssets(deck: Deck | undefined, authorized: Authorized) {
  const [photos, setPhotos] = useState<Record<string, string>>({})
  const [signature, setSignature] = useState<string>()
  const [ready, setReady] = useState(false)
  useEffect(() => {
    if (!deck) return
    let cancelled = false
    const urls: string[] = []
    setReady(false); setPhotos({}); setSignature(undefined)
    void (async () => {
      const queue = deck.cards.filter(card => card.student.has_photo)
      const loaded: Record<string, string> = {}
      await Promise.all(Array.from({ length: Math.min(6, queue.length) }, async () => {
        for (let next = queue.shift(); next; next = queue.shift()) {
          try { const url = URL.createObjectURL(await authorized(token => getStudentPhoto(token, next.student.id))); urls.push(url); loaded[next.student.id] = url } catch { /* the card shows its placeholder */ }
        }
      }))
      let sig: string | undefined
      if (deck.settings.has_signature) {
        try { sig = URL.createObjectURL(await authorized(token => getCardSignature(token))); urls.push(sig) } catch { /* printed without signature */ }
      }
      // URLs made after a cancelled load would otherwise never be released.
      if (cancelled) { urls.forEach(url => URL.revokeObjectURL(url)); return }
      setPhotos(loaded); setSignature(sig); setReady(true)
    })()
    return () => { cancelled = true; urls.forEach(url => URL.revokeObjectURL(url)) }
  }, [deck, authorized])
  return { photos, signature, ready }
}

function PrintSheets({ deck, lang, layout, cropMarks, photos, signature }: { deck: Deck; lang: CardLanguage; layout: Layout; cropMarks: boolean; photos: Record<string, string>; signature?: string }) {
  const { i18n } = useTranslation()
  const t = i18n.getFixedT(lang)
  const face = (card: CardData, back: boolean) => back
    ? <CardBack card={card} school={deck.school} settings={deck.settings} lang={lang} />
    : <CardFront card={card} school={deck.school} settings={deck.settings} lang={lang} photoUrl={photos[card.student.id]} signatureUrl={signature} />
  if (layout === 'cr80') {
    return <div className="idcard-print-root" data-layout="cr80">
      <style>{`@page { size: ${CARD_WIDTH_MM}mm ${CARD_HEIGHT_MM}mm; margin: 0; }`}</style>
      {deck.cards.flatMap(card => [false, true].map(back => <div key={`${card.student.id}-${back}`} className="idcard-page" style={{ width: `${CARD_WIDTH_MM}mm`, height: `${CARD_HEIGHT_MM}mm`, overflow: 'hidden' }}>{face(card, back)}</div>))}
    </div>
  }
  const digits = new Intl.NumberFormat(lang === 'bn' ? 'bn-BD' : 'en-US')
  const sheets: CardData[][] = []
  for (let i = 0; i < deck.cards.length; i += perSheet) sheets.push(deck.cards.slice(i, i + perSheet))
  return <div className="idcard-print-root" data-layout="a4">
    <style>{'@page { size: A4 portrait; margin: 0; }'}</style>
    {sheets.flatMap((sheet, number) => [false, true].map(back => <div key={`${number}-${back}`} className="idcard-page" style={{ position: 'relative', width: `${A4.width}mm`, height: `${A4.height}mm`, overflow: 'hidden', background: '#ffffff' }}>
      {/* Left of the centred registration mark, so the two never overlap. */}
      <p style={{ position: 'absolute', top: '3mm', left: `${sheetLeft}mm`, width: `${A4.width / 2 - sheetLeft - 6}mm`, margin: 0, fontFamily: 'Inter, "Noto Sans Bengali", sans-serif', fontSize: '6.5pt', lineHeight: 1.25, color: '#64748b' }}>
        {t(back ? 'idCardSheetBacks' : 'idCardSheetFronts', { sheet: digits.format(number + 1), total: digits.format(sheets.length) })}
      </p>
      <Registration top={sheetTop / 2} />
      <Registration top={A4.height - sheetTop / 2} />
      {sheet.map((card, index) => {
        const { left, top } = slotPosition(index, back)
        return <div key={card.student.id} data-card={card.student.id} data-face={back ? 'back' : 'front'} style={{ position: 'absolute', left: `${left}mm`, top: `${top}mm` }}>
          {cropMarks && <CropMarks />}
          {face(card, back)}
        </div>
      })}
    </div>))}
  </div>
}

// Hairline marks just outside each card's corners, in the gutter.
function CropMarks() {
  const mark = (style: CSSProperties) => <span aria-hidden="true" style={{ position: 'absolute', background: '#0f172a', ...style }} />
  const len = '2.6mm', off = '-3.2mm', w = '0.15mm'
  return <>
    {mark({ left: off, top: 0, width: len, height: w })}{mark({ left: 0, top: off, width: w, height: len })}
    {mark({ right: off, top: 0, width: len, height: w })}{mark({ right: 0, top: off, width: w, height: len })}
    {mark({ left: off, bottom: 0, width: len, height: w })}{mark({ left: 0, bottom: off, width: w, height: len })}
    {mark({ right: off, bottom: 0, width: len, height: w })}{mark({ right: 0, bottom: off, width: w, height: len })}
  </>
}

// Centred crosshair: it falls in the same place on both sides of the sheet.
function Registration({ top }: { top: number }) {
  return <span aria-hidden="true" style={{ position: 'absolute', left: `${A4.width / 2 - 2.5}mm`, top: `${top - 2.5}mm`, width: '5mm', height: '5mm', borderRadius: '50%', border: '0.15mm solid #0f172a', boxSizing: 'border-box' }}>
    <span style={{ position: 'absolute', left: '50%', top: '-1mm', bottom: '-1mm', width: '0.15mm', background: '#0f172a' }} />
    <span style={{ position: 'absolute', top: '50%', left: '-1mm', right: '-1mm', height: '0.15mm', background: '#0f172a' }} />
  </span>
}

const blankSettings: CardSettingsInput = { valid_until: '', signatory_title_en: '', signatory_title_bn: '', emergency_phone: '', usage_note_en: '', usage_note_bn: '', return_note_en: '', return_note_bn: '', show_qr: true }

function CardSettingsPanel({ authorized, onSaved }: { authorized: Authorized; onSaved: () => void }) {
  const { t } = useTranslation()
  const settings = useQuery({ queryKey: ['cards', 'settings'], queryFn: () => authorized(token => getCardSettings(token)), retry: false })
  const [form, setForm] = useState<CardSettingsInput>(blankSettings)
  const [file, setFile] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  useEffect(() => { if (settings.data) { const { has_signature: _unused, ...rest } = settings.data; setForm(rest) } }, [settings.data])
  const set = (key: keyof CardSettingsInput) => (value: string | boolean) => setForm(previous => ({ ...previous, [key]: value }))
  async function run(action: () => Promise<unknown>) {
    setBusy(true); setMessage(null)
    try { await action(); await settings.refetch(); onSaved(); setMessage({ tone: 'success', text: t('peopleSaved') }) }
    catch { setMessage({ tone: 'error', text: t('idCardSettingsError') }) } finally { setBusy(false) }
  }
  function save(event: FormEvent<HTMLFormElement>) { event.preventDefault(); void run(() => authorized(token => saveCardSettings(token, form))) }
  const text = useMemo(() => [['signatory_title_en', 80], ['signatory_title_bn', 160]] as const, [])
  if (settings.isPending) return <SectionCard title={t('idCardSettings')}><Skeleton rows={3} /></SectionCard>
  if (settings.isError) return <SectionCard title={t('idCardSettings')}><StateScreen kind="error" message={t('peopleLoadError')} /></SectionCard>
  return <SectionCard title={t('idCardSettings')} description={t('idCardSettingsHelp')} id="card-settings">
    {message && <div className="mb-4"><Notice tone={message.tone}>{message.text}</Notice></div>}
    <form className="grid gap-4 sm:grid-cols-2" onSubmit={save}>
      <label className={ui.label}>{t('idCardValidUntilSetting')}<input type="date" className={ui.field} value={form.valid_until} onChange={e => set('valid_until')(e.target.value)} /><span className="mt-1 block text-xs font-normal normal-case text-slate-500">{t('idCardValidUntilHelp')}</span></label>
      <label className={ui.label}>{t('idCardEmergency')}<input className={ui.field} inputMode="tel" maxLength={32} pattern="[+0-9() \-]*" value={form.emergency_phone} onChange={e => set('emergency_phone')(e.target.value)} /></label>
      {text.map(([key, max]) => <label key={key} className={ui.label}>{t(key === 'signatory_title_en' ? 'idCardSignatoryEN' : 'idCardSignatoryBN')}<input className={ui.field} maxLength={max} value={form[key]} placeholder={t(key === 'signatory_title_en' ? 'idCardDefaultSignatory' : 'idCardDefaultSignatoryBn')} onChange={e => set(key)(e.target.value)} /></label>)}
      {(['usage_note_en', 'usage_note_bn', 'return_note_en', 'return_note_bn'] as const).map(key => <label key={key} className={ui.label}>{t(`idCardSetting_${key}`)}<textarea rows={3} className={ui.field} maxLength={key.endsWith('_en') ? 400 : 800} value={form[key]} placeholder={t(key.startsWith('usage') ? 'idCardUsageDefault' : 'idCardReturnDefault', { lng: key.endsWith('_bn') ? 'bn' : 'en' })} onChange={e => set(key)(e.target.value)} /></label>)}
      <label className="flex items-center gap-2 text-sm font-semibold text-slate-700 sm:col-span-2"><input type="checkbox" className="h-4 w-4 accent-[var(--brand-primary)]" checked={form.show_qr} onChange={e => set('show_qr')(e.target.checked)} />{t('idCardShowQr')}</label>
      <div className="sm:col-span-2"><button disabled={busy} className={ui.primary}>{t('saveChanges')}</button></div>
    </form>
    <div className="mt-6 flex flex-col gap-3 rounded-xl border border-slate-200 bg-slate-50 p-4 sm:flex-row sm:items-end">
      <label className={`${ui.label} flex-1`}>{t('idCardSignature')}<input type="file" accept="image/png,image/jpeg" className={ui.field} onChange={e => setFile(e.target.files?.[0] ?? null)} /><span className="mt-1 block text-xs font-normal normal-case text-slate-500">{t('idCardSignatureHelp')}</span></label>
      <button type="button" disabled={busy || !file} className={ui.primary} onClick={() => file && void run(() => authorized(token => saveCardSignature(token, file)))}>{t('idCardSignatureUpload')}</button>
      {settings.data.has_signature && <button type="button" disabled={busy} className={ui.danger} onClick={() => void run(() => authorized(token => deleteCardSignature(token)))}>{t('idCardSignatureRemove')}</button>}
    </div>
    <p className="mt-2 text-sm text-slate-600">{t(settings.data.has_signature ? 'idCardSignatureOn' : 'idCardSignatureOff')}</p>
  </SectionCard>
}
