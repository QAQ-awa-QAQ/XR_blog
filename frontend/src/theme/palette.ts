/**
 * 8 时段锚点色板。
 *
 * 设计意图（design.md 2.1）：浅色系玻璃为基底，主色调随本地时间连续过渡；
 * 22:00 之后与凌晨时段允许转深色，形成"入夜—黎明"的自然循环。
 *
 * 关键约束：**文字色不参与插值**。
 * 原因（由 scripts/check-theme.mjs 抓出）：背景从浅到深、文字从深到浅，
 * 两者同时连续插值必然在中途亮度相等，那一瞬间对比度趋近于 0，文字实际不可读。
 * 这不是阈值问题而是模型问题，所以文字色改为按背景亮度**离散切换**两档，
 * 并把阈值放在亮度中点 —— 这样最差时刻的对比度仍有半个亮度区间。
 */

export type Anchor = {
  hour: number
  label: string
  bg1: string
  bg2: string
  orbA: string
  orbB: string
  accent: string
  /** 玻璃层的白色薄膜强度，深色时段需要相应降低 */
  glassAlpha: number
}

export const ANCHORS: Anchor[] = [
  {
    hour: 0,
    label: '凌晨',
    bg1: '#1A1B2A', bg2: '#12131F',
    orbA: '#6D4AE0', orbB: '#1E3A8A',
    accent: '#A78BFA',
    glassAlpha: 0.1,
  },
  {
    hour: 5,
    label: '清晨',
    bg1: '#FDF3F4', bg2: '#F2E9F6',
    orbA: '#FBCFE8', orbB: '#FDBA74',
    accent: '#DB2777',
    glassAlpha: 0.22,
  },
  {
    hour: 8,
    label: '上午',
    bg1: '#F2F8FD', bg2: '#E7F1FA',
    orbA: '#BAE6FD', orbB: '#A7F3D0',
    accent: '#0284C7',
    glassAlpha: 0.22,
  },
  {
    hour: 11,
    label: '中午',
    bg1: '#FFFDF5', bg2: '#FFF6E5',
    orbA: '#FDE68A', orbB: '#BAE6FD',
    accent: '#B45309',
    glassAlpha: 0.24,
  },
  {
    hour: 13,
    label: '下午',
    bg1: '#F3FBF8', bg2: '#E8F5F1',
    orbA: '#A7F3D0', orbB: '#99F6E4',
    accent: '#047857',
    glassAlpha: 0.22,
  },
  {
    hour: 17,
    label: '傍晚',
    bg1: '#FFF6F0', bg2: '#FDEBE2',
    orbA: '#FDBA74', orbB: '#F9A8D4',
    accent: '#C2410C',
    glassAlpha: 0.22,
  },
  {
    hour: 19,
    label: '晚上',
    bg1: '#F1F3FB', bg2: '#E6E9F7',
    orbA: '#A5B4FC', orbB: '#C4B5FD',
    accent: '#4338CA',
    glassAlpha: 0.22,
  },
  {
    hour: 22,
    label: '午夜',
    bg1: '#232439', bg2: '#16172A',
    orbA: '#7C3AED', orbB: '#312E81',
    accent: '#C4B5FD',
    glassAlpha: 0.12,
  },
]

/** 文字色的两个档位，按背景亮度离散选用 */
export const TEXT_ON_LIGHT = { fg: '#0B1220', fgMuted: 'rgba(11, 18, 32, 0.62)' }
export const TEXT_ON_DARK = { fg: '#F5F3FF', fgMuted: 'rgba(245, 243, 255, 0.66)' }

/**
 * 亮度阈值取中点：这样最差时刻（背景正好落在阈值上）的对比度
 * 仍有半个亮度区间，实测约 110，远高于可用下限。
 */
export const TEXT_SWITCH_LUMINANCE = 128

export type ThemeTokens = Omit<Anchor, 'hour'> & {
  fg: string
  fgMuted: string
  /** 背景是否处于深色档（与文字色同一门槛）。供"随明暗反向"的部件消费（如光标光晕） */
  isDark: boolean
}

type RGB = [number, number, number]

function hexToRgb(hex: string): RGB {
  const v = hex.replace('#', '')
  return [
    parseInt(v.slice(0, 2), 16),
    parseInt(v.slice(2, 4), 16),
    parseInt(v.slice(4, 6), 16),
  ]
}

function rgbToHex([r, g, b]: RGB): string {
  // 统一大写：锚点色板是大写书写，插值结果必须能与之逐字符比对
  const to = (n: number) =>
    Math.round(Math.min(255, Math.max(0, n)))
      .toString(16)
      .padStart(2, '0')
      .toUpperCase()
  return `#${to(r)}${to(g)}${to(b)}`
}

/** 感知亮度，用于判断该用深色还是浅色文字 */
export function luminance(hex: string): number {
  const [r, g, b] = hexToRgb(hex)
  return 0.299 * r + 0.587 * g + 0.114 * b
}

/** 在两个色值之间按 t∈[0,1] 线性插值 */
export function mixHex(from: string, to: string, t: number): string {
  const a = hexToRgb(from)
  const b = hexToRgb(to)
  return rgbToHex([
    a[0] + (b[0] - a[0]) * t,
    a[1] + (b[1] - a[1]) * t,
    a[2] + (b[2] - a[2]) * t,
  ])
}

/** 取某一时刻应使用的主题。24 小时首尾相连，凌晨与午夜之间同样平滑过渡。 */
export function themeAt(date: Date): ThemeTokens {
  const hours = date.getHours() + date.getMinutes() / 60

  let index = 0
  for (let k = 0; k < ANCHORS.length; k++) {
    if (ANCHORS[k].hour <= hours) index = k
  }

  const from = ANCHORS[index]
  const to = ANCHORS[(index + 1) % ANCHORS.length]
  const span = (to.hour - from.hour + 24) % 24 || 24
  const t = ((hours - from.hour + 24) % 24) / span

  const bg1 = mixHex(from.bg1, to.bg1, t)
  // 亮暗档位与文字色**同门槛、同判断**：要"随明暗反向"的部件（光标光晕）消费 isDark，
  // 不要在别处另立一套阈值
  const isDark = luminance(bg1) < TEXT_SWITCH_LUMINANCE
  const text = isDark ? TEXT_ON_DARK : TEXT_ON_LIGHT

  return {
    // 时段名跟随最接近的锚点，避免插值中途出现"半上午半中午"
    label: t < 0.5 ? from.label : to.label,
    bg1,
    bg2: mixHex(from.bg2, to.bg2, t),
    orbA: mixHex(from.orbA, to.orbA, t),
    orbB: mixHex(from.orbB, to.orbB, t),
    accent: mixHex(from.accent, to.accent, t),
    glassAlpha: from.glassAlpha + (to.glassAlpha - from.glassAlpha) * t,
    isDark,
    ...text,
  }
}
