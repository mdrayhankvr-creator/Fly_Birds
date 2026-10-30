// Small, testable rules for the one-page admission screen.

export type Placement = { session_id: string; class_id: string; section_id: string }

const placementKey = 'flybirds.admission.placement'

// The session, class and section used last on this device, so a run of
// admissions into one class keeps its placement.
export function readStoredPlacement(): Placement | null {
  try {
    const value = JSON.parse(localStorage.getItem(placementKey) ?? 'null')
    return value && typeof value.session_id === 'string' && typeof value.class_id === 'string' && typeof value.section_id === 'string' ? value : null
  } catch { return null }
}

export function storePlacement(p: Placement) {
  try { localStorage.setItem(placementKey, JSON.stringify(p)) } catch { /* private mode: no memory, no harm */ }
}

export function todayInDhaka(): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Dhaka' }).format(new Date())
}

// admissionLoginLocalPart turns an admission number into the part of a
// suggested login email before the @: lower-case letters and digits, with
// separators as single hyphens.
export function admissionLoginLocalPart(admissionNo: string): string {
  return admissionNo.toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 64)
}
