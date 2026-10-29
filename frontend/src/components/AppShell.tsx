import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link, NavLink, useLocation } from 'react-router'
import { useTranslation } from 'react-i18next'
import type { SchoolBranding } from '../api/client'
import { Icon } from './icons'
import { activeItem, type NavGroup, type NavItem } from './navigation'
import { navModule, tone } from './dashboard/kit'
import { NavGroupsContext } from './navContext'
import { currentTheme, setThemePreference, type Theme } from './theme'

type ShellUser = { email: string; role: string; display_name?: string }

function initials(name: string) {
  return name.split(/[\s@.]+/).filter(Boolean).slice(0, 2).map(part => [...part][0]).join('').toUpperCase()
}

export function SchoolMark({ branding, size = 'h-9 w-9' }: { branding: SchoolBranding | undefined; size?: string }) {
  const { i18n } = useTranslation()
  const name = branding ? (i18n.language === 'bn' ? branding.name_bn : branding.name_en) : ''
  if (branding?.logo_url) return <img src={branding.logo_url} alt="" referrerPolicy="no-referrer" className={`${size} shrink-0 rounded-lg bg-white object-contain ring-1 ring-slate-200`} />
  return <span aria-hidden="true" className={`${size} shrink-0 rounded-lg shell-mark flex items-center justify-center text-sm font-bold`}>{initials(name || 'FB') || <Icon name="school" />}</span>
}

export function LanguageSwitch({ onChange }: { onChange: (language: 'en' | 'bn') => void }) {
  const { t, i18n } = useTranslation()
  return <div role="group" aria-label={t('language')} className="flex items-center rounded-lg border border-slate-200 bg-white p-0.5 text-xs font-semibold">
    {(['en', 'bn'] as const).map(lang => <button key={lang} type="button" onClick={() => onChange(lang)} aria-pressed={i18n.language === lang}
      className={`rounded-md px-2.5 py-1.5 transition-colors focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] ${i18n.language === lang ? 'shell-lang-on' : 'text-slate-600 hover:text-slate-900'}`}>{lang === 'en' ? 'EN' : 'বাংলা'}</button>)}
  </div>
}

export function ThemeSwitch() {
  const { t } = useTranslation()
  const [theme, setTheme] = useState<Theme>(currentTheme)
  // The school default can arrive after first render; follow it until the user chooses.
  useEffect(() => {
    const observer = new MutationObserver(() => setTheme(currentTheme()))
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
    return () => observer.disconnect()
  }, [])
  const next: Theme = theme === 'dark' ? 'light' : 'dark'
  return <button type="button" onClick={() => setThemePreference(next)} aria-label={t(next === 'dark' ? 'p1ThemeToDark' : 'p1ThemeToLight')} title={t(next === 'dark' ? 'p1ThemeToDark' : 'p1ThemeToLight')}
    className="grid h-10 w-10 shrink-0 place-items-center rounded-xl text-slate-600 hover:bg-slate-100 hover:text-slate-900 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]">
    <Icon name={theme === 'dark' ? 'sun' : 'moon'} className="h-5 w-5" />
  </button>
}

function UserMenu({ user, onSignOut }: { user: ShellUser; onSignOut: () => void }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent ? e.key === 'Escape' : !ref.current?.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', close); document.addEventListener('keydown', close)
    return () => { document.removeEventListener('mousedown', close); document.removeEventListener('keydown', close) }
  }, [open])
  const name = user.display_name || user.email
  return <div className="relative" ref={ref}>
    <button type="button" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(v => !v)}
      className="flex items-center gap-2 rounded-xl p-1 pr-2 text-left hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]">
      <span aria-hidden="true" className="shell-avatar flex h-8 w-8 items-center justify-center rounded-full text-xs font-bold">{initials(name)}</span>
      <span className="hidden min-w-0 md:block"><span className="block max-w-[11rem] truncate text-sm font-semibold text-slate-900">{name}</span><span className="block text-xs text-slate-500">{t(`role_${user.role}`)}</span></span>
      <Icon name="chevronDown" className="hidden h-4 w-4 text-slate-500 md:block" />
    </button>
    {open && <div role="menu" className="absolute right-0 z-40 mt-2 w-64 overflow-hidden rounded-xl border border-slate-200 bg-white py-1 shadow-lg">
      <div className="border-b border-slate-100 px-4 py-3"><p className="truncate text-sm font-semibold text-slate-900">{name}</p><p className="truncate text-xs text-slate-500">{user.email}</p></div>
      <Link role="menuitem" to="/account" onClick={() => setOpen(false)} className="flex items-center gap-2 px-4 py-2.5 text-sm text-slate-700 hover:bg-slate-50"><Icon name="user" className="h-4 w-4" />{t('account')}</Link>
      <button role="menuitem" type="button" onClick={() => { setOpen(false); onSignOut() }} className="flex w-full items-center gap-2 px-4 py-2.5 text-left text-sm text-slate-700 hover:bg-slate-50"><Icon name="logout" className="h-4 w-4" />{t('signOut')}</button>
    </div>}
  </div>
}

