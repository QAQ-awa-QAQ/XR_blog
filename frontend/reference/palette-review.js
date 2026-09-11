// 时段色板对照页的逻辑。
// 只服务开发环境：不进入生产构建（Vite 仅以 index.html 为入口）。
// 纯 JS 书写，便于直接用 Vite 的 dev server 打开，不参与 tsc 检查。

import { ANCHORS, themeAt, luminance, TEXT_SWITCH_LUMINANCE } from '../src/theme/palette.ts'

const $ = (id) => document.getElementById(id)
const pad = (n) => String(n).padStart(2, '0')

const toRgb = (hex) => {
  const v = hex.replace('#', '')
  return [parseInt(v.slice(0, 2), 16), parseInt(v.slice(2, 4), 16), parseInt(v.slice(4, 6), 16)]
}

const at = (minutes) => {
  const date = new Date()
  date.setHours(0, Math.floor(minutes), 0, 0)
  return themeAt(date)
}

let follow = false

function render(minutes, fromScrub) {
  const hour = Math.floor(minutes / 60)
  const minute = minutes % 60
  const theme = at(minutes)

  $('clock').textContent = `${pad(hour)}:${pad(minute)}`
  if (fromScrub) {
    follow = false
    $('follow').setAttribute('aria-pressed', 'false')
  }

  document.documentElement.style.setProperty('--accent', theme.accent)
  document.documentElement.style.setProperty('--fg', theme.fg)
  document.documentElement.style.setProperty('--muted', theme.fgMuted)
  document.body.style.background = `linear-gradient(160deg, ${theme.bg1}, ${theme.bg2})`
  document.body.style.color = theme.fg

  $('nowSwatch').style.background = `linear-gradient(150deg, ${theme.bg1}, ${theme.bg2})`
  $('nowLabel').textContent = `${theme.label} · ${pad(hour)}:${pad(minute)}`

  const row = (key, value) =>
    `<div><code>${key}</code> = <code>${value}</code>
      <span class="swatch-inline" style="background:${value}"></span></div>`

  const backgroundLuminance = luminance(theme.bg1)
  $('nowValues').innerHTML = [
    row('--bg-1', theme.bg1),
    row('--bg-2', theme.bg2),
    row('--orb-a', theme.orbA),
    row('--orb-b', theme.orbB),
    row('--accent', theme.accent),
    row('--fg', theme.fg),
    `<div><code>glassAlpha</code> = <code>${theme.glassAlpha.toFixed(3)}</code></div>`,
    `<div>背景亮度 <strong>${backgroundLuminance.toFixed(1)}</strong>
      · 文字档位由阈值 <code>${TEXT_SWITCH_LUMINANCE}</code> 决定：
      <strong>${backgroundLuminance >= TEXT_SWITCH_LUMINANCE ? '深色文字' : '浅色文字'}</strong>
      · 对比度 <strong>${Math.abs(luminance(theme.fg) - backgroundLuminance).toFixed(1)}</strong></div>`,
  ].join('')

  $('liveSummary').innerHTML = `<span>
    插值结果：<strong>${theme.label}</strong> ·
    accent <code>${theme.accent}</code> ·
    背景 <code>${theme.bg1} → ${theme.bg2}</code>
  </span>`

  for (let i = 0; i < 8; i++) {
    const sample = at(i * 3 * 60)
    const cell = $('cell-' + i * 3)
    if (!cell) continue
    cell.style.setProperty('--c1', sample.bg1)
    cell.style.setProperty('--c2', sample.bg2)
    cell.style.color = sample.fg
    $('cell-label-' + i * 3).textContent = sample.label
    const glass = $('cell-glass-' + i * 3)
    glass.style.background = `rgba(255,255,255,${sample.glassAlpha.toFixed(3)})`
    glass.style.color = sample.fg
  }
}

// ---- 24 小时渐变条（96 段采样） ----
const stops = Array.from({ length: 97 }, (_, i) => {
  const minutes = Math.round((i / 96) * 1439)
  return `${at(minutes).bg1} ${((i / 96) * 100).toFixed(1)}%`
})
$('strip').style.background = `linear-gradient(90deg, ${stops.join(',')})`

// ---- 锚点卡 ----
$('anchors').innerHTML = ANCHORS.map(
  (anchor) => `
    <button class="anchor" data-hour="${anchor.hour}">
      <div class="anchor__swatch" style="background:linear-gradient(140deg,${anchor.bg1},${anchor.bg2})"></div>
      <div class="anchor__name">${anchor.label} · ${pad(anchor.hour)}:00</div>
      <div class="anchor__meta">accent ${anchor.accent} · 玻璃 ${anchor.glassAlpha}</div>
    </button>
  `,
).join('')

$('anchors').addEventListener('click', (event) => {
  const button = event.target.closest('.anchor')
  if (!button) return
  const minutes = Number(button.dataset.hour) * 60
  $('time').value = minutes
  render(minutes, true)
})

// ---- 每 3 小时取样 ----
$('samples').innerHTML = Array.from({ length: 8 }, (_, i) => i * 3)
  .map(
    (hour) => `
      <div class="cell" id="cell-${hour}">
        <div class="cell__time">${pad(hour)}:00 · <span id="cell-label-${hour}"></span></div>
        <div class="cell__glass" id="cell-glass-${hour}">玻璃卡片预演</div>
      </div>
    `,
  )
  .join('')

// ---- 连续性读数 ----
function reportContinuity() {
  const interpolated = ['bg1', 'bg2', 'orbA', 'orbB', 'accent']
  let worst = 0
  let worstAt = 0

  for (let minute = 0; minute < 1440; minute++) {
    const current = at(minute)
    const next = at((minute + 1) % 1440)
    for (const field of interpolated) {
      const a = toRgb(current[field])
      const b = toRgb(next[field])
      const delta = Math.max(...a.map((value, index) => Math.abs(value - b[index])))
      if (delta > worst) {
        worst = delta
        worstAt = minute
      }
    }
  }

  let switches = 0
  let previous = at(0).fg
  for (let minute = 0; minute < 1440; minute++) {
    if (at(minute).fg !== previous) {
      switches++
      previous = at(minute).fg
    }
  }

  const midnight = luminance(at(0).bg1)
  const noon = luminance(at(12 * 60).bg1)

  $('continuity').innerHTML = `
    <div>相邻 1 分钟的最大通道差：<strong>${worst}</strong>
      （在 <code>${pad(Math.floor(worstAt / 60))}:${pad(worstAt % 60)}</code>，≤ 3 视为无跳变）</div>
    <div>文字色一天切换 <strong>${switches}</strong> 次（理论值 2，更多说明阈值在抖动）</div>
    <div>凌晨亮度 <strong>${midnight.toFixed(1)}</strong> ／ 中午亮度 <strong>${noon.toFixed(1)}</strong>
      —— 夜间应明显更暗</div>
    <div>最差对比度出现在背景亮度接近阈值时，仍应远高于 0；
      这正是文字色必须离散而不能插值的原因。</div>
  `
}

// ---- 交互 ----
$('time').addEventListener('input', (event) => render(Number(event.target.value), true))
$('follow').addEventListener('click', () => {
  follow = !follow
  $('follow').setAttribute('aria-pressed', String(follow))
  if (follow) syncRealTime()
})

function syncRealTime() {
  const now = new Date()
  const minutes = now.getHours() * 60 + now.getMinutes()
  $('time').value = minutes
  render(minutes, false)
}

setInterval(() => follow && syncRealTime(), 1000)

syncRealTime()
reportContinuity()
