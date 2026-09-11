/**
 * 运动数学：纯函数，不依赖 DOM 与框架，因此可以被校验脚本直接导入断言。
 *
 * 这组函数的思路借鉴自 RhineLabUI 的 motion.ts —— 那里把动效写成"带注释的物理模型"
 * 而不是一堆 tween 参数，好处是意图明确、可单独测试、与渲染实现解耦。
 */

export const clamp01 = (t: number) => (t < 0 ? 0 : t > 1 ? 1 : t)

/**
 * 五次平滑（smootherstep）。
 * 一阶与二阶导数在 t=0、t=1 处都为 0，所以两端都没有速度突变——这就是"慢出慢停"。
 * 与 expo.out（快出慢停、带冲劲）是相反的观感，更贴合水滴玻璃的沉稳。
 */
export function smootherstep(t: number): number {
  const x = clamp01(t)
  return x * x * x * (10 + x * (-15 + 6 * x))
}

/** 供 GSAP 直接使用的等价缓动函数 */
export const smootherstepEase = (progress: number) => smootherstep(progress)

/** 高斯钟形。用来构造连续的包络，而不是逐个错开的独立 tween。 */
export const bell = (x: number, width: number) => Math.exp(-0.5 * (x / width) ** 2)

/**
 * 波包：一个带肩膀的波峰，其后跟一段浅波谷。
 * 相邻元素读取同一条曲线，所以读起来是"一个连续表面被推动"。
 */
export const wavePacket = (distance: number) =>
  2.5 * bell(distance, 3.8) - 0.58 * bell(distance - 6, 3.5)

/** wavePacket 的峰值（在 distance=0 处），用于归一化 */
const WAVE_PEAK = wavePacket(0)

/** 把波包归一化到 [0,1]，可直接当作权重使用 */
export const waveWeight = (distance: number) => clamp01(wavePacket(distance) / WAVE_PEAK)

/**
 * 波包式入场分配。
 * 延迟随元素到波源（左上）的距离增长，幅度按波包曲线轻微衰减，
 * 于是整片元素读起来是"一道波斜着推过去"，而不是"每个各自弹一下"。
 *
 * 行方向乘 1.6：屏幕横向比纵向宽，让波前保持接近对角。
 */
export function waveAssign(index: number, columns: number) {
  const cols = Math.max(1, columns)
  const row = Math.floor(index / cols)
  const column = index % cols
  const distance = Math.sqrt(column * column + row * row * 1.6)
  return {
    distance,
    delay: distance * 0.055,
    // 幅度只做轻微整形；差异过大会显得杂乱
    amplitude: 0.45 + 0.55 * waveWeight(distance - 1),
  }
}

/**
 * 空闲呼吸：两个互质周期（8s 与 13s）叠加。
 * 互质 → 长时间不重复，看不出"在循环"；峰值已归一化，实际幅度由调用方决定。
 */
export function idleBreath(time: number, phase = 0): number {
  return (
    0.735 * Math.sin((time * Math.PI * 2) / 8 + phase) +
    0.265 * Math.sin((time * Math.PI * 2) / 13 - phase * 0.6)
  )
}

/**
 * 临界阻尼弹簧的一步积分（半隐式欧拉）。
 *
 *   a = -2ω·v - ω²·(x - target)
 *
 * 临界阻尼下不会超调，而且**保留速度**——这正是"连续输入从当前运动接续"的实现基础：
 * 新目标只是换个 target，位置与速度都从当前状态继续，不必等上一次动画播完。
 */
export function springStep(
  x: number,
  v: number,
  target: number,
  omega: number,
  dt: number,
): [position: number, velocity: number] {
  const acceleration = -2 * omega * v - omega * omega * (x - target)
  const nextVelocity = v + acceleration * dt
  const nextPosition = x + nextVelocity * dt
  return [nextPosition, nextVelocity]
}

/** 临界阻尼的稳定时间约为 4/ω 秒，据此反推角频率 */
export const omegaForSettle = (seconds: number) => 4 / seconds
