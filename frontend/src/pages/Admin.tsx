import { useCallback, useEffect, useRef, useState } from 'react'
import gsap from 'gsap'
import { Orbs } from '../components/Orbs'
import { ErrorBanner, Spinner } from '../components/ui'
import { easings } from '../motion/tokens'
import { ApiError, api, type Ban, type Invite, type User } from '../api/client'

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

  const pageRef = useRef<HTMLDivElement>(null)
  const backRef = useRef<HTMLButtonElement>(null)

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
      gsap.from(page.querySelectorAll('.admin__header p, .admin__header .row, .admin__grid > *'), {
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
    const [inviteRes, banRes, userRes] = await Promise.all([
      api.admin.listInvites(),
      api.admin.listBans(),
      api.admin.listUsers(),
    ])
    setInvites(inviteRes.invites)
    setBans(banRes.bans)
    setUsers(userRes.users)
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

      <div className="admin__grid">
        {/* 邀请码 */}
        <section className="glass panel">
          <div className="panel__head">
            <h2 className="panel__title">邀请码</h2>
            <button
              type="button"
              className="btn btn--primary btn--sm"
              disabled={busy}
              onClick={() => void run(async () => void (await api.admin.createInvite()))}
            >
              生成邀请码
            </button>
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
                    <th>使用者</th>
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
                      <td>{invite.usedBy ?? '—'}</td>
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
        <section className="glass panel">
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
        <section className="glass panel">
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
      </div>
    </div>
  )
}
