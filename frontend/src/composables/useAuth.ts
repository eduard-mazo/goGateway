import { ref, computed } from 'vue'

// localStorage key namespace
const K = {
  access:  'gw:access',
  refresh: 'gw:refresh',
  session: 'gw:session',
} as const

export type Role = 'superadmin' | 'operator' | 'viewer'

export interface AuthUser {
  userId:   number
  username: string
  role:     Role | string
  exp:      number // Unix seconds
}

// Decode a JWT payload without a library (no signature verification — the
// backend validates on every protected request).
function parseToken(jwt: string): AuthUser | null {
  try {
    const b64 = jwt.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')
    const p = JSON.parse(atob(b64))
    return { userId: p.uid, username: p.sub, role: p.role, exp: p.exp }
  } catch {
    return null
  }
}

function loadStored(): AuthUser | null {
  const token = localStorage.getItem(K.access)
  if (!token) return null
  const user = parseToken(token)
  if (!user || user.exp * 1000 < Date.now()) {
    Object.values(K).forEach(k => localStorage.removeItem(k))
    return null
  }
  return user
}

// Module-level reactive state — shared across all composable calls.
const _user = ref<AuthUser | null>(loadStored())

// Singleton refresh promise — prevents concurrent refresh calls.
let _refreshing: Promise<boolean> | null = null

export function useAuth() {
  const user          = computed(() => _user.value)
  const isLoggedIn    = computed(() => _user.value !== null)
  const role          = computed(() => _user.value?.role ?? null)

  /** POST /api/auth/login → stores tokens, updates user. */
  async function login(username: string, password: string): Promise<void> {
    // Import api lazily to avoid circular dep (api.ts sets up interceptors
    // that call refresh(), which lives here).
    const { api } = await import('@/api')
    const { data } = await api.post('/auth/login', { username, password })
    localStorage.setItem(K.access,  data.access_token)
    localStorage.setItem(K.refresh, data.refresh_token)
    localStorage.setItem(K.session, data.session_id)
    _user.value = parseToken(data.access_token)
  }

  /** POST /api/auth/refresh → issues a new access token silently. */
  async function refresh(): Promise<boolean> {
    if (_refreshing) return _refreshing
    _refreshing = (async () => {
      const sessionId    = localStorage.getItem(K.session)
      const refreshToken = localStorage.getItem(K.refresh)
      if (!sessionId || !refreshToken) return false
      try {
        const { api } = await import('@/api')
        const { data } = await api.post('/auth/refresh', {
          session_id:    sessionId,
          refresh_token: refreshToken,
        })
        localStorage.setItem(K.access, data.access_token)
        _user.value = parseToken(data.access_token)
        return true
      } catch {
        return false
      } finally {
        _refreshing = null
      }
    })()
    return _refreshing
  }

  /** POST /api/auth/logout → revokes session, clears local state. */
  async function logout(): Promise<void> {
    try {
      const { api } = await import('@/api')
      await api.post('/auth/logout')
    } catch {
      // best-effort — always clear local state
    }
    Object.values(K).forEach(k => localStorage.removeItem(k))
    _user.value = null
  }

  function getAccessToken(): string | null {
    return localStorage.getItem(K.access)
  }

  return { user, isLoggedIn, role, login, logout, refresh, getAccessToken }
}
