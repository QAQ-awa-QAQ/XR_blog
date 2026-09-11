/**
 * 各页面的背景光斑运动。
 *
 * design.md 2.5 要求"背景随页面不同有不同动画"，所以这里按页面给出几套参数；
 * 但都共用同一组**互质周期**基元，因此都不会被看出"在循环重复"。
 * （「更多」页直接复用 drift —— 那一页全是账户信息，背景不该抢戏。）
 *
 * 纯函数：只吃时间与视口单位，便于校验脚本断言幅度与稳定性。
 */

export type OrbVariant = 'drift' | 'pulse' | 'wave'

export type OrbState = {
  x: number
  y: number
  scale: number
}

const TAU = Math.PI * 2

/** 相位错开，让同一页的三层光斑不同步 */
const phaseOf = (layer: number) => layer * 1.7

const wave = (time: number, period: number, phase: number) =>
  Math.sin((time * TAU) / period + phase)

/**
 * unit 建议传 1vmin 的像素值，这样位移幅度随视口等比缩放。
 */
export function orbState(
  variant: OrbVariant,
  time: number,
  layer: number,
  unit: number,
): OrbState {
  const phase = phaseOf(layer)

  switch (variant) {
    // 简介页：缓慢双层漂移，像水面下的光
    case 'drift':
      return {
        x: unit * (0.22 * wave(time, 23, phase) + 0.08 * wave(time, 31, -phase)),
        y: unit * (0.16 * wave(time, 29, phase * 1.3) + 0.06 * wave(time, 17, phase)),
        scale: 1 + 0.06 * wave(time, 37, phase),
      }

    // 功能页：呼吸式缩放，暗示"模块在待命"
    case 'pulse':
      return {
        x: unit * 0.07 * wave(time, 19, phase),
        y: unit * 0.09 * wave(time, 27, -phase),
        scale:
          1 + 0.13 * wave(time, 8, phase) + 0.04 * wave(time, 13, phase * 0.7),
      }

    // 联系页：横向流动，像信号波纹
    case 'wave':
      return {
        x: unit * (0.3 * wave(time, 20, phase) + 0.07 * wave(time, 9, phase)),
        y: unit * (0.08 * wave(time, 26, -phase) + 0.03 * wave(time, 41, phase)),
        scale:
          1 + 0.05 * wave(time, 33, phase) - 0.04 * wave(time, 12, phase),
      }
  }
}

/** 各变体的位移上限（以 unit 为单位），校验脚本据此断言不越界 */
export const orbAmplitudeBounds = {
  drift: { x: 0.3, y: 0.22 },
  pulse: { x: 0.07, y: 0.09 },
  wave: { x: 0.37, y: 0.11 },
  /** 缩放下限，必须保持正数否则光斑会翻面 */
  minScale: 0.7,
} as const
