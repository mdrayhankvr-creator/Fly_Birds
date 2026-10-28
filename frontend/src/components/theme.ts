// Light or dark theme. The school's dashboard_theme is the default; a user can
// override it for their own browser, and the choice is kept in localStorage.

export type Theme = 'light' | 'dark'

const KEY = 'flybirds.theme'
let schoolTheme: Theme = 'light'

export function themePreference(): Theme | null {
  try {
    const value = localStorage.getItem(KEY)
    return value === 'light' || value === 'dark' ? value : null
  } catch { return null }
}

export function currentTheme(): Theme {
  return document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light'
}

// applyTheme records the school default (when given) and shows the user's own
// choice if they made one.
export function applyTheme(school?: Theme) {
  if (school) schoolTheme = school
  document.documentElement.dataset.theme = themePreference() ?? schoolTheme
}

export function setThemePreference(theme: Theme) {
  try { localStorage.setItem(KEY, theme) } catch { /* storage unavailable: the choice lasts until reload */ }
  document.documentElement.dataset.theme = theme
}
