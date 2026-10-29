import type { CSSProperties } from 'react'

// Each area of the app has one recognisable hue (see --mod-* in styles.css).
export type Module = 'home' | 'routine' | 'homework' | 'diary' | 'notices' | 'events' | 'alerts' | 'exams' | 'attendance' | 'fees' | 'people' | 'analytics' | 'admin'

// tone passes a module (or any CSS colour) to the ds-* classes through --s.
export function tone(color: Module | string): CSSProperties {
  return { '--s': color.startsWith('#') || color.startsWith('var(') ? color : `var(--mod-${color})` } as CSSProperties
}

// Module for each navigation key, so links, tabs and pages share their module's colour.
const NAV_MODULE: Record<string, Module> = {
  dashboard: 'home', analytics: 'analytics', analytics_teacher: 'analytics', progress: 'analytics', reports: 'analytics',
  students: 'people', guardians: 'people', staff: 'people', me: 'people', children: 'people', staff_me: 'people',
  classes: 'exams', sections: 'exams', subjects: 'exams', sessions: 'routine',
  routines: 'routine', routine_me: 'routine',
  exams: 'exams', exams_me: 'exams', results: 'exams',
  diary: 'diary', diary_me: 'diary', diary_admin: 'diary',
  student_attendance: 'attendance', teacher_attendance: 'attendance', attendance_me: 'attendance', attendance_self: 'attendance',
  fees: 'fees', fees_me: 'fees',
  notices: 'notices', events: 'events', notifications: 'notices', delivery: 'notices',
}
export const navModule = (key: string): Module => NAV_MODULE[key] ?? 'admin'
