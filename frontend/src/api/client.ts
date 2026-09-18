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
  maxUses: number
  usedCount: number
  status: 'available' | 'used' | 'expired'
  expiresAt: string
  createdAt: string
}

/** 功能入口的公开字段（不含内网地址） */
export type Feature = {
  key: string
  title: string
  desc: string
  tag: string
  icon: string
}

/** 管理端视图：额外带内网地址与排序 */
export type AdminFeature = Feature & {
  url: string
  sort: number
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

/** 用户组：后台按组给功能入口授权 */
export type GroupView = {
  id: number
  name: string
  /** 该组可查看的功能 key 集合 */
  features: string[]
  /** 成员用户 ID */
  members: number[]
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

  /** 功能入口点击：后端按用户组鉴权后才下发内网地址（游客一律 403） */
  access: {
    openFeature: (key: string) =>
      request<{ url: string }>(`/api/features/${encodeURIComponent(key)}/open`, { method: 'POST' }),
  },

  /** 主页功能入口的公开列表（仅展示字段，不含地址） */
  features: {
    list: () => request<{ features: Feature[] }>('/api/features'),
  },

  admin: {
    listInvites: () => request<{ invites: Invite[] }>('/api/admin/invites'),
    createInvite: (maxUses: number, expiresInDays: number) =>
      request<Invite>('/api/admin/invites', { method: 'POST', body: JSON.stringify({ maxUses, expiresInDays }) }),
    revokeInvite: (id: number) => request<{ ok: true }>(`/api/admin/invites/${id}`, { method: 'DELETE' }),

    listBans: () => request<{ bans: Ban[] }>('/api/admin/bans'),
    createBan: (ip: string, permanent: boolean) =>
      request<Ban>('/api/admin/bans', { method: 'POST', body: JSON.stringify({ ip, permanent }) }),
    unban: (ip: string) => request<{ ok: true }>(`/api/admin/bans/${encodeURIComponent(ip)}/unban`, { method: 'POST' }),
    resetBan: (ip: string) => request<{ ok: true }>(`/api/admin/bans/${encodeURIComponent(ip)}`, { method: 'DELETE' }),

    listUsers: () => request<{ users: User[] }>('/api/admin/users'),
    setRole: (id: number, role: 'admin' | 'user') =>
      request<{ ok: true }>(`/api/admin/users/${id}/role`, { method: 'PATCH', body: JSON.stringify({ role }) }),

    listGroups: () => request<{ groups: GroupView[] }>('/api/admin/groups'),
    createGroup: (name: string) =>
      request<{ group: GroupView }>('/api/admin/groups', { method: 'POST', body: JSON.stringify({ name }) }),
    updateGroup: (id: number, patch: { name?: string; features?: string[] }) =>
      request<{ ok: true }>(`/api/admin/groups/${id}`, { method: 'PATCH', body: JSON.stringify(patch) }),
    deleteGroup: (id: number) => request<{ ok: true }>(`/api/admin/groups/${id}`, { method: 'DELETE' }),
    setGroupMembers: (id: number, userIds: number[]) =>
      request<{ ok: true }>(`/api/admin/groups/${id}/members`, {
        method: 'PUT',
        body: JSON.stringify({ userIds }),
      }),

    listFeatures: () => request<{ features: AdminFeature[] }>('/api/admin/features'),
    createFeature: (input: { title: string; desc: string; tag: string; icon: string; url: string }) =>
      request<{ feature: AdminFeature }>('/api/admin/features', { method: 'POST', body: JSON.stringify(input) }),
    updateFeature: (key: string, input: { title: string; desc: string; tag: string; icon: string; url: string }) =>
      request<{ ok: true }>(`/api/admin/features/${encodeURIComponent(key)}`, {
        method: 'PATCH',
        body: JSON.stringify(input),
      }),
    deleteFeature: (key: string) =>
      request<{ ok: true }>(`/api/admin/features/${encodeURIComponent(key)}`, { method: 'DELETE' }),
    reorderFeatures: (keys: string[]) =>
      request<{ ok: true }>('/api/admin/features/order', { method: 'PUT', body: JSON.stringify({ keys }) }),
  },
}
