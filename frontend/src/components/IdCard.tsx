import type { CSSProperties, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { CardData, CardSchool, CardSettings } from '../api/client'

// Portrait CR80 card faces (54 × 85.6 mm). Everything is styled inline with
// the school's own colours so the printed card looks the same in light and
// dark dashboard themes (theme rules target Tailwind classes only).
export const CARD_WIDTH_MM = 54
export const CARD_HEIGHT_MM = 85.6
export type CardLanguage = 'en' | 'bn'

const hex = /^#[0-9a-fA-F]{6}$/
const ink = '#0f172a'
const muted = '#475569'
const hairline = '#e2e8f0'

export type CardFaceProps = {
  card: CardData
  school: CardSchool
  settings: CardSettings
  lang: CardLanguage
  photoUrl?: string
  signatureUrl?: string
}

function palette(school: CardSchool) {
  return { primary: hex.test(school.primary_color) ? school.primary_color : '#0f766e', secondary: hex.test(school.secondary_color) ? school.secondary_color : '#334155' }
}
const font = (lang: CardLanguage) => lang === 'bn' ? '"Noto Sans Bengali", Inter, sans-serif' : 'Inter, "Noto Sans Bengali", sans-serif'
const pick = (lang: CardLanguage, en: string, bn: string) => lang === 'bn' && bn ? bn : en
// Long text steps down a size and is capped at two lines rather than overflowing.
const fitted = (text: string, base: number, long: number, veryLong: number): CSSProperties => ({
  fontSize: `${[...text].length > veryLong ? base - 2 : [...text].length > long ? base - 1 : base}pt`,
  display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden', overflowWrap: 'anywhere',
})
function formatDate(value: string, lang: CardLanguage) {
  if (!value) return ''
  const [y, m, d] = value.split('-').map(Number)
  return new Intl.DateTimeFormat(lang === 'bn' ? 'bn-BD' : 'en-GB', { day: '2-digit', month: 'short', year: 'numeric', timeZone: 'UTC' }).format(new Date(Date.UTC(y, m - 1, d)))
}

function Face({ lang, children, label }: { lang: CardLanguage; children: ReactNode; label: string }) {
  return <section aria-label={label} lang={lang} className="idcard-face" style={{ width: `${CARD_WIDTH_MM}mm`, height: `${CARD_HEIGHT_MM}mm`, position: 'relative', overflow: 'hidden', boxSizing: 'border-box', background: '#ffffff', color: ink, fontFamily: font(lang), lineHeight: 1.25, borderRadius: '3mm', display: 'flex', flexDirection: 'column' }}>{children}</section>
}

function Logo({ school, lang, size }: { school: CardSchool; lang: CardLanguage; size: number }) {
  const initial = [...pick(lang, school.name_en, school.name_bn)][0] ?? ''
  return <span style={{ width: `${size}mm`, height: `${size}mm`, borderRadius: '1.6mm', background: '#ffffff', display: 'grid', placeItems: 'center', flexShrink: 0, overflow: 'hidden' }}>
    {school.logo_url
      ? <img src={school.logo_url} alt="" referrerPolicy="no-referrer" style={{ width: '86%', height: '86%', objectFit: 'contain' }} />
      : <span aria-hidden="true" style={{ fontWeight: 800, fontSize: '11pt', color: palette(school).primary }}>{initial}</span>}
  </span>
}

function Detail({ label, value }: { label: string; value: string }) {
  return <div style={{ display: 'grid', gridTemplateColumns: '15mm 1fr', columnGap: '1.5mm', alignItems: 'baseline', padding: '0.55mm 0', borderBottom: `0.2mm solid ${hairline}` }}>
    <span style={{ fontSize: '5.6pt', color: muted, textTransform: 'uppercase', letterSpacing: '0.02em' }}>{label}</span>
    {/* Names wrap to a second line rather than being cut off on a printed card. */}
    <span style={{ fontSize: '6.6pt', fontWeight: 600, display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden', overflowWrap: 'anywhere' }}>{value || '—'}</span>
  </div>
}

export function CardFront({ card, school, settings, lang, photoUrl, signatureUrl }: CardFaceProps) {
  const { i18n } = useTranslation()
  const t = i18n.getFixedT(lang)
  const { primary, secondary } = palette(school)
  const s = card.student
  const schoolName = pick(lang, school.name_en, school.name_bn)
  const name = pick(lang, s.name_en, s.name_bn)
  const roll = s.roll_no == null ? '' : new Intl.NumberFormat(lang === 'bn' ? 'bn-BD' : 'en-US', { useGrouping: false }).format(s.roll_no)
  return <Face lang={lang} label={t('idCardFrontLabel')}>
    <header style={{ background: primary, color: '#ffffff', padding: '2.6mm 3mm 6.5mm', display: 'flex', alignItems: 'center', gap: '2mm', flexShrink: 0 }}>
      <Logo school={school} lang={lang} size={9.5} />
      <div style={{ minWidth: 0 }}>
        <p style={{ margin: 0, fontWeight: 800, ...fitted(schoolName, 8, 30, 46) }}>{schoolName}</p>
        <p style={{ margin: '0.6mm 0 0', fontSize: '5.4pt', letterSpacing: lang === 'bn' ? 0 : '0.12em', textTransform: 'uppercase', opacity: 0.88 }}>{t('idCardTitle')}</p>
      </div>
    </header>
    {/* Only the photo gives up height when long names need a second line. */}
    <div style={{ display: 'flex', justifyContent: 'center', marginTop: '-4.5mm', height: '26.9mm', minHeight: '19.4mm', flexShrink: 1 }}>
      <div style={{ width: '21mm', height: '100%', boxSizing: 'border-box', borderRadius: '2mm', border: '0.7mm solid #ffffff', boxShadow: `0 0 0 0.35mm ${primary}`, overflow: 'hidden', background: '#f1f5f9', display: 'grid', placeItems: 'center' }}>
        {photoUrl
          ? <img src={photoUrl} alt={t('idCardPhotoAlt', { name })} style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
          : <span role="img" aria-label={t('idCardNoPhoto')} style={{ fontSize: '17pt', fontWeight: 700, color: primary, opacity: 0.55 }}>{[...name][0] ?? ''}</span>}
      </div>
    </div>
    <div style={{ padding: '1.6mm 3.2mm 0', textAlign: 'center', flexShrink: 0 }}>
      <h3 style={{ margin: 0, fontWeight: 800, color: ink, ...fitted(name, 9.5, 22, 34) }}>{name}</h3>
      <p style={{ margin: '0.8mm 0 0', fontSize: '6.2pt', fontWeight: 700, color: secondary, letterSpacing: '0.03em' }}>{t('idCardStudentId')}: {s.student_code || s.admission_no}</p>
    </div>
    <div style={{ padding: '1.4mm 3.2mm 0', flex: '1 0 auto' }}>
      <Detail label={t('idCardFather')} value={pick(lang, s.father_name_en, s.father_name_bn)} />
      <Detail label={t('idCardMother')} value={pick(lang, s.mother_name_en, s.mother_name_bn)} />
      <Detail label={t('idCardClass')} value={`${s.class_name} · ${s.section_name}`} />
      <Detail label={t('idCardRoll')} value={roll} />
    </div>
    <footer style={{ padding: '0 3.2mm 2.2mm', display: 'flex', alignItems: 'flex-end', justifyContent: 'space-between', gap: '2mm', flexShrink: 0 }}>
      <div>
        <p style={{ margin: 0, fontSize: '5.2pt', color: muted, textTransform: 'uppercase' }}>{t('idCardValidUntil')}</p>
        <p style={{ margin: 0, fontSize: '6.6pt', fontWeight: 700 }}>{formatDate(card.valid_until, lang)}</p>
      </div>
      <div style={{ textAlign: 'center', minWidth: '19mm' }}>
        <div style={{ height: '6mm', display: 'flex', alignItems: 'flex-end', justifyContent: 'center' }}>{signatureUrl && <img src={signatureUrl} alt={t('idCardSignatureAlt')} style={{ maxHeight: '6mm', maxWidth: '19mm', objectFit: 'contain' }} />}</div>
        <div style={{ borderTop: `0.25mm solid ${ink}`, marginTop: '0.4mm', paddingTop: '0.4mm', fontSize: '5.2pt', color: muted }}>{pick(lang, settings.signatory_title_en || t('idCardDefaultSignatory'), settings.signatory_title_bn)}</div>
      </div>
    </footer>
    <div aria-hidden="true" style={{ height: '2mm', background: secondary, flexShrink: 0 }} />
  </Face>
}

export function CardBack({ card, school, settings, lang }: CardFaceProps) {
  const { i18n } = useTranslation()
  const t = i18n.getFixedT(lang)
  const { primary, secondary } = palette(school)
  const schoolName = pick(lang, school.name_en, school.name_bn)
  const usage = pick(lang, settings.usage_note_en, settings.usage_note_bn) || t('idCardUsageDefault')
  // The default note points to the address only when the school has one.
  const back = pick(lang, settings.return_note_en, settings.return_note_bn) || t(school.address ? 'idCardReturnDefault' : 'idCardReturnDefaultNoAddress')
  const contact = [school.phone, school.email].filter(Boolean)
  const heading: CSSProperties = { margin: '0 0 0.5mm', fontSize: '5.8pt', fontWeight: 800, color: primary, textTransform: lang === 'bn' ? 'none' : 'uppercase', letterSpacing: lang === 'bn' ? 0 : '0.06em' }
  const body: CSSProperties = { margin: 0, fontSize: '6pt', color: '#1e293b' }
  return <Face lang={lang} label={t('idCardBackLabel')}>
    <div aria-hidden="true" style={{ height: '2mm', background: primary }} />
    <div style={{ padding: '2.6mm 3.2mm 0', textAlign: 'center' }}>
      <div style={{ display: 'flex', justifyContent: 'center', marginBottom: '1.2mm' }}><span style={{ display: 'inline-flex', borderRadius: '1.6mm', boxShadow: `0 0 0 0.3mm ${hairline}` }}><Logo school={school} lang={lang} size={8} /></span></div>
      <p style={{ margin: 0, fontWeight: 800, ...fitted(schoolName, 7.6, 30, 46) }}>{schoolName}</p>
      {school.address && <p style={{ margin: '0.6mm 0 0', fontSize: '5.6pt', color: muted, display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>{school.address}</p>}
      {contact.length > 0 && <p style={{ margin: '0.4mm 0 0', fontSize: '5.6pt', color: muted, overflowWrap: 'anywhere' }}>{contact.join(' · ')}</p>}
    </div>
    <div style={{ margin: '2mm 3.2mm 0', borderTop: `0.2mm solid ${hairline}`, paddingTop: '1.6mm', display: 'grid', gap: '1.4mm', flex: 1, alignContent: 'start' }}>
      <div><p style={heading}>{t('idCardUsageHeading')}</p><p style={body}>{usage}</p></div>
      <div><p style={heading}>{t('idCardReturnHeading')}</p><p style={body}>{back}</p></div>
      {settings.emergency_phone && <div><p style={heading}>{t('idCardEmergency')}</p><p style={{ ...body, fontWeight: 700 }}>{settings.emergency_phone}</p></div>}
    </div>
    {settings.show_qr && card.qr_data_url && <div style={{ display: 'flex', alignItems: 'center', gap: '2mm', padding: '0 3.2mm 1.8mm' }}>
      <img src={card.qr_data_url} alt={t('idCardQrAlt')} style={{ width: '17mm', height: '17mm', imageRendering: 'pixelated' }} />
      <p style={{ margin: 0, fontSize: '5.4pt', color: muted }}>{t('idCardQrCaption')}</p>
    </div>}
    <div style={{ background: secondary, color: '#ffffff', fontSize: '5pt', textAlign: 'center', padding: '1mm 2mm' }}>{t('idCardProperty', { school: schoolName })}</div>
  </Face>
}