function SidebarNav({ groups, collapsed, onNavigate }: { groups: NavGroup[]; collapsed: boolean; onNavigate?: () => void }) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const current = activeItem(groups, pathname)
  const ref = useRef<HTMLElement>(null)
  useEffect(() => { ref.current?.querySelector('[aria-current="page"]')?.scrollIntoView({ block: 'nearest' }) }, [pathname])
  return <nav ref={ref} aria-label={t('m12MainNavigation')} className="flex-1 space-y-5 overflow-y-auto px-3 py-4">
    {groups.map(group => <div key={group.key}>
      {!collapsed && <p className="px-3 pb-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">{t(group.label)}</p>}
      {collapsed && <div className="mx-3 mb-2 border-t border-slate-200" aria-hidden="true" />}
      <ul className="space-y-0.5">{group.items.map(item => {
        const active = current?.path === item.path
        return <li key={item.path}>
          <NavLink to={item.path} onClick={onNavigate} title={collapsed ? t(item.label) : undefined} aria-current={active ? 'page' : undefined}
            className={`group flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--brand-primary)] ${collapsed ? 'justify-center' : ''} ${active ? 'shell-nav-active' : 'text-slate-700 hover:bg-slate-100 hover:text-slate-900'}`}>
            <Icon name={item.icon} className={`h-5 w-5 shrink-0 ${active ? '' : 'text-slate-500 group-hover:text-slate-700'}`} />
            <span className={collapsed ? 'sr-only' : 'truncate'}>{t(item.label)}</span>
          </NavLink>
        </li>
      })}</ul>
    </div>)}
  </nav>
}

// Preferred bottom-bar tabs for each role. Only items already in the user's
// navigation (their permissions and enabled modules) can appear; empty slots
// are filled from the rest of the navigation in sidebar order.
const BOTTOM_TABS: Record<string, string[]> = {
  admin: ['dashboard', 'students', 'fees', 'notices'],
  head_teacher: ['dashboard', 'student_attendance', 'routines', 'notices'],
  teacher: ['dashboard', 'routine_me', 'diary', 'student_attendance'],
  accountant: ['dashboard', 'fees', 'reports', 'notices'],
  student: ['dashboard', 'routine_me', 'diary_me', 'results'],
}
const SHORT_LABEL: Record<string, string> = {
  dashboard: 'p1Tab_home', students: 'p1Tab_students', fees: 'p1Tab_fees', fees_me: 'p1Tab_fees', notices: 'p1Tab_notices', student_attendance: 'p1Tab_attendance',
  routines: 'p1Tab_routine', routine_me: 'p1Tab_routine', diary: 'p1Tab_diary', diary_me: 'p1Tab_diary', results: 'p1Tab_results', reports: 'p1Tab_reports',
  children: 'p1Tab_children', progress: 'p1Tab_progress', notifications: 'p1Tab_alerts', analytics: 'p1Tab_analytics', exams_me: 'p1Tab_marks',
}

