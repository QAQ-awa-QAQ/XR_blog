import { useCallback, useEffect, useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import gsap from 'gsap'
import { Orbs } from '../components/Orbs'
import { ErrorBanner, Field, Modal, Spinner } from '../components/ui'
import { FeatureIcon } from '../components/FeatureIcon'
import { FEATURE_ICON_LABELS, FEATURE_ICON_NAMES } from '../design/featureIcons'
import { easings } from '../motion/tokens'
import { DEFAULT_MORE_SETTINGS, DEFAULT_SITE, type SectionConfig, type SiteConfig } from '../content/site'
import {
  ApiError,
  api,
  type AdminFeature,
  type Ban,
  type GroupView,
  type Invite,
  type User,
} from '../api/client'

type Stage = 'checking' | 'ready' | 'denied'

/** 返回箭头的自绘线：与欢迎页 / 登录页同一套「线 + 两笔尖」，整体镜像成 ← */
function BackArrow() {
  return (
    <svg className="admin__back-arrow" viewBox="-1.75 0 24 16" aria-hidden="true">
      <line className="admin__back-line" x1="19" y1="8" x2="1.5" y2="8" />
      <path className="admin__back-line" d="M1.5 8 L7.8 2.8" />
      <path className="admin__back-line" d="M1.5 8 L7.8 13.2" />
    </svg>
  )
}

function formatTime(value: string | null) {
  if (!value) return '—'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

/** 后台分区：左栏导航用（文字条目，沿用主页侧栏的胶囊语言） */
type AdminSectionId = 'invites' | 'bans' | 'users' | 'groups' | 'features' | 'site'

const ADMIN_SECTIONS: { id: AdminSectionId; label: string }[] = [
  { id: 'invites', label: '邀请码' },
  { id: 'bans', label: '封禁名单' },
  { id: 'users', label: '用户' },
  { id: 'groups', label: '用户组' },
  { id: 'features', label: '功能配置' },
  { id: 'site', label: '站点配置' },
]

/**
 * 管理后台（用户批准的 4.3 白名单例外）。
 * 独立路径 /admin，不占用 design.md 2.5 规定的三个侧栏入口。
 */
export function Admin({ themeLabel, onExit }: { themeLabel: string; onExit: () => void }) {
  const [stage, setStage] = useState<Stage>('checking')
  const [me, setMe] = useState<User | null>(null)
  const [invites, setInvites] = useState<Invite[]>([])
  const [bans, setBans] = useState<Ban[]>([])
  const [users, setUsers] = useState<User[]>([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [banIP, setBanIP] = useState('')
  const [banForever, setBanForever] = useState(false)

  const [section, setSection] = useState<AdminSectionId>('invites')
  const [groups, setGroups] = useState<GroupView[]>([])
  const [adminFeatures, setAdminFeatures] = useState<AdminFeature[]>([])
  const [featureDraft, setFeatureDraft] = useState<AdminFeature | 'new' | null>(null)
  const [inviteUses, setInviteUses] = useState(1)
  const [inviteDays, setInviteDays] = useState(7)
  const [newGroupName, setNewGroupName] = useState('')
  const [dragKey, setDragKey] = useState('')
  /** 站点内容配置：后台只编辑草稿，保存时由服务端全量校验并落盘 */
  const [siteConfig, setSiteConfig] = useState<SiteConfig>(DEFAULT_SITE)

  const pageRef = useRef<HTMLDivElement>(null)
  const backRef = useRef<HTMLButtonElement>(null)
  const gridRef = useRef<HTMLDivElement>(null)
  const switchTweenRef = useRef<gsap.core.Timeline | null>(null)

  /** 「←」的过场：箭头先向左发射飞走，页面再淡出，然后把视图切回主页
      （同 bundle 内切换，没有整页导航的闪帧） */
  const backToSite = () => {
    const finish = () => onExit()
    const page = pageRef.current
    const arrow = backRef.current?.querySelector('svg')
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches || !page || !arrow) {
      finish()
      return
    }
    const tl = gsap.timeline({ onComplete: finish })
    tl.to(arrow, { x: -56, autoAlpha: 0, duration: 0.38, ease: 'power2.in' }, 0)
    tl.to(page, { autoAlpha: 0, duration: 0.34, ease: 'power1.in' }, 0.12)
  }

  // 正文向上浮现（checking → ready 时播放）—— 接在「管理后台按钮扩大至全屏」之后的那一段
  useEffect(() => {
    if (stage !== 'ready') return
    const page = pageRef.current
    if (!page) return
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    const ctx = gsap.context(() => {
      // 标题不动：主页那四个字就停在这个位置，切过来直接接替它（标题再动就是「重复出场」）
      gsap.from(page.querySelectorAll('.admin__header p, .admin__header .row, .admin__nav, .admin__grid > *:not([hidden])'), {
        y: 56,
        autoAlpha: 0,
        duration: 0.65,
        ease: easings.soft,
        stagger: 0.07,
      })
    }, page)
    return () => ctx.revert()
  }, [stage])

  const load = useCallback(async () => {
    const [inviteRes, banRes, userRes, groupRes, featureRes, siteRes] = await Promise.all([
      api.admin.listInvites(),
      api.admin.listBans(),
      api.admin.listUsers(),
      api.admin.listGroups(),
      api.admin.listFeatures(),
      api.site(),
    ])
    setInvites(inviteRes.invites)
    setBans(banRes.bans)
    setUsers(userRes.users)
    setGroups(groupRes.groups)
    setAdminFeatures(featureRes.features)
    setSiteConfig(siteRes)
  }, [])

  useEffect(() => {
    let cancelled = false

    const boot = async () => {
      try {
        const { user } = await api.session()
        if (cancelled) return
        if (user.role !== 'admin') {
          setMe(user)
          setStage('denied')
          return
        }
        setMe(user)
        await load()
        if (!cancelled) setStage('ready')
      } catch {
        if (!cancelled) setStage('denied')
      }
    }

    void boot()
    return () => {
      cancelled = true
    }
  }, [load])

  const run = async (action: () => Promise<unknown>) => {
    setBusy(true)
    setError('')
    try {
      await action()
      await load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '操作失败，请稍后再试')
    } finally {
      setBusy(false)
    }
  }

  /** ---------- 用户组 / 功能地址的操作 ---------- */

  const createGroup = () =>
    void run(async () => {
      await api.admin.createGroup(newGroupName.trim())
      setNewGroupName('')
    })

  const renameGroup = (id: number, name: string) => void run(() => api.admin.updateGroup(id, { name }))

  const toggleGroupFeature = (group: GroupView, key: string, on: boolean) =>
    void run(() =>
      api.admin.updateGroup(group.id, {
        features: on ? [...group.features, key] : group.features.filter((k) => k !== key),
      }),
    )

  const toggleGroupMember = (group: GroupView, userId: number, on: boolean) =>
    void run(() =>
      api.admin.setGroupMembers(
        group.id,
        on ? [...group.members, userId] : group.members.filter((id) => id !== userId),
      ),
    )

  const deleteGroup = (group: GroupView) => {
    if (!window.confirm(`删除用户组「${group.name}」？授权与成员关系会一并清除。`)) return
    void run(() => api.admin.deleteGroup(group.id))
  }

  const createInvite = () => void run(() => api.admin.createInvite(inviteUses, inviteDays))

  const removeFeature = (item: AdminFeature) => {
    if (!window.confirm(`删除功能「${item.title}」？各组对它的授权会一并清除。`)) return
    void run(() => api.admin.deleteFeature(item.key))
  }

  const submitFeature = (input: {
    title: string
    desc: string
    tag: string
    icon: string
    url: string
    mode: string
    publicHost: string
  }) =>
    void run(async () => {
      if (featureDraft === 'new') await api.admin.createFeature(input)
      else if (featureDraft) await api.admin.updateFeature(featureDraft.key, input)
      setFeatureDraft(null)
    })

  /** 站点配置：保存草稿（校验失败时错误进 ErrorBanner，成功用返回值刷新） */
  const saveSite = () =>
    void run(async () => {
      const { config } = await api.admin.updateSite(siteConfig)
      setSiteConfig(config)
    })

  /** 分区切换：先淡出当前面板，换内容后再淡入（prefers-reduced-motion 时直接切） */
  const switchSection = (id: AdminSectionId) => {
    if (id === section) return
    const grid = gridRef.current
    if (!grid || window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      setSection(id)
      return
    }
    switchTweenRef.current?.kill()
    const current = Array.from(grid.querySelectorAll<HTMLElement>(':scope > section:not([hidden])'))
    const tl = gsap.timeline()
    switchTweenRef.current = tl
    tl.to(current, { autoAlpha: 0, duration: 0.15, ease: 'power1.out' })
    tl.add(() => {
      // flushSync：新面板在同一帧里完成挂载，紧接着的淡入才不会「闪」
      flushSync(() => setSection(id))
      const next = Array.from(grid.querySelectorAll<HTMLElement>(':scope > section:not([hidden])'))
      tl.fromTo(next, { autoAlpha: 0 }, { autoAlpha: 1, duration: 0.24, ease: 'power1.out' })
    })
  }

  if (stage === 'checking') {
    return (
      <div className="admin">
        <Orbs variant="pulse" />
        <Spinner label="正在校验权限…" />
      </div>
    )
  }

  if (stage === 'denied') {
    return (
      <div className="admin" ref={pageRef}>
        <Orbs variant="pulse" />
        <div className="glass panel" style={{ maxWidth: 460, margin: '10vh auto' }}>
          <h1 className="panel__title">无法访问管理后台</h1>
          <p className="field__hint">
            {me ? '当前账号不是管理员。' : '尚未登录，请先登录后再访问。'}
          </p>
          <div className="row">
            <button
              type="button"
              className="btn btn--primary btn--sm admin__back"
              aria-label="返回站点"
              title="返回站点"
              ref={backRef}
              onClick={backToSite}
            >
              <BackArrow />
            </button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="admin" ref={pageRef}>
      <Orbs variant="pulse" />

      <header className="admin__header">
        <div>
          <h1 className="section__title">管理后台</h1>
          <p className="field__hint">
            {themeLabel} · 当前账号 {me?.account}（{me?.nickname}）· 最近登录 IP {me?.ip}
          </p>
        </div>
        <div className="row">
          <button
            type="button"
            className="btn btn--glass btn--sm admin__back"
            aria-label="返回站点"
            title="返回站点"
            ref={backRef}
            onClick={backToSite}
          >
            <BackArrow />
          </button>
          <button type="button" className="btn btn--ghost btn--sm" onClick={() => void run(load)} disabled={busy}>
            刷新
          </button>
        </div>
      </header>

      <ErrorBanner message={error} />

      <div className="admin__grid admin__grid--with-nav" ref={gridRef}>
        {/* 左栏：文字条目（沿用主页侧栏的胶囊语言） */}
        <nav className="admin__nav" aria-label="后台分区">
          {ADMIN_SECTIONS.map((item) => (
            <button
              key={item.id}
              type="button"
              className="admin__nav-item"
              aria-current={section === item.id}
              onClick={() => switchSection(item.id)}
            >
              {item.label}
            </button>
          ))}
        </nav>

        {/* 邀请码 */}
        <section className="glass panel" hidden={section !== 'invites'}>
          <div className="panel__head">
            <h2 className="panel__title">邀请码</h2>
            <div className="row">
              <label className="chk">
                可用次数
                <input
                  className="field__input invite-num"
                  type="number"
                  min={1}
                  max={100}
                  value={inviteUses}
                  onChange={(e) => setInviteUses(Number(e.target.value))}
                  aria-label="可用次数"
                />
              </label>
              <label className="chk">
                有效期
                <select
                  className="field__input invite-num"
                  value={inviteDays}
                  onChange={(e) => setInviteDays(Number(e.target.value))}
                  aria-label="有效期"
                >
                  <option value={1}>1 天</option>
                  <option value={3}>3 天</option>
                  <option value={7}>7 天</option>
                  <option value={30}>30 天</option>
                </select>
              </label>
              <button type="button" className="btn btn--primary btn--sm" disabled={busy} onClick={createInvite}>
                生成邀请码
              </button>
            </div>
          </div>

          {invites.length === 0 ? (
            <p className="empty">还没有邀请码，生成一个用于注册。</p>
          ) : (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>邀请码</th>
                    <th>状态</th>
                    <th>过期时间</th>
                    <th>用量</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {invites.map((invite) => (
                    <tr key={invite.id}>
                      <td>
                        <code>{invite.code}</code>
                      </td>
                      <td>
                        <span
                          className={`pill ${
                            invite.status === 'available' ? 'pill--ok' : invite.status === 'used' ? 'pill--used' : 'pill--warn'
                          }`}
                        >
                          {invite.status === 'available' ? '可用' : invite.status === 'used' ? '已使用' : '已过期'}
                        </span>
                      </td>
                      <td>{formatTime(invite.expiresAt)}</td>
                      <td>
                        {invite.usedCount} / {invite.maxUses} 次
                      </td>
                      <td>
                        <div className="row">
                          <button
                            type="button"
                            className="btn btn--ghost btn--sm"
                            onClick={() => void navigator.clipboard?.writeText(invite.code)}
                          >
                            复制
                          </button>
                          {invite.status === 'available' ? (
                            <button
                              type="button"
                              className="btn btn--danger btn--sm"
                              disabled={busy}
                              onClick={() => void run(() => api.admin.revokeInvite(invite.id))}
                            >
                              撤销
                            </button>
                          ) : null}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        {/* 封禁名单 */}
        <section className="glass panel" hidden={section !== 'bans'}>
          <div className="panel__head">
            <h2 className="panel__title">封禁名单</h2>
            <div className="row">
              <input
                className="field__input"
                style={{ width: 200 }}
                placeholder="IP 地址"
                value={banIP}
                onChange={(e) => setBanIP(e.target.value)}
              />
              <label className="row" style={{ gap: 6, fontSize: 13 }}>
                <input type="checkbox" checked={banForever} onChange={(e) => setBanForever(e.target.checked)} />
                永久
              </label>
              <button
                type="button"
                className="btn btn--primary btn--sm"
                disabled={busy || banIP.trim() === ''}
                onClick={() =>
                  void run(async () => {
                    await api.admin.createBan(banIP.trim(), banForever)
                    setBanIP('')
                    setBanForever(false)
                  })
                }
              >
                封禁
              </button>
            </div>
          </div>

          {bans.length === 0 ? (
            <p className="empty">暂无封禁记录。</p>
          ) : (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>IP</th>
                    <th>触犯次数</th>
                    <th>状态</th>
                    <th>到期时间</th>
                    <th>原因</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {bans.map((ban) => (
                    <tr key={ban.ip}>
                      <td>
                        <code>{ban.ip}</code>
                      </td>
                      <td>{ban.offenseCount}</td>
                      <td>
                        <span className={`pill ${ban.active ? (ban.permanent ? 'pill--danger' : 'pill--warn') : 'pill--used'}`}>
                          {ban.active ? (ban.permanent ? '永久封禁' : '限时封禁') : '已失效'}
                        </span>
                      </td>
                      <td>{ban.permanent ? '—' : formatTime(ban.expiresAt)}</td>
                      <td>{ban.reason}</td>
                      <td>
                        <div className="row">
                          <button
                            type="button"
                            className="btn btn--ghost btn--sm"
                            disabled={busy || !ban.active}
                            onClick={() => void run(() => api.admin.unban(ban.ip))}
                          >
                            解封
                          </button>
                          <button
                            type="button"
                            className="btn btn--danger btn--sm"
                            disabled={busy}
                            onClick={() => void run(() => api.admin.resetBan(ban.ip))}
                          >
                            清除记录
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="field__hint">
            「解封」只结束当前封禁，触犯次数保留；「清除记录」会把次数归零，该 IP 再犯按首次处理。
          </p>
        </section>

        {/* 用户 */}
        <section className="glass panel" hidden={section !== 'users'}>
          <div className="panel__head">
            <h2 className="panel__title">用户</h2>
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>账号</th>
                  <th>昵称</th>
                  <th>最近 IP</th>
                  <th>角色</th>
                </tr>
              </thead>
              <tbody>
                {users.map((item) => (
                  <tr key={item.id}>
                    <td>{item.id}</td>
                    <td>
                      <code>{item.account}</code>
                    </td>
                    <td>{item.nickname}</td>
                    <td>
                      <code>{item.ip || '—'}</code>
                    </td>
                    <td>
                      <button
                        type="button"
                        className="btn btn--glass btn--sm"
                        disabled={busy || item.id === me?.id}
                        title={item.id === me?.id ? '不能修改自己的角色' : '点击切换角色'}
                        onClick={() => void run(() => api.admin.setRole(item.id, item.role === 'admin' ? 'user' : 'admin'))}
                      >
                        {item.role === 'admin' ? '管理员' : '普通用户'}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>

        {/* 用户组 */}
        <section className="glass panel" hidden={section !== 'groups'}>
          <div className="panel__head">
            <h2 className="panel__title">用户组</h2>
            <div className="row">
              <input
                className="field__input"
                style={{ width: 180 }}
                placeholder="新组名"
                value={newGroupName}
                onChange={(e) => setNewGroupName(e.target.value)}
                aria-label="新组名"
              />
              <button
                type="button"
                className="btn btn--primary btn--sm"
                disabled={busy || newGroupName.trim() === ''}
                onClick={createGroup}
              >
                新建用户组
              </button>
            </div>
          </div>

          {groups.length === 0 ? (
            <p className="empty">还没有用户组。建组并勾选「可查看的功能」，再把成员拉进来；未入组的用户点击功能会被后端拒绝。</p>
          ) : (
            groups.map((group) => (
              <div className="group-card" key={group.id}>
                <div className="row group-card__head">
                  <input
                    className="field__input"
                    style={{ width: 180 }}
                    defaultValue={group.name}
                    aria-label={`重命名用户组「${group.name}」`}
                    onBlur={(e) => {
                      const value = e.target.value.trim()
                      if (value && value !== group.name) renameGroup(group.id, value)
                    }}
                  />
                  <button
                    type="button"
                    className="btn btn--danger btn--sm"
                    disabled={busy}
                    onClick={() => deleteGroup(group)}
                  >
                    删除用户组
                  </button>
                </div>

                <span className="field__label">可查看的功能</span>
                <div className="row">
                  {adminFeatures.length === 0 ? (
                    <span className="field__hint">还没有功能入口，先去「功能配置」新建。</span>
                  ) : (
                    adminFeatures.map((feature) => (
                      <label className="chk" key={feature.key}>
                        <input
                          type="checkbox"
                          checked={group.features.includes(feature.key)}
                          disabled={busy}
                          onChange={(e) => toggleGroupFeature(group, feature.key, e.target.checked)}
                        />
                        {feature.title}
                      </label>
                    ))
                  )}
                </div>

                <span className="field__label">成员</span>
                <div className="row">
                  {users.length === 0 ? (
                    <span className="field__hint">还没有可分配的用户。</span>
                  ) : (
                    users.map((item) => (
                      <label className="chk" key={item.id}>
                        <input
                          type="checkbox"
                          checked={group.members.includes(item.id)}
                          disabled={busy}
                          onChange={(e) => toggleGroupMember(group, item.id, e.target.checked)}
                        />
                        {item.nickname}（{item.account}）
                      </label>
                    ))
                  )}
                </div>
              </div>
            ))
          )}
          <p className="field__hint">
            勾选「可查看」= 该组成员在主页点击此功能时后端放行（未授权点击会收到「您没有权限查看」）；
            管理员不受分组限制，游客一律不可点。
          </p>
        </section>

        {/* 功能配置 */}
        <section className="glass panel" hidden={section !== 'features'}>
          <div className="panel__head">
            <h2 className="panel__title">功能配置</h2>
            <button
              type="button"
              className="btn btn--primary btn--sm"
              disabled={busy}
              onClick={() => setFeatureDraft('new')}
            >
              新建功能
            </button>
          </div>
          <p className="field__hint">
            拖动左侧手柄调整主页显示顺序；内网地址只存服务端、点击时按用户组鉴权下发（留空 = 未开放）。
          </p>
          {adminFeatures.length === 0 ? (
            <p className="empty">还没有功能入口，点「新建功能」添加。</p>
          ) : (
            adminFeatures.map((item) => (
              <div className={`endpoint-row${dragKey === item.key ? ' is-dragging' : ''}`} key={item.key}
                onDragOver={(e) => {
                  e.preventDefault()
                  if (!dragKey || dragKey === item.key) return
                  // 拖动中即时预览排序，松手时再落库
                  setAdminFeatures((cur) => {
                    const from = cur.findIndex((f) => f.key === dragKey)
                    const to = cur.findIndex((f) => f.key === item.key)
                    if (from < 0 || to < 0 || from === to) return cur
                    const next = cur.slice()
                    const [moved] = next.splice(from, 1)
                    next.splice(to, 0, moved)
                    return next
                  })
                }}
                onDrop={(e) => e.preventDefault()}
              >
                <span
                  className="drag-handle"
                  draggable
                  role="button"
                  tabIndex={0}
                  title="拖动排序"
                  aria-label={`拖动「${item.title}」排序`}
                  onDragStart={(e) => {
                    setDragKey(item.key)
                    e.dataTransfer.effectAllowed = 'move'
                  }}
                  onDragEnd={() => {
                    setDragKey('')
                    void run(() => api.admin.reorderFeatures(adminFeatures.map((f) => f.key)))
                  }}
                >
                  ⠿
                </span>
                <span className="endpoint-row__name">{item.title}</span>
                <span
                  className="field__hint endpoint-row__url"
                  title={
                    item.mode === 'proxy'
                      ? `中转 → ${item.publicHost || '未配置对外主机名'}`
                      : item.url || '未配置地址'
                  }
                >
                  {item.mode === 'proxy' ? `中转 → ${item.publicHost || '未配置主机名'}` : item.url || '未配置地址'}
                </span>
                <button type="button" className="btn btn--glass btn--sm" disabled={busy} onClick={() => setFeatureDraft(item)}>
                  编辑
                </button>
                <button type="button" className="btn btn--danger btn--sm" disabled={busy} onClick={() => removeFeature(item)}>
                  删除
                </button>
              </div>
            ))
          )}
        </section>
        {/* 站点配置 */}
        <section className="glass panel" hidden={section !== 'site'}>
          <div className="panel__head">
            <h2 className="panel__title">站点配置</h2>
            <button type="button" className="btn btn--primary btn--sm" disabled={busy} onClick={saveSite}>
              保存
            </button>
          </div>
          <p className="field__hint">
            保存后立即生效（主页下次请求 /api/site 时更新）；直接修改服务器上的 data/site.json 也会被热加载。
            板块顺序即主页的排列顺序，取消勾选「前台可见」会把它从主页一并移除。
          </p>
          <SiteSettingsPanel config={siteConfig} onChange={setSiteConfig} />
        </section>      </div>

      {featureDraft ? (
        <FeatureEditModal
          draft={featureDraft}
          busy={busy}
          onClose={() => setFeatureDraft(null)}
          onSubmit={submitFeature}
        />
      ) : null}
    </div>
  )
}

/** 功能编辑弹窗：新建与编辑共用 */
function FeatureEditModal({
  draft,
  busy,
  onClose,
  onSubmit,
}: {
  draft: AdminFeature | 'new'
  busy: boolean
  onClose: () => void
  onSubmit: (input: {
    title: string
    desc: string
    tag: string
    icon: string
    url: string
    mode: string
    publicHost: string
  }) => void
}) {
  const base = draft === 'new' ? null : draft
  const [title, setTitle] = useState(base?.title ?? '')
  const [desc, setDesc] = useState(base?.desc ?? '')
  const [tag, setTag] = useState(base?.tag ?? '')
  const [icon, setIcon] = useState(base?.icon ?? 'terminal')
  const [url, setUrl] = useState(base?.url ?? '')
  const [mode, setMode] = useState<'redirect' | 'proxy'>(base?.mode === 'proxy' ? 'proxy' : 'redirect')
  const [publicHost, setPublicHost] = useState(base?.publicHost ?? '')

  const ready =
    title.trim() !== '' &&
    desc.trim() !== '' &&
    tag.trim() !== '' &&
    (mode === 'redirect' || publicHost.trim() !== '')

  return (
    <Modal
      title={base ? `编辑功能「${base.title}」` : '新建功能'}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn btn--ghost btn--sm" onClick={onClose}>
            取消
          </button>
          <button
            type="button"
            className="btn btn--primary btn--sm"
            disabled={busy || !ready}
            onClick={() =>
              onSubmit({ title, desc, tag, icon, url, mode, publicHost: mode === 'proxy' ? publicHost : '' })
            }
          >
            保存
          </button>
        </>
      }
    >
      <div className="feature-form">
        <Field label="标题" value={title} maxLength={12} onChange={(e) => setTitle(e.target.value)} />
        <Field label="描述" value={desc} maxLength={40} onChange={(e) => setDesc(e.target.value)} />
        <Field label="标签" value={tag} maxLength={6} onChange={(e) => setTag(e.target.value)} />
        <div className="field">
          <span className="field__label">点击行为</span>
          <div className="mode-pick" role="radiogroup" aria-label="点击行为">
            <button
              type="button"
              role="radio"
              aria-checked={mode === 'redirect'}
              className="mode-pick__item"
              onClick={() => setMode('redirect')}
            >
              <b>直跳</b>
              <small>浏览器直接打开内网地址（适合公网外链）</small>
            </button>
            <button
              type="button"
              role="radio"
              aria-checked={mode === 'proxy'}
              className="mode-pick__item"
              onClick={() => setMode('proxy')}
            >
              <b>中转网关</b>
              <small>经本站反代，上游零暴露、地址固定</small>
            </button>
          </div>
        </div>
        <div className="field">
          <span className="field__label">图标（共 {FEATURE_ICON_NAMES.length} 个）</span>
          <div className="icon-picker" role="radiogroup" aria-label="图标">
            {FEATURE_ICON_NAMES.map((name) => (
              <button
                key={name}
                type="button"
                className="icon-picker__item"
                role="radio"
                aria-checked={icon === name}
                aria-label={FEATURE_ICON_LABELS[name]}
                title={FEATURE_ICON_LABELS[name]}
                onClick={() => setIcon(name)}
              >
                <FeatureIcon name={name} size={18} />
              </button>
            ))}
          </div>
        </div>
        <Field
          label={mode === 'proxy' ? '上游地址（仅服务端可见）' : '内网地址'}
          hint={
            mode === 'proxy'
              ? '如 http://127.0.0.1:3000/；浏览器永远看不到它，由服务端转发'
              : 'http(s)://…；留空 = 暂不开放点击'
          }
          value={url}
          placeholder={mode === 'proxy' ? 'http://127.0.0.1:3000/' : 'http://192.168.x.x:端口/'}
          onChange={(e) => setUrl(e.target.value)}
        />
        {mode === 'proxy' ? (
          <Field
            label="对外主机名"
            hint="如 cvat.example.com：穿透/局域网把它解析到本服务器，网关按它认领该服务"
            value={publicHost}
            placeholder="cvat.example.com"
            onChange={(e) => setPublicHost(e.target.value)}
          />
        ) : null}
      </div>
    </Modal>
  )
}

/** 板块类型 → 编辑器里的标题（与渲染器同名，标明类型供排查） */
const SECTION_TYPE_NAMES: Record<SectionConfig['type'], string> = {
  intro: '首页 · intro',
  features: '功能 · features',
  contact: '联系 · contact',
  more: '更多 · more',
}

/** 站点内容编辑器：站名 / 欢迎语 + 各板块文案与类型专属明细。
    受控组件：所有修改走 onChange，由外层统一提交（服务端全量校验） */
function SiteSettingsPanel({
  config,
  onChange,
}: {
  config: SiteConfig
  onChange: (next: SiteConfig) => void
}) {
  const edit = (index: number, change: (item: SectionConfig) => SectionConfig) =>
    onChange({
      ...config,
      sections: config.sections.map((item, i) => (i === index ? change(item) : item)),
    })

  const move = (index: number, delta: number) => {
    const to = index + delta
    if (to < 0 || to >= config.sections.length) return
    const next = config.sections.slice()
    const [moved] = next.splice(index, 1)
    next.splice(to, 0, moved)
    onChange({ ...config, sections: next })
  }

  return (
    <div className="site-form">
      <div className="site-form__grid">
        <Field
          label="站点名（1-24 字）"
          value={config.brand}
          maxLength={24}
          onChange={(e) => onChange({ ...config, brand: e.target.value })}
        />
        <Field
          label="欢迎页标题（4-24 字）"
          value={config.welcome.title}
          maxLength={24}
          onChange={(e) => onChange({ ...config, welcome: { ...config.welcome, title: e.target.value } })}
        />
        <Field
          label="欢迎页副标题（1-60 字）"
          value={config.welcome.subtitle}
          maxLength={60}
          onChange={(e) => onChange({ ...config, welcome: { ...config.welcome, subtitle: e.target.value } })}
        />
      </div>

      {config.sections.map((item, index) => {
        const settings = item.settings ?? DEFAULT_MORE_SETTINGS
        return (
          <div className="glass site-sec" key={item.type}>
            <div className="row site-sec__head">
              <b>{SECTION_TYPE_NAMES[item.type]}</b>
              <label className="chk">
                <input
                  type="checkbox"
                  checked={item.visible}
                  onChange={(e) => edit(index, (s) => ({ ...s, visible: e.target.checked }))}
                />
                前台可见
              </label>
              <span className="row site-sec__ops">
                <button
                  type="button"
                  className="btn btn--ghost btn--sm"
                  disabled={index === 0}
                  onClick={() => move(index, -1)}
                >
                  上移
                </button>
                <button
                  type="button"
                  className="btn btn--ghost btn--sm"
                  disabled={index === config.sections.length - 1}
                  onClick={() => move(index, 1)}
                >
                  下移
                </button>
              </span>
            </div>

            <div className="site-form__grid">
              <Field
                label="侧栏导航名（1-6 字，动作口径）"
                value={item.label}
                maxLength={6}
                onChange={(e) => edit(index, (s) => ({ ...s, label: e.target.value }))}
              />
              <Field
                label="眉标（1-24 字）"
                value={item.eyebrow}
                maxLength={24}
                onChange={(e) => edit(index, (s) => ({ ...s, eyebrow: e.target.value }))}
              />
            </div>
            <Field
              label="标题（1-24 字）"
              value={item.title}
              maxLength={24}
              onChange={(e) => edit(index, (s) => ({ ...s, title: e.target.value }))}
            />
            <Field
              label="描述（1-160 字）"
              value={item.desc}
              maxLength={160}
              onChange={(e) => edit(index, (s) => ({ ...s, desc: e.target.value }))}
            />

            {item.type === 'intro' ? (
              <div className="field">
                <span className="field__label">数据卡（1-8 行；保存时自动去掉全空行）</span>
                {(item.stats ?? []).map((row, j) => (
                  <div className="row" key={j}>
                    <input
                      className="field__input"
                      style={{ width: 140 }}
                      placeholder="数值（1-24 字）"
                      maxLength={24}
                      value={row.value}
                      aria-label={`数据卡 ${j + 1} 数值`}
                      onChange={(e) =>
                        edit(index, (s) => ({
                          ...s,
                          stats: (s.stats ?? []).map((r, k) => (k === j ? { ...r, value: e.target.value } : r)),
                        }))
                      }
                    />
                    <input
                      className="field__input"
                      style={{ width: 200 }}
                      placeholder="标签（1-16 字）"
                      maxLength={16}
                      value={row.label}
                      aria-label={`数据卡 ${j + 1} 标签`}
                      onChange={(e) =>
                        edit(index, (s) => ({
                          ...s,
                          stats: (s.stats ?? []).map((r, k) => (k === j ? { ...r, label: e.target.value } : r)),
                        }))
                      }
                    />
                    <button
                      type="button"
                      className="btn btn--ghost btn--sm"
                      onClick={() =>
                        edit(index, (s) => ({ ...s, stats: (s.stats ?? []).filter((_, k) => k !== j) }))
                      }
                    >
                      删除
                    </button>
                  </div>
                ))}
                <button
                  type="button"
                  className="btn btn--ghost btn--sm"
                  onClick={() => edit(index, (s) => ({ ...s, stats: [...(s.stats ?? []), { value: '', label: '' }] }))}
                >
                  添加一行
                </button>
              </div>
            ) : null}

            {item.type === 'contact' ? (
              <div className="field">
                <span className="field__label">
                  联系方式（1-8 行；链接支持 mailto: 邮箱 或 http(s)://，可留空）
                </span>
                {(item.items ?? []).map((row, j) => (
                  <div className="row site-sec__row" key={j}>
                    <input
                      className="field__input"
                      style={{ width: 110 }}
                      placeholder="标题"
                      maxLength={12}
                      value={row.title}
                      aria-label={`条目 ${j + 1} 标题`}
                      onChange={(e) =>
                        edit(index, (s) => ({
                          ...s,
                          items: (s.items ?? []).map((r, k) => (k === j ? { ...r, title: e.target.value } : r)),
                        }))
                      }
                    />
                    <input
                      className="field__input"
                      style={{ width: 180 }}
                      placeholder="显示值"
                      maxLength={64}
                      value={row.value}
                      aria-label={`条目 ${j + 1} 显示值`}
                      onChange={(e) =>
                        edit(index, (s) => ({
                          ...s,
                          items: (s.items ?? []).map((r, k) => (k === j ? { ...r, value: e.target.value } : r)),
                        }))
                      }
                    />
                    <input
                      className="field__input"
                      style={{ width: 170 }}
                      placeholder="说明"
                      maxLength={40}
                      value={row.hint}
                      aria-label={`条目 ${j + 1} 说明`}
                      onChange={(e) =>
                        edit(index, (s) => ({
                          ...s,
                          items: (s.items ?? []).map((r, k) => (k === j ? { ...r, hint: e.target.value } : r)),
                        }))
                      }
                    />
                    <input
                      className="field__input"
                      style={{ width: 230 }}
                      placeholder="链接（可空）"
                      maxLength={512}
                      value={row.href ?? ''}
                      aria-label={`条目 ${j + 1} 链接`}
                      onChange={(e) =>
                        edit(index, (s) => ({
                          ...s,
                          items: (s.items ?? []).map((r, k) =>
                            k === j ? { ...r, href: e.target.value === '' ? undefined : e.target.value } : r,
                          ),
                        }))
                      }
                    />
                    <button
                      type="button"
                      className="btn btn--ghost btn--sm"
                      onClick={() => edit(index, (s) => ({ ...s, items: (s.items ?? []).filter((_, k) => k !== j) }))}
                    >
                      删除
                    </button>
                  </div>
                ))}
                <button
                  type="button"
                  className="btn btn--ghost btn--sm"
                  onClick={() =>
                    edit(index, (s) => ({
                      ...s,
                      items: [...(s.items ?? []), { title: '', value: '', hint: '' }],
                    }))
                  }
                >
                  添加一行
                </button>
              </div>
            ) : null}

            {item.type === 'more' ? (
              <div className="site-form__grid">
                <Field
                  label="设置区 · 跟随时间名称"
                  value={settings.follow.name}
                  maxLength={80}
                  onChange={(e) =>
                    edit(index, (s) => ({
                      ...s,
                      settings: { ...settings, follow: { ...settings.follow, name: e.target.value } },
                    }))
                  }
                />
                <Field
                  label="设置区 · 跟随时间说明"
                  value={settings.follow.hint}
                  maxLength={80}
                  onChange={(e) =>
                    edit(index, (s) => ({
                      ...s,
                      settings: { ...settings, follow: { ...settings.follow, hint: e.target.value } },
                    }))
                  }
                />
                <Field
                  label="设置区 · 主题时段名称"
                  value={settings.slot.name}
                  maxLength={80}
                  onChange={(e) =>
                    edit(index, (s) => ({
                      ...s,
                      settings: { ...settings, slot: { ...settings.slot, name: e.target.value } },
                    }))
                  }
                />
                <Field
                  label="设置区 · 跟随中的说明"
                  value={settings.slot.hintAuto}
                  maxLength={80}
                  onChange={(e) =>
                    edit(index, (s) => ({
                      ...s,
                      settings: { ...settings, slot: { ...settings.slot, hintAuto: e.target.value } },
                    }))
                  }
                />
                <Field
                  label="设置区 · 手动的说明"
                  value={settings.slot.hintManual}
                  maxLength={80}
                  onChange={(e) =>
                    edit(index, (s) => ({
                      ...s,
                      settings: { ...settings, slot: { ...settings.slot, hintManual: e.target.value } },
                    }))
                  }
                />
              </div>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}
