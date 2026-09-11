import { useState } from 'react'
import { useTimeTheme } from './theme/useTimeTheme'
import { Welcome } from './pages/Welcome'
import { Auth } from './pages/Auth'
import { MainShell } from './pages/main/MainShell'
import { Admin } from './pages/Admin'
import { api, type User } from './api/client'

type Stage = 'welcome' | 'auth' | 'main'

export function App() {
  const theme = useTimeTheme()
  const [stage, setStage] = useState<Stage>('welcome')
  const [user, setUser] = useState<User | null>(null)
  const [checking, setChecking] = useState(false)

  // 后台不占用 design.md 2.5 规定的三个侧栏入口，因此走独立路径。
  if (window.location.pathname.replace(/\/+$/, '') === '/admin') {
    return <Admin themeLabel={theme.label} />
  }

  /** 点击「登录 →」后先验一次登录态：已登录直接进 main，否则进登录/注册页。 */
  const enter = async () => {
    setChecking(true)
    try {
      const { user: current } = await api.session()
      setUser(current)
      setStage('main')
    } catch {
      setStage('auth')
    } finally {
      setChecking(false)
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

  return (
    <>
      {stage === 'welcome' ? <Welcome onEnter={enter} busy={checking} /> : null}
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
        <MainShell user={user} themeLabel={theme.label} onLogout={logout} />
      ) : null}
    </>
  )
}
