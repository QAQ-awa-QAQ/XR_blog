import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  smootherstep,
  waveAssign,
  waveWeight,
  idleBreath,
  springStep,
  omegaForSettle,
} from '../src/motion/math.ts'
import { orbState, orbAmplitudeBounds } from '../src/motion/orbMotion.ts'
import { durations, omega } from '../src/motion/tokens.ts'

/**
 * 运动数学校验。
 *
 * 这些动效参数是"手感"的来源，一旦被无意改动很难靠肉眼发现，
 * 所以把可机械判定的性质（不超调、幅度有界、两端无速度突变、周期互质）都锁进断言。
 */

const sample = (from, to, step) => {
  const out = []
  for (let t = from; t <= to; t += step) out.push(t)
  return out
}

test('smootherstep：端点精确、单调不减、两端速度为零', () => {
  assert.equal(smootherstep(0), 0)
  assert.equal(smootherstep(1), 1)
  assert.equal(smootherstep(-5), 0, '小于 0 应被夹住')
  assert.equal(smootherstep(9), 1, '大于 1 应被夹住')

  const points = sample(0, 1, 0.001)
  let previous = -Infinity
  for (const t of points) {
    const value = smootherstep(t)
    assert.ok(value >= previous - 1e-12, `在 t=${t} 处不单调`)
    previous = value
  }

  // 两端导数趋近 0 —— 这正是"慢出慢停、无速度突变"的依据
  const h = 1e-4
  const startSlope = (smootherstep(h) - smootherstep(0)) / h
  const endSlope = (smootherstep(1) - smootherstep(1 - h)) / h
  assert.ok(startSlope < 1e-2, `起点速度应接近 0，实际 ${startSlope}`)
  assert.ok(endSlope < 1e-2, `终点速度应接近 0，实际 ${endSlope}`)
})

test('临界阻尼弹簧：不超调、按时收敛、末速归零', () => {
  const dt = 1 / 120
  let x = 0
  let v = 0
  let peak = 0

  for (let step = 0; step < 120 * 2; step++) {
    ;[x, v] = springStep(x, v, 1, omega.page, dt)
    peak = Math.max(peak, x)
  }

  assert.ok(Math.abs(x - 1) < 1e-3, `2 秒后应到达目标，实际 ${x}`)
  assert.ok(peak <= 1 + 1e-6, `临界阻尼不应超调，峰值 ${peak}`)
  assert.ok(Math.abs(v) < 1e-3, `末速应归零，实际 ${v}`)
})

test('临界阻尼弹簧：中途换目标时从当前状态接续（不重置速度）', () => {
  const dt = 1 / 120
  let x = 0
  let v = 0

  // 先朝 1 走 0.15 秒，此时已有速度
  for (let step = 0; step < 18; step++) ;[x, v] = springStep(x, v, 1, omega.page, dt)
  assert.ok(v > 0.1, `此时应处于运动中，实际速度 ${v}`)

  // 反向打断：位置必须连续，速度方向不应被瞬间归零
  const before = x
  const [next, nextV] = springStep(x, v, 0, omega.page, dt)
  assert.ok(Math.abs(next - before) < 0.02, '打断处位置必须连续，不能瞬移')
  assert.ok(nextV > 0, '惯性应保留一帧，而不是立刻反向')
})

test('波包权重：峰值在原点、远处衰减到 0 附近', () => {
  assert.ok(Math.abs(waveWeight(0) - 1) < 1e-9, '峰值应归一化为 1')
  assert.ok(waveWeight(-40) < 0.02 && waveWeight(40) < 0.02, '远处应衰减')
})

test('波包分配：延迟由距离唯一决定、幅度有界', () => {
  const columns = 3
  for (let index = 0; index < 12; index++) {
    const { delay, amplitude, distance } = waveAssign(index, columns)
    assert.ok(Math.abs(delay - distance * 0.055) < 1e-12, '延迟必须由距离唯一决定')
    assert.ok(amplitude >= 0.45 && amplitude <= 1, `幅度越界：${amplitude}`)
    assert.ok(distance >= 0, '距离不能为负')
  }

  // 同一行内沿横向展开，延迟严格递增
  assert.ok(waveAssign(1, columns).delay > waveAssign(0, columns).delay)
  assert.ok(waveAssign(2, columns).delay > waveAssign(1, columns).delay)

  // 二维波前：换行后从左侧重新开始，所以第 4 个元素比上一行末尾更接近波源。
  // 这不是错误——行首行尾本就不该依次加长，否则会变成"逐条延迟"而不是波。
  assert.ok(
    waveAssign(3, columns).delay < waveAssign(2, columns).delay,
    '换行后距离应回到左侧起点',
  )
})

test('空闲呼吸：幅度有界，且不存在 8s/13s/26s/40s 的重复周期', () => {
  const times = sample(0, 60, 0.1)
  let peak = 0
  for (const t of times) peak = Math.max(peak, Math.abs(idleBreath(t)))
  assert.ok(peak <= 1.02, `峰值应归一化到 1 附近，实际 ${peak}`)

  // 两个互质周期叠加 → 只有最小公倍数（104s）才是真正的周期
  for (const period of [8, 13, 26, 40]) {
    let maxDiff = 0
    for (const t of times) {
      maxDiff = Math.max(maxDiff, Math.abs(idleBreath(t) - idleBreath(t + period)))
    }
    assert.ok(maxDiff > 0.05, `不应以 ${period}s 为周期重复（实测最大差 ${maxDiff}）`)
  }
})

test('光斑运动：数值有限、位移不越界、缩放不会翻面', () => {
  const unit = 10 // 假设 1vmin = 10px
  for (const variant of ['drift', 'pulse', 'wave']) {
    const bounds = orbAmplitudeBounds[variant]
    for (const t of sample(0, 600, 0.25)) {
      for (const layer of [0, 1, 2]) {
        const state = orbState(variant, t, layer, unit)
        assert.ok(Number.isFinite(state.x) && Number.isFinite(state.y) && Number.isFinite(state.scale))
        assert.ok(Math.abs(state.x) <= bounds.x * unit + 1e-6, `${variant} 横向越界：${state.x}`)
        assert.ok(Math.abs(state.y) <= bounds.y * unit + 1e-6, `${variant} 纵向越界：${state.y}`)
        assert.ok(state.scale >= orbAmplitudeBounds.minScale, `${variant} 缩放过小：${state.scale}`)
      }
    }
  }
})

test('时间分档：数值合法且与 ω 换算一致', () => {
  for (const [key, value] of Object.entries(durations)) {
    assert.ok(value > 0 && value < 5, `${key} 时长不合理：${value}`)
  }
  assert.ok(Math.abs(omega.page - omegaForSettle(durations.pageSettle)) < 1e-9)
  // 顺手锁定整屏切换的刚度：防止无意间改动「翻页手感」。
  // 0.5s → ω = 8（原为 0.67s → 6，按实测手感调快后同步更新）
  assert.ok(omega.page > 7.5 && omega.page < 8.5, `ω 应在 8 附近，实际 ${omega.page}`)
})
