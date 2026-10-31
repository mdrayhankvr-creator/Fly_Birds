import { useId, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getBlueprintVersion, getClassBlueprints, type BlueprintDefinition, type BlueprintVersion } from '../../api/client'
import { confirmAction } from '../../components/ConfirmDialog'
import { CardTitle, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'
import { applyBlueprintPreset, copyFromClass, type BlueprintPreset } from './blueprintModel'
import { useExplain } from './setupShared'

const PRESETS: { preset: BlueprintPreset; classTest: boolean }[] = [
  { preset: 'cq_mcq', classTest: false },
  { preset: 'cq_mcq', classTest: true },
  { preset: 'cq_mcq_prac', classTest: false },
  { preset: 'cq_mcq_prac', classTest: true },
]

// QuickSetup (E7 step 5): one click sets every subject from a preset, or
// copies another class's blueprint of the same session; subjects are then
// edited one by one as before.
export function QuickSetup({ v, def, termId, termName, change, onDone }: { v: BlueprintVersion; def: BlueprintDefinition; termId: string; termName: string; change: (d: BlueprintDefinition) => void; onDone: (text: string, tone: 'success' | 'error') => void }) {
  const { t } = useTranslation()
  const uid = useId()
  const authorized = usePeopleApi()
  const explain = useExplain()
  const [from, setFrom] = useState('')
  const [busy, setBusy] = useState(false)
  const others = useQuery({ queryKey: ['e5', 'blueprints', v.session_id], queryFn: () => authorized(token => getClassBlueprints(token, v.session_id)), retry: false })
  const sources = (others.data ?? []).filter(c => c.class_id !== v.class_id && c.active)

  async function preset(p: BlueprintPreset, classTest: boolean) {
    if (!termId) return
    const label = t(`e7Preset_${p}`) + (classTest ? ` · ${t('e7PresetCT')}` : '')
    if (!await confirmAction(t('e7PresetConfirm', { preset: label, term: termName }))) return
    const next = applyBlueprintPreset(def, termId, p, classTest)
    if (!next) { onDone(t('e7PresetNoExam', { term: termName }), 'error'); return }
    change(next)
    onDone(t('e7PresetApplied', { preset: label }), 'success')
  }

  async function copy() {
    const source = sources.find(c => c.class_id === from)
    if (!source?.active) return
    if (!await confirmAction(t('e7CopyConfirm', { cls: source.class_name }))) return
    setBusy(true)
    try {
      const sv = await authorized(token => getBlueprintVersion(token, source.active!.id))
      const { def: next, copied } = copyFromClass(def, sv.definition)
      change(next)
      onDone(t('e7Copied', { cls: source.class_name, count: copied }), 'success')
    } catch (cause) { onDone(explain(cause), 'error') } finally { setBusy(false) }
  }

  return <section className={`${ui.card} space-y-4 p-4 sm:p-5`} aria-labelledby={`${uid}-title`}>
    <div>
      <CardTitle id={`${uid}-title`} icon="layers">{t('e7QuickTitle')}</CardTitle>
      <p className="mt-1 text-sm text-slate-600">{t('e7QuickIntro', { term: termName })}</p>
    </div>
    {termId && <ul className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4" aria-label={t('e7PresetsLabel')}>{PRESETS.map(p => <li key={`${p.preset}-${p.classTest}`}>
      <button type="button" className="flex h-full w-full flex-col items-start rounded-xl border border-slate-200 bg-white p-3 text-left transition hover:border-slate-300 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]"
        onClick={() => void preset(p.preset, p.classTest)}>
        <span className="font-semibold text-slate-900">{t(`e7Preset_${p.preset}`)}</span>
        <span className="text-xs text-slate-600">{p.classTest ? t('e7PresetCTHint') : t('e7PresetNoCTHint')}</span>
      </button>
    </li>)}</ul>}
    {sources.length > 0 && <form className="flex flex-col gap-2 border-t border-slate-100 pt-4 sm:flex-row sm:items-end" onSubmit={e => { e.preventDefault(); void copy() }}>
      <label className={`${ui.label} min-w-0 flex-1`}>{t('e7CopyFrom')}
        <select className={ui.field} value={from} onChange={e => setFrom(e.target.value)}>
          <option value="">{t('e7CopyChoose')}</option>
          {sources.map(c => <option key={c.class_id} value={c.class_id}>{c.class_name}</option>)}
        </select>
      </label>
      <button type="submit" className={ui.secondary} disabled={!from || busy}>{t('e7CopyAction')}</button>
    </form>}
  </section>
}