function BottomNav({ groups, role, menuOpen, onMore }: { groups: NavGroup[]; role: string; menuOpen: boolean; onMore: () => void }) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const items = groups.flatMap(g => g.items)
  const preferred = (BOTTOM_TABS[role] ?? ['dashboard']).map(key => items.find(i => i.key === key)).filter((i): i is NavItem => !!i)
  const tabs = [...preferred, ...items.filter(i => !preferred.includes(i))].slice(0, 4)
  const current = activeItem(groups, pathname)
  return <nav aria-label={t('p1BottomNav')} className="ds-bottom-nav lg:hidden print:hidden">
    <ul className="flex items-stretch px-1">
      {tabs.map(item => {
        const active = current?.path === item.path
        return <li key={item.path} className="min-w-0 flex-1">
          <Link to={item.path} aria-current={active ? 'page' : undefined} style={tone(navModule(item.key))}
            className="ds-tab flex h-full flex-col items-center justify-center gap-1 pb-1.5 pt-2">
            <span className="ds-tab-pill grid h-8 w-14 max-w-full place-items-center rounded-full"><Icon name={item.icon} className="h-[22px] w-[22px]" /></span>
            <span className="ds-tab-label max-w-full truncate px-0.5 text-[11.5px] font-semibold leading-snug">{t(SHORT_LABEL[item.key] ?? item.label)}</span>
          </Link>
        </li>
      })}
      {items.length > tabs.length && <li className="min-w-0 flex-1">
        <button type="button" onClick={onMore} aria-expanded={menuOpen} aria-haspopup="dialog" style={tone('admin')} className="ds-tab flex h-full w-full flex-col items-center justify-center gap-1 pb-1.5 pt-2">
          <span className="ds-tab-pill grid h-8 w-14 max-w-full place-items-center rounded-full"><Icon name="menu" className="h-[22px] w-[22px]" /></span>
          <span className="ds-tab-label max-w-full truncate px-0.5 text-[11.5px] font-semibold leading-snug">{t('p1More')}</span>
        </button>
      </li>}
    </ul>
  </nav>
}

