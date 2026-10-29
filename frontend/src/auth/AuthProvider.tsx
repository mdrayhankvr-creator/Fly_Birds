import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { demoLogin, hasRefreshSession, login, logout, refreshSession, type AuthSession, type AuthUser, type DemoRole } from '../api/client'
import { clearMarksDrafts } from '../pages/marks/useSheetAutosave'

type AuthContextValue = {
  user: AuthUser | null
  accessToken: string | null
  ready: boolean
  expired: boolean
  signIn: (email: string, password: string) => Promise<void>
  signInAsDemo: (role: DemoRole) => Promise<void>
  signOut: () => Promise<void>
  renew: () => Promise<string>
  expire: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<AuthSession | null>(null)
  const [ready, setReady] = useState(false)
  const [expired, setExpired] = useState(false)

  const renew = useCallback(async () => {
    try {
      const next = await refreshSession()
      setSession(next)
      setExpired(false)
      return next.access_token
    } catch (error) {
      setSession(null)
      setExpired(true)
      throw error
    }
  }, [])

  useEffect(() => {
    let active = true
    if (!hasRefreshSession()) {
      setReady(true)
      return () => { active = false }
    }
    void renew().catch(() => {}).finally(() => {
      if (active) setReady(true)
    })
    return () => { active = false }
  }, [renew])

  useEffect(() => {
    if (!session) return
    const timeout = window.setTimeout(() => {
      void renew().catch(() => {})
    }, 14 * 60 * 1000)
    return () => window.clearTimeout(timeout)
  }, [session, renew])

  async function signIn(email: string, password: string) {
    const next = await login(email, password)
    setSession(next)
    setExpired(false)
  }

  async function signInAsDemo(role: DemoRole) {
    const next = await demoLogin(role)
    setSession(next)
    setExpired(false)
  }

  async function signOut() {
    clearMarksDrafts() // unsaved marks kept for recovery never outlive the session
    await logout()
    setSession(null)
    setExpired(false)
  }

  const expire = useCallback(() => {
    setSession(null)
    setExpired(true)
  }, [])

  return (
    <AuthContext.Provider value={{ user: session?.user ?? null, accessToken: session?.access_token ?? null, ready, expired, signIn, signInAsDemo, signOut, renew, expire }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) throw new Error('AuthProvider is required')
  return context
}
