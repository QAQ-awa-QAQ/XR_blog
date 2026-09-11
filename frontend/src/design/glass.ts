/**
 * iOS 27 液态玻璃（Liquid Glass）的折射参数。
 *
 * 液态玻璃跟普通毛玻璃（glassmorphism）的区别在于它**不只是模糊背景**：
 *   1. **折射** —— 背景透过玻璃时被"厚薄不均"的玻璃挤歪，这是"液态"的来源；
 *   2. **光晕** —— 外缘一圈柔和的辉光，取自当前主题色；
 *   3. **厚度** —— 顶部内高光 + 底部内反光 + 外侧描边，读起来像一块有厚度的实体。
 *
 * 第 1 条在 Web 上只能靠 SVG 滤镜做到：`feTurbulence` 生成一张低频噪声当位移图，
 * 再交给 `feDisplacementMap` 去挤 `SourceGraphic`。在 `backdrop-filter` 里
 * `SourceGraphic` 就是**元素背后的画面**，所以这是真正的"对背景的形变"，
 * 不是叠一层假纹理。滤镜定义见 `components/LiquidGlassDefs.tsx`。
 *
 * 第 2、3 条是纯 CSS，口径记在 README.md 里。
 *
 * ⚠️ 性能取舍：位移滤镜要按元素面积重算，而本站背景的光斑是逐帧在动的。
 * 所以只把它挂在**面积小、又是视觉焦点**的侧栏上（面板 + 选中胶囊），
 * 卡片仍然只用纯模糊 —— 那几张卡片加起来几千平方像素，加上去会掉帧。
 */

/** 滤镜 id。CSS 里用 `backdrop-filter: url(#...)` 引用 */
export const LIQUID_FILTER_ID = 'liquid-glass'

export const LIQUID_REFRACTION = {
  /** 噪声频率。要**低**：频率一高就变成"磨砂颗粒"，而不是玻璃的厚度起伏 */
  baseFrequency: '0.012 0.022',
  /** 两层足够。再多只涨开销，看不出差别 */
  numOctaves: 2,
  /** 固定 seed，否则每次刷新折射的花纹都不一样 */
  seed: 11,
  /** 先把噪声抹平：位移场不连续的话，背景会被切成一格一格，而不是被"挤" */
  smoothing: 3,
  /** 最大位移 = scale / 2（px）。10 对 52px 宽的侧栏，是「看得出在挤、但还读得出字」的量级 */
  scale: 10,
  /** 滤镜区域外扩：形变会把边缘外的背景拉进来，不外扩就会在边界切出一条硬边 */
  region: { x: '-30%', y: '-30%', width: '160%', height: '160%' },
} as const