export function AppShell({ branding, user, groups, onLanguage, onSignOut, bell, children }: {
  branding: SchoolBranding | undefined; user: ShellUser; groups: NavGroup[]; onLanguage: (l: 'en' | 'bn') => void; onSignOut: () => void; bell?: ReactNode; children: ReactNode
}) {
  const { t, i18n } = useTranslation()
  const { pathname } = useLocation()
  const [collapsed, setCollapsed] = useState(() => { try { return localStorage.getItem('flybirds.sidebar') === 'collapsed' } catch { return false } })
  const [drawer, setDrawer] = useState(false)
  const drawerRef = useRef<HTMLDivElement>(null)
  const schoolName = branding ? (i18n.language === 'bn' ? branding.name_bn : branding.name_en) : t('appName')
  const current = activeItem(groups, pathname)
  const currentGroup = current ? groups.find(g => g.items.includes(current)) : undefined
  // Pages that are not in the sidebar still get a meaningful title.
  const fallbackCrumb = pathname === '/account' ? 'accountHeading' : pathname.startsWith('/analytics/students/') ? 'm11StudentProgress' : ''
  useEffect(() => { setDrawer(false) }, [pathname])
  useEffect(() => {
    if (!drawer) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setDrawer(false) }
    document.addEventListener('keydown', onKey)
    drawerRef.current?.querySelector<HTMLElement>('a, button')?.focus()
    document.body.style.overflow = 'hidden'
    return () => { document.removeEventListener('keydown', onKey); document.body.style.overflow = '' }
  }, [drawer])
  function toggleCollapsed() {
    setCollapsed(v => { const next = !v; try { localStorage.setItem('flybirds.sidebar', next ? 'collapsed' : 'open') } catch { /* storage unavailable */ } return next })
  }
  const brand = (compact: boolean) => <Link to="/dashboard" className="flex min-w-0 items-center gap-3 rounded-lg focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]">
    <SchoolMark branding={branding} />
    {!compact && <span className="min-w-0"><span className="line-clamp-2 block text-sm font-bold leading-snug text-slate-900">{schoolName}</span>{[...schoolName].length <= 28 && <span className="block text-xs text-slate-500">{t('m12Tagline')}</span>}</span>}
  </Link>
  return <div className="shell-root min-h-screen">
    <a href="#main" className="sr-only focus:not-sr-only focus:fixed focus:left-3 focus:top-3 focus:z-50 focus:rounded-lg focus:bg-white focus:px-4 focus:py-2 focus:shadow">{t('m12SkipToContent')}</a>
    {/* Desktop sidebar */}
    <aside className={`shell-sidebar fixed inset-y-0 left-0 z-30 hidden flex-col border-r border-slate-200 lg:flex print:hidden ${collapsed ? 'w-[76px]' : 'w-64'}`}>
      <div className={`flex h-16 items-center border-b border-slate-200 ${collapsed ? 'justify-center px-2' : 'px-4'}`}>{brand(collapsed)}</div>
      <SidebarNav groups={groups} collapsed={collapsed} />
      <div className="border-t border-slate-200 p-3">
        <button type="button" onClick={toggleCollapsed} aria-expanded={!collapsed} className={`flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-slate-600 hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] ${collapsed ? 'justify-center' : ''}`}>
          <Icon name={collapsed ? 'chevronRight' : 'chevronLeft'} className="h-5 w-5" /><span className={collapsed ? 'sr-only' : ''}>{t(collapsed ? 'm12ExpandSidebar' : 'm12CollapseSidebar')}</span>
        </button>
      </div>
    </aside>
    {/* Mobile drawer */}
    {drawer && <div className="fixed inset-0 z-40 lg:hidden" role="dialog" aria-modal="true" aria-label={t('m12MainNavigation')}>
      <button type="button" className="absolute inset-0 bg-slate-900/40" aria-label={t('m12CloseMenu')} onClick={() => setDrawer(false)} />
      <div ref={drawerRef} className="shell-sidebar absolute inset-y-0 left-0 flex w-[min(20rem,86vw)] flex-col shadow-xl">
        <div className="flex h-16 items-center justify-between border-b border-slate-200 px-4">{brand(false)}
          <button type="button" onClick={() => setDrawer(false)} className="rounded-lg p-2 text-slate-600 hover:bg-slate-100" aria-label={t('m12CloseMenu')}><Icon name="close" /></button></div>
        <SidebarNav groups={groups} collapsed={false} onNavigate={() => setDrawer(false)} />
        <div className="flex items-center justify-between gap-3 border-t border-slate-200 p-4"><span className="text-xs font-semibold text-slate-500">{t('language')}</span><LanguageSwitch onChange={onLanguage} /></div>
      </div>
    </div>}
    <div className={`flex min-h-screen flex-col ${collapsed ? 'lg:pl-[76px]' : 'lg:pl-64'}`}>
      <header className="shell-topbar sticky top-0 z-20 flex h-16 items-center gap-3 border-b border-slate-200 px-4 sm:px-6 print:hidden">
        <button type="button" onClick={() => setDrawer(true)} className="-ml-1 rounded-lg p-2 text-slate-700 hover:bg-slate-100 lg:hidden" aria-label={t('m12OpenMenu')} aria-expanded={drawer}><Icon name="menu" /></button>
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <span className="lg:hidden">{brand(true)}</span>
          <nav aria-label={t('m12Breadcrumb')} className="min-w-0"><ol className="flex min-w-0 items-center gap-1.5 text-sm">
            {currentGroup && currentGroup.key !== 'overview' && <li className="hidden shrink-0 items-center gap-1.5 text-slate-500 sm:flex">{t(currentGroup.label)}<Icon name="chevronRight" className="h-3.5 w-3.5" /></li>}
            <li className="min-w-0 truncate font-semibold text-slate-900 sm:text-base" aria-current="page">{current ? t(current.label) : fallbackCrumb ? t(fallbackCrumb) : schoolName}</li>
          </ol></nav>
        </div>
        <div className="flex items-center gap-1.5 sm:gap-3">
          <span className="hidden sm:block"><LanguageSwitch onChange={onLanguage} /></span>
          <ThemeSwitch />
          {bell}
          <UserMenu user={user} onSignOut={onSignOut} />
        </div>
      </header>
      <main id="main" tabIndex={-1} className="mx-auto w-full max-w-7xl flex-1 px-4 pb-28 pt-6 focus:outline-none sm:px-6 lg:px-8 lg:py-8 print:pb-0"><NavGroupsContext.Provider value={groups}>{children}</NavGroupsContext.Provider></main>
    </div>
    <BottomNav groups={groups} role={user.role} menuOpen={drawer} onMore={() => setDrawer(true)} />
  </div>
}
