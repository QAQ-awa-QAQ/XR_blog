import { useEffect, useState } from 'react'
import { themeAt, type ThemeTokens } from './palette'

function alpha(hex: string, a: number): string {
  const v = hex.replace('#', '')
  const r = parseInt(v.slice(0, 2), 16)
  const g = parseInt(v.slice(2, 4), 16)
  const b = parseInt(v.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, ${a})`
}

/** 把主题写成 CSS 变量，全局样式表只消费变量，不写死颜色。 */
export function applyTheme(t: ThemeTokens) {
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
  document
    .querySelector('meta[name="theme-color"]')
    ?.setAttribute('content', t.bg1)
}

/**
 * 每分钟重新取色；CSS 侧有长过渡时间，因此视觉上永远是连续渐变，
 * 不会出现"到点切换"的突兀感。
 */
export function useTimeTheme(): ThemeTokens {
  const [tokens, setTokens] = useState<ThemeTokens>(() => themeAt(new Date()))

  useEffect(() => {
    const tick = () => setTokens(themeAt(new Date()))
    tick()
    const id = window.setInterval(tick, 60_000)
    return () => window.clearInterval(id)
  }, [])

  useEffect(() => {
    applyTheme(tokens)
  }, [tokens])

  return tokens
}
