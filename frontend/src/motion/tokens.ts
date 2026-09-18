import { omegaForSettle, smootherstepEase } from './math.ts'

/**
 * 时间分档。
 *
 * 原则（借自 RhineLabUI 的 DESIGN.md）：**进入慢、退出快**。
 * 理由：退出时用户已经知道结果，等待时间纯属损耗；进入时则要给画面一点时间成形。
 *
 * 另一条贯穿的规则：所有过渡都必须能被**从当前位置接续**打断，而不是从头重播。
 */
export const durations = {
  /** 整屏切换：临界阻尼弹簧的稳定时间（ω = 4/该值 ≈ 8 rad/s）。
      注意这个 4/ω 只是「大体到位」的口径（约剩 9%），真正视觉停稳还要再久；
      所以 MainShell 里另配了亚像素吸附，消掉指数收尾的拖尾 */
  pageSettle: 0.5,

  /** 登录 / 注册面板的横向平移（卡片内约一屏宽的位移，见 Auth.tsx）。
      位移量只有整屏切换的三分之一左右，跟整屏同速会显得拖（实测反馈「太慢」），
      收紧一档：0.5 → 0.32（ω ≈ 12.5） */
  authSlide: 0.32,

  /** 卡片入场：波包式错开 + 弹性收尾，所以给得比别的长 */
  cardReveal: 0.8,

  /** 侧栏指示条滑到下一项；用 smootherstep 曲线，不是颜色淡入淡出 */
  navSlide: 0.42,

  /** 「更多」按钮：三点连成横线 → 旋转 90° → 展出箭头 ↑。
      该形变已归档到 `src/design/morph.ts`，当前没有调用方，时长留着配套 */
  moreMorph: 0.3,
  /** 旧版侧栏页脚把菜单拉出的时长。侧栏改成四页导航后已没有调用方 */
  moreReveal: 0.42,

  /** 面板类：进入慢、退出快 */
  panelEnter: 0.3,
  panelExit: 0.2,

  /** 欢迎页时间轴：幕布退场 → logo 浮现 → 三元素分列 → 按钮内箭头绘出 */
  welcomeCurtain: 0.9,
  welcomeBeam: 1,
  welcomeShift: 0.85,
  welcomeDraw: 0.34,
  welcomeArrow: 0.26,

  /** 登录 → 主页的过场（Handoff）：欢迎页左移出 / 幽灵飞行变形 / 箭头旋转发射 / 按钮弹出 */
  handoffExit: 0.6,
  handoffFly: 0.62,
  handoffSpin: 0.28,
  handoffLaunch: 0.34,
  handoffButtons: 0.3,
  handoffStagger: 0.09,

  /** 「欢迎页 → 登录页」的过场（AuthIntro）：
      文案淡出 / XR 放大居中 / 箭头左移 / 副箭头影子浮现与显影 / 卡片淡入 / 右移带按钮 */
  authIntroFade: 0.4,
  authIntroBeam: 0.45,
  authIntroLeft: 0.4,
  authIntroShadow: 0.3,
  authIntroCard: 0.5,
  authIntroCarry: 0.6,

  /** 覆盖层：进入 / 退出 */
  overlayEnter: 0.32,
  overlayExit: 0.22,
} as const

/** 临界阻尼角频率，供 springStep 使用 */
export const omega = {
  page: omegaForSettle(durations.pageSettle),
  /** 登录页面板平移（Auth.tsx 的 progress 弹簧）；高度弹簧仍用 page */
  authSlide: omegaForSettle(durations.authSlide),
} as const

/** 「转圈 → 箭头」交接播完再启动过场的等待（ms）。
    欢迎页与登录页共用：按钮都要先把 spinner 交接回箭头，幽灵才克隆得到「→」 */
export const HANDOFF_LEAD_MS = 300

export const easings = {
  /** 慢出慢停、无速度突变——玻璃／水滴质感首选。
      但它的**两端速度归零**：前 25% 时间只走完约 1.5% 的位移，
      所以只适合整屏、长距离的慢镜头过渡，不要拿它做按钮/菜单的短程位移 */
  smooth: smootherstepEase,
  /** 快出慢停、带冲劲——用于"弹出来"的元素 */
  out: 'expo.out',
  /** 柔和收尾——起步不迟滞、尾段减速看得见过程，适合需要「看得出缓动」的短程位移 */
  soft: 'power2.out',
  /** 点下去立刻就走：起步干脆、尾部稳稳停住，没有 smootherstep 的顿挫。
      侧栏指示条滑动、菜单拉出这类**短程响应式位移**用它 */
  snappy: 'power3.out',
} as const
