/**
 * 8 时段锚点色板。
 * 设计意图（design.md 2.1）：浅色系玻璃为基底，主色调随本地时间连续过渡；
 * 22:00 之后与凌晨时段允许转深色，形成"入夜—黎明"的自然循环。
 * 通过线性插值保证任何时刻都不出现跳变。
 */
export type Anchor = {
  hour: number
  label: string
  bg1: string
  bg2: string
  orbA: string
  orbB: string
  accent: string
  fg: string
  fgMuted: string
  /** 玻璃层的白色薄膜强度，深色时段需要相应降低 */
  glassAlpha: number
}

export const ANCHORS: Anchor[] = [
  {
    hour: 0,
    label: '凌晨',
    bg1: '#1A1B2A', bg2: '#12131F',
    orbA: '#6D4AE0', orbB: '#1E3A8A',
    accent: '#A78BFA', fg: '#F4F2FF', fgMuted: 'rgba(244,242,255,0.62)',
    glassAlpha: 0.1,
  },
  {
    hour: 5,
    label: '清晨',
    bg1: '#FDF3F4', bg2: '#F2E9F6',
    orbA: '#FBCFE8', orbB: '#FDBA74',
    accent: '#DB2777', fg: '#17141A', fgMuted: 'rgba(23,20,26,0.62)',
    glassAlpha: 0.22,
  },
  {
    hour: 8,
    label: '上午',
    bg1: '#F2F8FD', bg2: '#E7F1FA',
    orbA: '#BAE6FD', orbB: '#A7F3D0',
    accent: '#0284C7', fg: '#0B1220', fgMuted: 'rgba(11,18,32,0.62)',
    glassAlpha: 0.22,
  },
  {
    hour: 11,
    label: '中午',
    bg1: '#FFFDF5', bg2: '#FFF6E5',
    orbA: '#FDE68A', orbB: '#BAE6FD',
    accent: '#B45309', fg: '#14120B', fgMuted: 'rgba(20,18,11,0.62)',
    glassAlpha: 0.24,
  },
  {
    hour: 13,
    label: '下午',
    bg1: '#F3FBF8', bg2: '#E8F5F1',
    orbA: '#A7F3D0', orbB: '#99F6E4',
    accent: '#047857', fg: '#0A1512', fgMuted: 'rgba(10,21,18,0.62)',
    glassAlpha: 0.22,
  },
  {
    hour: 17,
    label: '傍晚',
    bg1: '#FFF6F0', bg2: '#FDEBE2',
    orbA: '#FDBA74', orbB: '#F9A8D4',
    accent: '#C2410C', fg: '#1A120B', fgMuted: 'rgba(26,18,11,0.62)',
    glassAlpha: 0.22,
  },
  {
    hour: 19,
    label: '晚上',
    bg1: '#F1F3FB', bg2: '#E6E9F7',
    orbA: '#A5B4FC', orbB: '#C4B5FD',
    accent: '#4338CA', fg: '#0D0F1A', fgMuted: 'rgba(13,15,26,0.62)',
    glassAlpha: 0.22,
  },
  {
    hour: 22,
    label: '午夜',
    bg1: '#232439', bg2: '#16172A',
    orbA: '#7C3AED', orbB: '#312E81',
    accent: '#C4B5FD', fg: '#F5F3FF', fgMuted: 'rgba(245,243,255,0.66)',
    glassAlpha: 0.12,
  },
]

export type ThemeTokens = Omit<Anchor, 'hour'>

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
  const to = (n: number) => Math.round(Math.min(255, Math.max(0, n))).toString(16).padStart(2, '0')
  return `#${to(r)}${to(g)}${to(b)}`
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

  let i = 0
  for (let k = 0; k < ANCHORS.length; k++) {
    if (ANCHORS[k].hour <= hours) i = k
  }

  const from = ANCHORS[i]
  const to = ANCHORS[(i + 1) % ANCHORS.length]
  const span = (to.hour - from.hour + 24) % 24 || 24
  const t = ((hours - from.hour + 24) % 24) / span

  return {
    // 时段名跟随最接近的锚点，避免插值中途出现"半上午半中午"
    label: t < 0.5 ? from.label : to.label,
    bg1: mixHex(from.bg1, to.bg1, t),
    bg2: mixHex(from.bg2, to.bg2, t),
    orbA: mixHex(from.orbA, to.orbA, t),
    orbB: mixHex(from.orbB, to.orbB, t),
    accent: mixHex(from.accent, to.accent, t),
    fg: mixHex(from.fg, to.fg, t),
    fgMuted: mixHex(from.fgMuted, to.fgMuted, t),
    glassAlpha: from.glassAlpha + (to.glassAlpha - from.glassAlpha) * t,
  }
}
