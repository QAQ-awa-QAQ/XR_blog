export type User = {
  id: number
  account: string
  nickname: string
  /** guest = 游客：只读浏览，没有真实账户 */
  role: 'admin' | 'user' | 'guest'
  ip: string
}

export type AuthResult = {
  user: User
  csrfToken: string
}

export type Invite = {
  id: number
  code: string
  createdBy: number
  usedBy: number | null
  status: 'available' | 'used' | 'expired'
  expiresAt: string
  createdAt: string
}

export type Ban = {
  ip: string
  offenseCount: number
  permanent: boolean
  active: boolean
  expiresAt: string | null
  reason: string
  updatedAt: string
}

/** 与后端 httpx.Fail 的响应体保持一致 */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly retryAfter?: number,
    readonly permanent?: boolean,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export function readCookie(name: string): string {
  const target = `${name}=`
  for (const part of document.cookie.split(';')) {
    const item = part.trim()
    if (item.startsWith(target)) return decodeURIComponent(item.slice(target.length))
  }
  return ''
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body) headers.set('Content-Type', 'application/json')

  // admin 写接口需要 double-submit CSRF token
  const method = (init.method ?? 'GET').toUpperCase()
  if (method !== 'GET' && method !== 'HEAD') {
    const csrf = readCookie('csrf')
    if (csrf) headers.set('X-CSRF-Token', csrf)
  }

  const res = await fetch(path, { credentials: 'include', ...init, headers })
  const text = await res.text()
  const data = text ? (JSON.parse(text) as Record<string, unknown>) : null

  if (!res.ok) {
    throw new ApiError(
      res.status,
      (data?.code as string) ?? 'error',
      (data?.message as string) ?? '请求失败，请稍后再试',
      data?.retry_after as number | undefined,
      data?.permanent as boolean | undefined,
    )
  }
  return data as T
}

export const api = {
  session: () => request<{ user: User }>('/api/auth/session'),
  login: (account: string, password: string) =>
    request<AuthResult>('/api/auth/login', { method: 'POST', body: JSON.stringify({ account, password }) }),
  register: (payload: { account: string; password: string; nickname: string; inviteCode: string }) =>
    request<AuthResult>('/api/auth/register', { method: 'POST', body: JSON.stringify(payload) }),
  /** 访客登录：无凭据，直接建立只读会话 */
  guest: () => request<AuthResult>('/api/auth/guest', { method: 'POST' }),
  logout: () => request<{ ok: true }>('/api/auth/logout', { method: 'POST' }),

  admin: {
    listInvites: () => request<{ invites: Invite[] }>('/api/admin/invites'),
    createInvite: () => request<Invite>('/api/admin/invites', { method: 'POST' }),
    revokeInvite: (id: number) => request<{ ok: true }>(`/api/admin/invites/${id}`, { method: 'DELETE' }),

    listBans: () => request<{ bans: Ban[] }>('/api/admin/bans'),
    createBan: (ip: string, permanent: boolean) =>
      request<Ban>('/api/admin/bans', { method: 'POST', body: JSON.stringify({ ip, permanent }) }),
    unban: (ip: string) => request<{ ok: true }>(`/api/admin/bans/${encodeURIComponent(ip)}/unban`, { method: 'POST' }),
    resetBan: (ip: string) => request<{ ok: true }>(`/api/admin/bans/${encodeURIComponent(ip)}`, { method: 'DELETE' }),

    listUsers: () => request<{ users: User[] }>('/api/admin/users'),
    setRole: (id: number, role: 'admin' | 'user') =>
      request<{ ok: true }>(`/api/admin/users/${id}/role`, { method: 'PATCH', body: JSON.stringify({ role }) }),
  },
}
