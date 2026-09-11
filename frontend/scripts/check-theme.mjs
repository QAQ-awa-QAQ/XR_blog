import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  ANCHORS,
  TEXT_ON_DARK,
  TEXT_ON_LIGHT,
  TEXT_SWITCH_LUMINANCE,
  luminance,
  themeAt,
} from '../src/theme/palette.ts'

/**
 * 时段主题校验。
 *
 * 主题色随时间连续变化，一旦插值写错就会出现肉眼难查的跳变、夜间过亮，
 * 或者（本轮实际抓到过的）文字与背景亮度交叉导致文字不可读。
 * 这里把四条不变量固定下来：锚点精确、逐分钟连续、夜间明显更暗、对比度始终足够。
 */

const at = (minutes) => {
  const date = new Date()
  date.setHours(0, Math.floor(minutes), 0, 0)
  return themeAt(date)
}

const toRgb = (hex) => {
  const v = hex.replace('#', '')
  return [parseInt(v.slice(0, 2), 16), parseInt(v.slice(2, 4), 16), parseInt(v.slice(4, 6), 16)]
}

/** 参与连续插值的字段（文字色离散，不在其中） */
const INTERPOLATED = ['bg1', 'bg2', 'orbA', 'orbB', 'accent']

/** 相邻 1 分钟之间允许的最大通道差；超过就说明出现了可感知的跳变 */
const MAX_STEP = 3

/** 前景与背景的亮度差下限，低于此值文字会难以辨认 */
const MIN_CONTRAST = 100

test('每个整点锚点都精确命中色板定义', () => {
  for (const anchor of ANCHORS) {
    const theme = at(anchor.hour * 60)
    for (const field of INTERPOLATED) {
      assert.equal(theme[field], anchor[field], `${anchor.label} 的 ${field} 应为锚点原值`)
    }
    assert.equal(theme.glassAlpha, anchor.glassAlpha)
    assert.equal(theme.label, anchor.label)
  }
})

test('全天逐分钟连续，且跨日无断裂', () => {
  let worst = 0
  let worstAt = 0

  for (let minute = 0; minute < 1440; minute++) {
    const current = at(minute)
    const next = at((minute + 1) % 1440)
    for (const field of INTERPOLATED) {
      const a = toRgb(current[field])
      const b = toRgb(next[field])
      const delta = Math.max(...a.map((value, index) => Math.abs(value - b[index])))
      if (delta > worst) {
        worst = delta
        worstAt = minute
      }
    }
  }

  const where = `${String(Math.floor(worstAt / 60)).padStart(2, '0')}:${String(worstAt % 60).padStart(2, '0')}`
  assert.ok(worst <= MAX_STEP, `相邻分钟最大通道差 ${worst}（在 ${where}），超过上限 ${MAX_STEP}`)
  console.log(`  相邻分钟最大通道差 ${worst}（出现在 ${where}），上限 ${MAX_STEP}`)
})

test('凌晨明显比中午暗（夜间转深色生效）', () => {
  const midnight = luminance(at(0).bg1)
  const noon = luminance(at(12 * 60).bg1)
  assert.ok(
    noon - midnight > 120,
    `中午亮度 ${noon.toFixed(1)} 与凌晨 ${midnight.toFixed(1)} 差距不足，夜间没有明显转深`,
  )
  assert.ok(midnight < 110, `凌晨背景应偏深，实际亮度 ${midnight.toFixed(1)}`)
})

test('任何时刻的文字对比度都足够（文字色必须是离散两档）', () => {
  let worst = Infinity
  let worstAt = 0
  const allowed = new Set([TEXT_ON_LIGHT.fg, TEXT_ON_DARK.fg])

  for (let minute = 0; minute < 1440; minute++) {
    const theme = at(minute)
    assert.ok(allowed.has(theme.fg), `${minute} 分钟处的文字色不是两档之一：${theme.fg}`)

    const contrast = Math.abs(luminance(theme.fg) - luminance(theme.bg1))
    if (contrast < worst) {
      worst = contrast
      worstAt = minute
    }
  }

  const where = `${String(Math.floor(worstAt / 60)).padStart(2, '0')}:${String(worstAt % 60).padStart(2, '0')}`
  assert.ok(
    worst > MIN_CONTRAST,
    `最差时刻 ${where} 的前景/背景亮度差仅 ${worst.toFixed(1)}，低于下限 ${MIN_CONTRAST}`,
  )
  console.log(`  最差对比度出现在 ${where}，亮度差 ${worst.toFixed(1)}（下限 ${MIN_CONTRAST}）`)
})

test('文字色由背景亮度决定，且一天只在阈值上穿越两次', () => {
  let switches = 0
  let previous = at(0).fg

  for (let minute = 0; minute < 1440; minute++) {
    const theme = at(minute)
    const expected =
      luminance(theme.bg1) >= TEXT_SWITCH_LUMINANCE ? TEXT_ON_LIGHT.fg : TEXT_ON_DARK.fg
    assert.equal(theme.fg, expected, `${minute} 分钟处的文字色与背景亮度不匹配`)
    if (theme.fg !== previous) switches++
    previous = theme.fg
  }

  assert.ok(switches <= 2, `一天内文字色切换了 ${switches} 次，阈值附近可能在抖动`)
})

test('全天输出的都是合法色值与合法玻璃参数', () => {
  const labels = new Set(ANCHORS.map((anchor) => anchor.label))
  for (let minute = 0; minute < 1440; minute++) {
    const theme = at(minute)
    for (const field of INTERPOLATED) {
      assert.match(theme[field], /^#[0-9a-f]{6}$/i, `${field} 非法：${theme[field]}`)
    }
    assert.match(theme.fg, /^#[0-9a-f]{6}$/i, `fg 非法：${theme.fg}`)
    assert.match(theme.fgMuted, /^rgba\(\d+, \d+, \d+, [\d.]+\)$/, `fgMuted 非法：${theme.fgMuted}`)
    assert.ok(theme.glassAlpha >= 0.05 && theme.glassAlpha <= 0.3, `glassAlpha 越界：${theme.glassAlpha}`)
    assert.ok(labels.has(theme.label), `未知时段名：${theme.label}`)
  }
})
