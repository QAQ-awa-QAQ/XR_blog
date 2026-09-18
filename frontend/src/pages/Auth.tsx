import { useEffect, useLayoutEffect, useRef, useState, type FormEvent } from 'react'
import gsap from 'gsap'
import { Orbs } from '../components/Orbs'
import { ErrorBanner, Field } from '../components/ui'
import { durations, easings, omega, HANDOFF_LEAD_MS } from '../motion/tokens'
import { springStep } from '../motion/math'
import type { HandoffFrom } from './Handoff'
import { ApiError, api, type User } from '../api/client'

type Mode = 'login' | 'register'

type Props = {
  /** cta = 点击的是哪个提交按钮（过场用它找克隆源） */
  onAuthenticated: (user: User, cta: HandoffFrom) => void
  onBack: () => void
  /** true = 入场动画由「欢迎页 → 登录页」过场（AuthIntro）接手，本组件不再自播 */
  intro?: boolean
}

/** 按钮内的「→」+ 转圈交接槽：与欢迎页同一套自绘线（样式见 .auth__cta 一节）。
    三个提交按钮共用；转圈常驻 DOM，显隐由 [data-busy] 的 CSS 过渡驱动（双向都有动画） */
function CtaArrow() {
  return (
    <span className="auth__cta-slot" aria-hidden="true">
      <svg className="auth__arrow" viewBox="-1.75 0 24 16">
        <line className="auth__arrow-shaft" x1="1.5" y1="8" x2="19" y2="8" />
        <path className="auth__arrow-head" d="M19 8 L12.2 2.8" />
        <path className="auth__arrow-head" d="M19 8 L12.2 13.2" />
      </svg>
      <span className="spinner auth__spinner" />
    </span>
  )
}

/** 弹簧的落位阈值：指数收尾会拖很久，剩余亚像素就直接吸附。
    平移按 ~0.9px（面板宽的 0.2%）收口 —— 比主页正文的 0.5px 稍宽：
    卡片内位移小，“停下来”的尾巴更显眼，实测把全过程从 ~1.3s 收敛到 ~0.7s */
const SNAP_PROGRESS = 0.002
const SNAP_HEIGHT = 0.05

