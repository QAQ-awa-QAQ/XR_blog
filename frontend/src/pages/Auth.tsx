import { useEffect, useRef, useState, type FormEvent } from 'react'
import gsap from 'gsap'
import { Orbs } from '../components/Orbs'
import { ErrorBanner, Field } from '../components/ui'
import { durations, easings } from '../motion/tokens'
import { ApiError, api, type User } from '../api/client'

type Mode = 'login' | 'register'

type Props = {
  onAuthenticated: (user: User) => void
  onBack: () => void
}

export function Auth({ onAuthenticated, onBack }: Props) {
  const cardRef = useRef<HTMLDivElement>(null)
  const [mode, setMode] = useState<Mode>('login')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const [account, setAccount] = useState('')
  const [password, setPassword] = useState('')
  const [nickname, setNickname] = useState('')
  const [inviteCode, setInviteCode] = useState('')

  useEffect(() => {
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

  // 切换登录 / 注册时清空提示，避免残留的错误信息误导
  useEffect(() => {
    setError('')
    setNotice('')
  }, [mode])

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
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
      onAuthenticated(result.user)
    } catch (err) {
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
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="auth">
      <Orbs variant="drift" />
      <div className="glass auth__card" ref={cardRef}>
        <div className="auth__tabs" role="tablist" aria-label="登录或注册">
          <button
            type="button"
            role="tab"
            className="auth__tab"
            aria-selected={mode === 'login'}
            onClick={() => setMode('login')}
          >
            登录
          </button>
          <button
            type="button"
            role="tab"
            className="auth__tab"
            aria-selected={mode === 'register'}
            onClick={() => setMode('register')}
          >
            注册
          </button>
        </div>

        <h2 className="auth__title">{mode === 'login' ? '欢迎回来' : '创建账号'}</h2>

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
            autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
            placeholder="至少 8 位"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />

          {mode === 'register' ? (
            <>
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
            </>
          ) : null}

          <button type="submit" className="btn btn--primary" disabled={busy}>
            {busy ? <span className="spinner" /> : null}
            {mode === 'login' ? '登 录' : '注 册'}
          </button>
        </form>

        <div className="auth__footer">
          <button type="button" className="sidebar__link" onClick={onBack}>
            ← 返回欢迎页
          </button>
        </div>
      </div>
    </div>
  )
}
