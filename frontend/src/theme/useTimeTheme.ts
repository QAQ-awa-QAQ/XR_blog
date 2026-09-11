import { useCallback, useEffect, useState } from 'react'
import { themeAt, type ThemeTokens } from './palette'

function alpha(hex: string, a: number): string {
  const v = hex.replace('#', '')
  const r = parseInt(v.slice(0, 2), 16)
  const g = parseInt(v.slice(2, 4), 16)
  const b = parseInt(v.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, ${a})`
}

/** 把主题写成 CSS 变量，全局样式表只消费变量，不写死颜色。 */
export function applyTheme(t: ThemeTokens, now = new Date()) {
  const root = document.documentElement.style
  root.setProperty('--bg-1', t.bg1)
  root.setProperty('--bg-2', t.bg2)
  root.setProperty('--orb-a', t.orbA)
  root.setProperty('--orb-b', t.orbB)
  root.setProperty('--accent', t.accent)
  root.setProperty('--accent-soft', alpha(t.accent, 0.14))
  root.setProperty('--fg', t.fg)
  root.setProperty('--fg-muted', t.fgMuted)
  root.setProperty('--glass-1', `rgba(255,255,255,${t.glassAlpha.toFixed(3)})`)
  root.setProperty('--glass-2', `rgba(255,255,255,${(t.glassAlpha * 0.55).toFixed(3)})`)
  root.setProperty('--glass-border', `rgba(255,255,255,${Math.min(0.75, t.glassAlpha * 2.6).toFixed(3)})`)
  root.setProperty('--glass-shadow', `rgba(15,23,42,${(0.05 + t.glassAlpha * 0.5).toFixed(3)})`)

  // 把实际生效的参数写到 DOM 上：便于界面展示，也让"颜色是否连续"可被脚本断言
  document.documentElement.dataset.timeTheme = JSON.stringify({
    label: t.label,
    accent: t.accent,
    bg1: t.bg1,
    fg: t.fg,
    glassAlpha: Number(t.glassAlpha.toFixed(3)),
    at: `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`,
  })

  document
    .querySelector('meta[name="theme-color"]')
    ?.setAttribute('content', t.bg1)
}

/** 主题偏好：跟随本地时间，还是固定到某个时段 */
export type ThemeControls = {
  /** 是否跟随本地时间（**默认开**） */
  auto: boolean
  /** 滑块上的取值 0~24：auto 时等于当前时刻，否则等于固定的时段 */
  hour: number
  setAuto: (auto: boolean) => void
  setHour: (hour: number) => void
}

const PREF_KEY = 'xr-blog:theme-pref'

/** 读偏好。默认**跟随时间**；localStorage 不可用（隐私模式）时同样退回默认值。 */
function readPref(): { auto: boolean; hour: number } {
  const fallback = { auto: true, hour: new Date().getHours() }
  try {
    const raw = window.localStorage.getItem(PREF_KEY)
    if (!raw) return fallback
    const saved = JSON.parse(raw) as Partial<{ auto: boolean; hour: number }>
    if (typeof saved.hour !== 'number' || !Number.isFinite(saved.hour)) return fallback
    return { auto: saved.auto !== false, hour: Math.min(24, Math.max(0, saved.hour)) }
  } catch {
    return fallback
  }
}

/** 固定时段用的取样时刻（24 点回绕到 0 点） */
function dateAtHour(hour: number): Date {
  const d = new Date()
  d.setHours(Math.floor(hour) % 24, Math.round((hour % 1) * 60), 0, 0)
  return d
}

function nowHour(): number {
  const d = new Date()
  return d.getHours() + d.getMinutes() / 60
}

/**
 * 主题：默认**跟随本地时间** —— 每分钟重新取色，而 CSS 侧有长过渡，
 * 所以视觉上永远是连续渐变，不会出现「到点切换」的突跳。
 *
 * 也可以在「更多」页把「跟随时间」关掉、用滑块把主题固定到某个时段。
 * 偏好写在 localStorage，刷新后还记得；写了读不了（隐私模式）就当默认值用。
 */
export function useTimeTheme(): ThemeTokens & ThemeControls {
  const [pref, setPref] = useState(readPref)
  const [tokens, setTokens] = useState(() =>
    themeAt(pref.auto ? new Date() : dateAtHour(pref.hour)),
  )

  useEffect(() => {
    const tick = () => setTokens(themeAt(pref.auto ? new Date() : dateAtHour(pref.hour)))
    tick()
    // 手动档不会自己变，没必要每分钟重取
    if (!pref.auto) return
    const id = window.setInterval(tick, 60_000)
    return () => window.clearInterval(id)
  }, [pref.auto, pref.hour])

  useEffect(() => {
    applyTheme(tokens)
  }, [tokens])

  useEffect(() => {
    try {
      window.localStorage.setItem(PREF_KEY, JSON.stringify(pref))
    } catch {
      // 隐私模式 / 配额满：记不住就算了，不影响本次使用
    }
  }, [pref])

  // 切回自动时把滑块拉回「现在」，否则它会停在旧的手动位置
  const setAuto = useCallback((auto: boolean) => {
    setPref((p) => (p.auto === auto ? p : { auto, hour: auto ? new Date().getHours() : p.hour }))
  }, [])
  // 拖滑块 = 接管为手动（不再另设一个「手动模式」开关，少一个状态）
  const setHour = useCallback((hour: number) => setPref({ auto: false, hour }), [])

  return {
    ...tokens,
    auto: pref.auto,
    hour: pref.auto ? nowHour() : pref.hour,
    setAuto,
    setHour,
  }
}