export function Auth({ onAuthenticated, onBack, intro = false }: Props) {
  const cardRef = useRef<HTMLDivElement>(null)
  const tabsRef = useRef<HTMLDivElement>(null)
  const railRef = useRef<HTMLSpanElement>(null)
  const pillRef = useRef<HTMLSpanElement>(null)
  const tabRefs = useRef<(HTMLButtonElement | null)[]>([])
  /** 壳上那个「洞」的当前值。写给 GSAP tween，在 onUpdate 里写进 --cut-* ——
      洞与指示条**同一条曲线、同一时刻**移动（照搬侧栏做法） */
  const cutRef = useRef({ x: 0, y: 0, width: 0, height: 0 })

  /** 登录↔注册的两条弹簧（与主页正文同一套临界阻尼物理）：
      · progress 0↔1 —— 轨道横向平移，两块面板（内容）左右换位
      · height —— 舞台高度，大底框随之向下伸 / 向上缩
      同一 rAF 里逐帧追赶目标：来回连点会合并成最新目标并保留速度改向 */
  const stageRef = useRef<HTMLDivElement>(null)
  const trackRef = useRef<HTMLDivElement>(null)
  const panelRefs = useRef<(HTMLDivElement | null)[]>([])
  const springRef = useRef({ progress: 0, progressVel: 0, height: 0, heightVel: 0 })
  /** 锚顶基准 = 初始（登录态）面板高度：卡片居中布局下，高度每变化 Δ，
      顶边会漂 -Δ/2；rAF 里补 +Δ/2 的 relative top，让**卡片顶边不动**、
      切换栏位置完全稳定 */
  const baseHeightRef = useRef(0)
  /** 舞台裁剪口四周的外扩量（读自 CSS --stage-bleed）：高度目标 = 面板高 + 2×它 */
  const bleedRef = useRef(0)
  /** 每一步平移的距离 = 面板宽 + 面板间距（横向弹簧的目标单位） */
  const stepRef = useRef(0)
  const targetRef = useRef({ progress: 0, height: 0 })
  const modeRef = useRef<Mode>('login')

  const [mode, setMode] = useState<Mode>('login')
  /** '' = 空闲；'form' / 'guest' = 哪个按钮在忙（spinner 只出现在被点的那个按钮上） */
  const [busy, setBusy] = useState<'' | 'form' | 'guest'>('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const [account, setAccount] = useState('')
  const [password, setPassword] = useState('')
  const [nickname, setNickname] = useState('')
  const [inviteCode, setInviteCode] = useState('')

  /** 「登录 / 注册」指示条落位：完整照搬侧栏胶囊那一套 —— 用按钮的真实 rect
      相对轨道定位，切换时沿 snappy 曲线滑过去；同时把 --cut-* 写给壳（挖洞）。
      animate=false（首帧 / 视口变化）时直接放置 */
  const placePill = (animate: boolean) => {
    const tabs = tabsRef.current
    const rail = railRef.current
    const pill = pillRef.current
    const target = tabRefs.current[mode === 'login' ? 0 : 1]
    if (!tabs || !rail || !pill || !target) return

    const tabsBox = tabs.getBoundingClientRect()
    const railBox = rail.getBoundingClientRect()
    const box = target.getBoundingClientRect()

    const next = {
      x: box.left - tabsBox.left,
      y: box.top - tabsBox.top,
      width: box.width,
      height: box.height,
      opacity: 1,
    }
    // 洞的坐标以「轨道」为基准（--cut-* 写在 rail 上，壳与光点都从它继承）
    const cut = {
      x: box.left - railBox.left,
      y: box.top - railBox.top,
      width: box.width,
      height: box.height,
    }
    const writeCut = () => {
      const style = rail.style
      const value = cutRef.current
      style.setProperty('--cut-x', `${value.x}px`)
      style.setProperty('--cut-y', `${value.y}px`)
      style.setProperty('--cut-w', `${value.width}px`)
      style.setProperty('--cut-h', `${value.height}px`)
    }

    // 洞的形状：按「洞的真实像素尺寸 ∧ 圆角 = 洞高的一半」现算一颗完整胶囊，
    // viewBox 与 mask-size 的拉伸比 1:1 —— 屏幕上就是两个正圆 + 两条直线，无缝。
    // （写死一个 SVG 不行：preserveAspectRatio='none' 会把静态圆角按宽高各自拉伸成
    //   横扁的椭圆角；两个圆 + 矩形拼接又躲不掉抗锯齿接缝）
    const holeW = box.width + 2
    const holeH = box.height + 2
    const w = holeW.toFixed(2)
    const h = holeH.toFixed(2)
    const r = (holeH / 2).toFixed(2)
    rail.style.setProperty(
      '--hole',
      `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='${w}' height='${h}' viewBox='0 0 ${w} ${h}' preserveAspectRatio='none'%3E%3Crect width='${w}' height='${h}' rx='${r}' ry='${r}' fill='%23fff'/%3E%3C/svg%3E")`,
    )

    if (!animate || window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      gsap.set(pill, { ...next, autoRound: false })
      Object.assign(cutRef.current, cut)
      writeCut()
      return
    }
    // 洞与指示条同一曲线、同一时刻（照搬侧栏：两条 tween 同参）
    gsap.to(cutRef.current, {
      ...cut,
      duration: durations.navSlide,
      ease: easings.snappy,
      onUpdate: writeCut,
    })
    gsap.to(pill, {
      ...next,
      duration: durations.navSlide,
      ease: easings.snappy,
      overwrite: 'auto',
      // 关掉取整：指示条要与按钮像素级重合
      autoRound: false,
    })
  }

  // ⚠️ 本 effect 必须排在卡片入场动画（下一个 effect）**前面**：gsap.from 会立即把卡片
  // 置为 scale .98，先量就会量到缩放后的 rect（指示条宽度差 2%）。
  // effect 按声明顺序执行，排在前面才量得到真实终态
  useEffect(() => {
    const first = pillRef.current?.dataset.ready !== 'true'
    if (pillRef.current) pillRef.current.dataset.ready = 'true'
    placePill(!first)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode])

  // 视口变化：指示条重新落位（不滑动）；舞台重测后**直接落位**（不弹簧），
  // 并刷新锚顶基准与轨道宽
  useEffect(() => {
    const relayout = () => {
      placePill(false)
      const stage = stageRef.current
      const login = panelRefs.current[0]
      const index = modeRef.current === 'register' ? 1 : 0
      const panel = panelRefs.current[index]
      if (!stage || !login || !panel) return
      const track = trackRef.current
      if (!track) return
      bleedRef.current = parseFloat(getComputedStyle(stage).paddingTop) || 0
      const bleed = bleedRef.current
      const gap = parseFloat(getComputedStyle(track).columnGap) || 0
      stepRef.current = login.getBoundingClientRect().width + gap
      const height = panel.offsetHeight + bleed * 2
      baseHeightRef.current = login.offsetHeight + bleed * 2
      const spring = springRef.current
      spring.progress = index
      spring.progressVel = 0
      spring.height = height
      spring.heightVel = 0
      targetRef.current = { progress: index, height }
      stage.style.height = `${height}px`
    }
    window.addEventListener('resize', relayout)
    return () => window.removeEventListener('resize', relayout)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode])

  // 初次测量必须发生在浏览器绘制**之前**：stage 高度默认 auto 会被更高的注册面板
  // 撑高（两块面板并排，track 高度取 max），绘制前写入登录高度才不会闪一帧
  useLayoutEffect(() => {
    const stage = stageRef.current
    const track = trackRef.current
    const login = panelRefs.current[0]
    if (!stage || !track || !login) return
    // 外扩量与面板间距都从 CSS 读，避免数值在两边各维护一份
    bleedRef.current = parseFloat(getComputedStyle(stage).paddingTop) || 0
    const bleed = bleedRef.current
    const gap = parseFloat(getComputedStyle(track).columnGap) || 0
    const height = login.offsetHeight + bleed * 2
    baseHeightRef.current = height
    springRef.current.height = height
    springRef.current.progress = 0
    targetRef.current = { progress: 0, height }
    stepRef.current = login.getBoundingClientRect().width + gap
    stage.style.height = `${height}px`
  }, [])

  // 唯一的时间源：两条临界阻尼弹簧（机制照搬主页正文 —— 目标随时可改，
  // 位置与速度从当前状态接续；来回连点不会重播）
  useEffect(() => {
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    const spring = springRef.current
    let raf = 0
    let last = performance.now()

    const tick = (now: number) => {
      const dt = Math.min(0.05, (now - last) / 1000)
      last = now
      const target = targetRef.current

      if (reduce) {
        spring.progress = target.progress
        spring.progressVel = 0
        spring.height = target.height
        spring.heightVel = 0
      } else {
        let [progress, progressVel] = springStep(
          spring.progress,
          spring.progressVel,
          target.progress,
          omega.authSlide,
          dt,
        )
        if (Math.abs(progress - target.progress) < SNAP_PROGRESS) {
          progress = target.progress
          progressVel = 0
        }
        spring.progress = progress
        spring.progressVel = progressVel

        let [height, heightVel] = springStep(spring.height, spring.heightVel, target.height, omega.page, dt)
        if (Math.abs(height - target.height) < SNAP_HEIGHT) {
          height = target.height
          heightVel = 0
        }
        spring.height = height
        spring.heightVel = heightVel
      }

      const track = trackRef.current
      const stage = stageRef.current
      const card = cardRef.current
      if (track) {
        track.style.transform = `translate3d(${(-spring.progress * stepRef.current).toFixed(2)}px, 0, 0)`
      }
      if (stage) stage.style.height = `${spring.height.toFixed(2)}px`
      // 锚顶补偿：卡片在网格里居中，高度每变化 Δ 顶边漂 -Δ/2，补 +Δ/2
      if (card) card.style.top = `${((spring.height - baseHeightRef.current) / 2).toFixed(2)}px`

      raf = requestAnimationFrame(tick)
    }

    raf = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf)
  }, [])

  // 面板自身尺寸变化（错误横幅出现/消失、字体晚到）也要让高度目标跟上；
  // 只观察面板、不观察经弹簧写高度的舞台，避免每帧回调
  useEffect(() => {
    const observer = new ResizeObserver(() => {
      const index = modeRef.current === 'register' ? 1 : 0
      const panel = panelRefs.current[index]
      if (!panel) return
      targetRef.current.height = panel.offsetHeight + bleedRef.current * 2
    })
    panelRefs.current.forEach((panel) => panel && observer.observe(panel))
    return () => observer.disconnect()
  }, [])

  const introRef = useRef(intro)
  useEffect(() => {
    // 「欢迎页 → 登录页」过场期间跳过自播：卡片/按钮由 AuthIntro 编排
    // （用 ref 读初值：过场结束后 intro 变 false，不能触发重播）
    if (introRef.current) return
    const ctx = gsap.context(() => {
      if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
      gsap.from(cardRef.current, {
        opacity: 0,
        y: 24,
        scale: 0.98,
        duration: durations.panelEnter,
        ease: easings.smooth,
      })
    }, cardRef)
    return () => ctx.revert()
  }, [])

  // 切换登录 / 注册：清空残留提示，并把两条弹簧的目标换到新面板。
  // （高度目标里若还临时含着错误横幅，清空生效后由 ResizeObserver 修正，
  //   弹簧从当前位置接续，不会重播）
  useEffect(() => {
    modeRef.current = mode
    setError('')
    setNotice('')
    const panel = panelRefs.current[mode === 'register' ? 1 : 0]
    if (panel) {
      targetRef.current = {
        progress: mode === 'register' ? 1 : 0,
        height: panel.offsetHeight + bleedRef.current * 2,
      }
    }
  }, [mode])

  /** 认证失败的统一分支：封禁 / 冷却 / 业务错误 / 网络异常 */
  const handleAuthError = (err: unknown) => {
    if (err instanceof ApiError) {
      if (err.code === 'banned') {
        setError('你涉嫌网络攻击已被封禁，无法继续访问。')
      } else if (err.code === 'cooldown') {
        setNotice(`操作过于频繁，请 ${err.retryAfter ?? 10} 秒后再试。`)
      } else {
        setError(err.message)
      }
    } else {
      setError('网络异常，请稍后再试')
    }
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy('form')
    setError('')
    setNotice('')

    try {
      const result =
        mode === 'login'
          ? await api.login(account.trim(), password)
          : await api.register({
              account: account.trim(),
              password,
              nickname: nickname.trim(),
              inviteCode: inviteCode.trim(),
            })
      // 先让转圈交接回箭头（与欢迎页同一节奏），再交给过场：
      // 幽灵克隆的是"箭头已回来"的按钮
      setBusy('')
      await new Promise((resolve) => window.setTimeout(resolve, HANDOFF_LEAD_MS))
      onAuthenticated(result.user, mode === 'login' ? 'login-submit' : 'register-submit')
    } catch (err) {
      handleAuthError(err)
    } finally {
      setBusy('')
    }
  }

  /** 访客登录：不创建账号直接进入（只读；管理接口由后端角色判定拒绝） */
  const enterAsGuest = async () => {
    setBusy('guest')
    setError('')
    setNotice('')

    try {
      const result = await api.guest()
      // 同 submit：先交接回箭头，再起飞
      setBusy('')
      await new Promise((resolve) => window.setTimeout(resolve, HANDOFF_LEAD_MS))
      onAuthenticated(result.user, 'login-guest')
    } catch (err) {
      handleAuthError(err)
    } finally {
      setBusy('')
    }
  }

  return (
    <div className="auth">
      <Orbs variant="drift" />
      <div className="glass auth__card" ref={cardRef}>
        <div className="auth__tabs" role="tablist" aria-label="登录或注册" ref={tabsRef}>
          {/* 轨道与壳（照搬横边栏）：假边线被胶囊挖洞，四个断头挂常亮光点。
              洞的位置由 placePill 写在 --cut-* 上 */}
          <span className="auth__rail" ref={railRef} aria-hidden="true">
            <span className="auth__edge">
              <span className="auth__edge-heads" />
              <span className="auth__edge-heads-glow" />
            </span>
          </span>
          {/* 选中胶囊：玻璃片，位置由 placePill 写；垫在按钮下面 */}
          <span className="auth__tab-pill" ref={pillRef} aria-hidden="true" />
          <button
            type="button"
            role="tab"
            className="auth__tab"
            aria-selected={mode === 'login'}
            ref={(element) => {
              tabRefs.current[0] = element
            }}
            onClick={() => setMode('login')}
          >
            登录
          </button>
          <button
            type="button"
            role="tab"
            className="auth__tab"
            aria-selected={mode === 'register'}
            ref={(element) => {
              tabRefs.current[1] = element
            }}
            onClick={() => setMode('register')}
          >
            注册
          </button>
        </div>

        {/* 舞台=裁窗口（高度由弹簧写）；轨道把两块面板并排、横向平移。
            锚顶：高度变化时卡片顶边不动（补偿写在 rAF 里），切换栏位置因此完全稳定 */}
        <div className="auth__stage" ref={stageRef}>
          <div className="auth__track" ref={trackRef}>
            <div
              className="auth__panel"
              ref={(element) => {
                panelRefs.current[0] = element
              }}
              inert={mode !== 'login'}
            >
              <h2 className="auth__title">欢迎回来</h2>

              {error ? <ErrorBanner message={error} /> : null}
              {notice ? <ErrorBanner message={notice} tone="warn" /> : null}

              <form className="auth__form" onSubmit={submit}>
                <Field
                  label="账号"
                  name="account"
                  autoComplete="username"
                  placeholder="3-20 位字母、数字或下划线"
                  value={account}
                  onChange={(e) => setAccount(e.target.value)}
                  required
                />

                <Field
                  label="密码"
                  name="password"
                  type="password"
                  autoComplete="current-password"
                  placeholder="至少 8 位"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                />

                <button
                  type="submit"
                  className="btn btn--primary auth__cta"
                  data-cta="login-submit"
                  data-busy={busy === 'form' || undefined}
                  disabled={busy !== ''}
                >
                  <span className="auth__cta-label">登 录</span>
                  <CtaArrow />
                </button>

                <button
                  type="button"
                  className="btn btn--ghost auth__cta"
                  data-cta="login-guest"
                  data-busy={busy === 'guest' || undefined}
                  disabled={busy !== ''}
                  onClick={enterAsGuest}
                >
                  <span className="auth__cta-label">访客登录</span>
                  <CtaArrow />
                </button>
              </form>

              <div className="auth__footer">
                <button type="button" className="sidebar__link" onClick={onBack}>
                  ← 返回欢迎页
                </button>
              </div>
            </div>

            <div
              className="auth__panel"
              ref={(element) => {
                panelRefs.current[1] = element
              }}
              inert={mode !== 'register'}
            >
              <h2 className="auth__title">创建账号</h2>

              {error ? <ErrorBanner message={error} /> : null}
              {notice ? <ErrorBanner message={notice} tone="warn" /> : null}

              <form className="auth__form" onSubmit={submit}>
                <Field
                  label="账号"
                  name="account"
                  autoComplete="username"
                  placeholder="3-20 位字母、数字或下划线"
                  value={account}
                  onChange={(e) => setAccount(e.target.value)}
                  required
                />

                <Field
                  label="密码"
                  name="password"
                  type="password"
                  autoComplete="new-password"
                  placeholder="至少 8 位"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                />

                <Field
                  label="昵称"
                  name="nickname"
                  autoComplete="nickname"
                  placeholder="展示用，1-20 个字符"
                  value={nickname}
                  onChange={(e) => setNickname(e.target.value)}
                  required
                />

                <Field
                  label="邀请码"
                  name="inviteCode"
                  placeholder="由管理员生成"
                  hint="本站在此阶段仅接受持邀请码的注册"
                  value={inviteCode}
                  onChange={(e) => setInviteCode(e.target.value)}
                  required
                />

                <button
                  type="submit"
                  className="btn btn--primary auth__cta"
                  data-cta="register-submit"
                  data-busy={busy === 'form' || undefined}
                  disabled={busy !== ''}
                >
                  <span className="auth__cta-label">注 册</span>
                  <CtaArrow />
                </button>
              </form>

              <div className="auth__footer">
                <button type="button" className="sidebar__link" onClick={onBack}>
                  ← 返回欢迎页
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
