import { useState } from 'react'
import { useTimeTheme } from './theme/useTimeTheme'
import { useCursorGlow } from './motion/useCursorGlow'
import { LiquidGlassDefs } from './components/LiquidGlassDefs'
import { Welcome } from './pages/Welcome'
import { Auth } from './pages/Auth'
import { Handoff } from './pages/Handoff'
import { MainShell } from './pages/main/MainShell'
import { Admin } from './pages/Admin'
import { api, type User } from './api/client'

type Stage = 'welcome' | 'auth' | 'main'

/** 点击箭头后，转圈至少「弹出 0.4s → 完整展出 0.5s」再走下一步，避免一闪而过 */
const SPIN_MIN_MS = 900

/** 转圈缩出 + 箭头淡回的交接时间（与 .welcome__cta 的 CSS 过渡时长对应）：
    交接播完再启动过场，否则按钮会被过场瞬间隐藏，“转圈→箭头”看不到 */
const HANDOFF_LEAD_MS = 300

export function App() {
  const theme = useTimeTheme()
  // 光标高光是整站行为（含欢迎页与后台），所以挂在这一层
  useCursorGlow()
  const [stage, setStage] = useState<Stage>('welcome')
  /** 登录成功 → 主页的过场进行中：欢迎页与主页面同时在场，由 Handoff 编排动画 */
  const [handoff, setHandoff] = useState(false)
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
    if (ok) {
      // 先让「转圈 → 箭头」的交接播完（转圈缩出、箭头淡回），再开始过场
      await new Promise((resolve) => window.setTimeout(resolve, HANDOFF_LEAD_MS))
      setStage('main')
      setHandoff(true)
    } else {
      setStage('auth')
    }
  }

  const logout = async () => {
    try {
      await api.logout()
    } finally {
      setUser(null)
      setStage('welcome')
    }
  }

  // 过场期间欢迎页要留在场上（内容左移出、箭头交给 Handoff 的幽灵），所以这里带上 handoff
  const showWelcome = stage === 'welcome' || handoff

  return (
    <>
      <LiquidGlassDefs />
      {showWelcome ? <Welcome onEnter={enter} busy={checking} /> : null}
      {stage === 'auth' ? (
        <Auth
          onAuthenticated={(current) => {
            setUser(current)
            setStage('main')
          }}
          onBack={() => setStage('welcome')}
        />
      ) : null}
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
          {handoff ? <Handoff onDone={() => setHandoff(false)} /> : null}
        </>
      ) : null}
    </>
  )
}
