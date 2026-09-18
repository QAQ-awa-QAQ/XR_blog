import { useState } from 'react'
import { useTimeTheme } from './theme/useTimeTheme'
import { useCursorGlow } from './motion/useCursorGlow'
import { HANDOFF_LEAD_MS } from './motion/tokens'
import { LiquidGlassDefs } from './components/LiquidGlassDefs'
import { Welcome } from './pages/Welcome'
import { Auth } from './pages/Auth'
import { AuthIntro } from './pages/AuthIntro'
import { Handoff, type HandoffFrom } from './pages/Handoff'
import { MainShell } from './pages/main/MainShell'
import { Admin } from './pages/Admin'
import { api, type User } from './api/client'

type Stage = 'welcome' | 'auth' | 'main'

/** 点击箭头后，转圈至少「弹出 0.4s → 完整展出 0.5s」再走下一步，避免一闪而过 */
const SPIN_MIN_MS = 900


/** 调试开关：true = 忽略已有会话（免密登录），点击箭头总是进登录/注册窗口。调试完改回 false */
const FORCE_LOGIN_WINDOW = true

export function App() {
  const theme = useTimeTheme()
  // 光标高光是整站行为（含欢迎页与后台），所以挂在这一层
  useCursorGlow()
  const [stage, setStage] = useState<Stage>('welcome')
  /** 进入主页的过场进行中：出场页与主页面同时在场，由 Handoff 编排动画。
      值 = 过场来源（欢迎页箭头 / 登录页的哪个按钮），null = 无过场 */
  const [handoffFrom, setHandoffFrom] = useState<HandoffFrom | null>(null)
  /** 「欢迎页 → 登录页」的过场进行中（欢迎页与登录页同时在场，由 AuthIntro 编排） */
  const [authIntro, setAuthIntro] = useState(false)
  const [user, setUser] = useState<User | null>(null)
  const [checking, setChecking] = useState(false)

  // 后台不占用 design.md 2.5 规定的侧栏入口，因此走独立路径。
  if (window.location.pathname.replace(/\/+$/, '') === '/admin') {
    return (
      <>
        <LiquidGlassDefs />
        <Admin themeLabel={theme.label} />
      </>
    )
  }

  /** 点击「登录 →」后先验一次登录态：已登录直接进 main，否则进登录/注册页。 */
  const enter = async () => {
    if (checking) return
    setChecking(true)
    const shownAt = performance.now()

    let ok = false
    try {
      const { user: current } = await api.session()
      setUser(current)
      ok = true
    } catch {
      ok = false
    }

    // 校验通常远快于转圈的「完整展出」，不足就补够；校验慢就直接用校验的时间
    const remain = SPIN_MIN_MS - (performance.now() - shownAt)
    if (remain > 0) await new Promise((resolve) => window.setTimeout(resolve, remain))

    setChecking(false)
    if (ok && !FORCE_LOGIN_WINDOW) {
      // 先让「转圈 → 箭头」的交接播完（转圈缩出、箭头淡回），再开始过场
      await new Promise((resolve) => window.setTimeout(resolve, HANDOFF_LEAD_MS))
      setStage('main')
      setHandoffFrom('welcome')
    } else {
      // 同一节奏：转圈交接回箭头，再播「欢迎页 → 登录页」的过场
      await new Promise((resolve) => window.setTimeout(resolve, HANDOFF_LEAD_MS))
      setStage('auth')
      setAuthIntro(true)
    }
  }

  const logout = async () => {
    try {
      await api.logout()
    } finally {
      setHandoffFrom(null)
      setUser(null)
      setStage('welcome')
    }
  }

  // 过场期间「出场页」要留在场上（内容左移出、按钮交给 Handoff 的幽灵）：
  // welcome 来源留欢迎页；其余来源留登录页（此时 stage 已是 main，靠 handoffFrom 把它留住）；
  // authIntro 期间欢迎页也要留场（AuthIntro 的演出背景）
  const handoff = handoffFrom !== null
  const showWelcome = stage === 'welcome' || handoffFrom === 'welcome' || authIntro
  const showAuth = stage === 'auth' || (handoffFrom !== null && handoffFrom !== 'welcome')

  return (
    <>
      <LiquidGlassDefs />
      {showWelcome ? <Welcome onEnter={enter} busy={checking} /> : null}
      {showAuth ? (
        <Auth
          intro={authIntro}
          onAuthenticated={(current, cta) => {
            setUser(current)
            setHandoffFrom(cta)
            setStage('main')
          }}
          onBack={() => setStage('welcome')}
        />
      ) : null}
      {authIntro ? <AuthIntro onDone={() => setAuthIntro(false)} /> : null}
      {stage === 'main' && user ? (
        <>
          <MainShell
            user={user}
            themeLabel={theme.label}
            accent={theme.accent}
            themeAuto={theme.auto}
            themeHour={theme.hour}
            onThemeAuto={theme.setAuto}
            onThemeHour={theme.setHour}
            onLogout={logout}
            handoff={handoff}
          />
          {handoffFrom ? <Handoff from={handoffFrom} onDone={() => setHandoffFrom(null)} /> : null}
        </>
      ) : null}
    </>
  )
}
