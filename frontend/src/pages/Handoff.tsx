import { useLayoutEffect, useRef } from 'react'
import gsap from 'gsap'
import { durations, easings } from '../motion/tokens'

/** 过场来源：欢迎页箭头 / 登录页的三个提交按钮（对应按钮的 data-cta 值） */
export type HandoffFrom = 'welcome' | 'login-submit' | 'register-submit' | 'login-guest'

/**
 * 进入主页的过场编排（不渲染任何 UI，由 App 在 handoff 期间挂载）。
 *
 * 来源有两处，流程完全同构，只有「出场页」与「被克隆的按钮」不同：
 *   · welcome —— 欢迎页的箭头按钮
 *   · login-submit / register-submit / login-guest —— 登录页的三个提交按钮
 *
 * 序列（转圈交接回箭头、完整展出之后）：
 *   1. 出场页内容整体**左移出屏**（光斑同时淡出）；
 *   2. 按钮的「幽灵」（克隆体）从原位飞向侧栏「首页」按钮的位置，
 *      同时变形：壳收缩到按钮规格、箭头等比缩小；
 *   3. 落位后箭头**旋转**朝向下一个按钮的排列方向（桌面 ↓ / 手机 →）；
 *   4. 箭头沿该方向**发射**飞出并淡出 —— 期间侧栏按钮依次弹出（带回一次光晕脉冲），
 *      指示条与首页按钮同时现身；
 *   5. 完成后回调 onDone，App 撤掉出场页。
 *
 * 为什么用克隆体：飞行体要脱离原布局（fixed + 独立尺寸动画），
 * 而 React 不欢迎外部 DOM 移动它管理的节点。克隆是"只退不进"的一次性演出件，
 * 结束时移除即可。
 *
 * ⚠️ 只有「指示条」用 style.visibility 手工藏 —— 它的 opacity 由 Sidebar 的
 * placePill 管理（那个 effect 晚于这里的 useLayoutEffect 执行），
 * 用 GSAP 设 opacity 会被它随后覆盖。按钮们没有这个顾虑，直接用 autoAlpha。
 * ⚠️ 登录页按钮的文案（.auth__cta-label）不参与飞行，克隆后立刻删除：
 *    394px 宽的胶囊要收成 68px 的侧栏按钮，文字折行会穿帮。
 */
export function Handoff({ from, onDone }: { from: HandoffFrom; onDone: () => void }) {
  const firedRef = useRef(false)
  // 只挂载时演出一次：onDone 经 ref 取用，避免 App 重渲染（主题每分钟 tick）
  // 传入新函数导致 effect 重跑、过场重演
  const onDoneRef = useRef(onDone)
  onDoneRef.current = onDone

  useLayoutEffect(() => {
    const finish = () => {
      if (firedRef.current) return
      firedRef.current = true
      onDoneRef.current()
    }

    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    const btns = gsap.utils.toArray<HTMLElement>('.sidebar__btn')
    const pill = document.querySelector<HTMLElement>('.sidebar__pill')
    // 出场页三件套：整页层（要被提出文档流）、要左移出的内容、要淡出的光斑
    const layers =
      from === 'welcome'
        ? { stage: '.welcome', inner: '.welcome__inner', orbs: '.welcome .orb-field' }
        : { stage: '.auth', inner: '.auth__card', orbs: '.auth .orb-field' }
    const source = document.querySelector<HTMLElement>(
      from === 'welcome' ? '.welcome__cta' : `[data-cta="${from}"]`,
    )
    const inner = document.querySelector<HTMLElement>(layers.inner)
    const exitStage = document.querySelector<HTMLElement>(layers.stage)
    const welcomeOrbs = document.querySelector<HTMLElement>(layers.orbs)

    // 结构不对（或用户要求减弱动效）：不演出，直接收尾
    if (reduce || !source || !inner || btns.length === 0) {
      finish()
      return () => {}
    }

    const home = btns[0]
    const rest = btns.slice(1)
    const isDesktop = window.matchMedia('(min-width: 901px)').matches
    // 假边栏端点的「虚拟光标」光点（1px 硬高光 + 弥散光晕）—— 过场里它们随箭头从上向下划入
    const edgeLights = ['.sidebar__edge-heads', '.sidebar__edge-heads-glow']
      .map((sel) => document.querySelector<HTMLElement>(sel))
      .filter((el): el is HTMLElement => el !== null)

    // ⚠️ 必须先把出场页从文档流里「取出来」再做任何测量：
    // 它和主页面都是 height:100% 的块，同时在场时会**上下堆叠**（主页面被推到
    // 下一屏），幽灵的测量目标会整体偏移一屏 —— 表现是"箭头往左下飞走"。
    // 设为 absolute + inset 0 后两层重叠在同一屏，视觉不变。
    if (exitStage) {
      gsap.set(exitStage, {
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        zIndex: 5, // 出场页要「划过」主页面之上
        pointerEvents: 'none', // 演出期间不接受点击
      })
    }

    // ---- 测量（此刻欢迎页已脱离文档流、主页面在第一屏） ----
    const srcRect = source.getBoundingClientRect()
    const dstRect = home.getBoundingClientRect()
    const dstSvg = home.querySelector('svg')
    const dstIconSize = dstSvg ? dstSvg.getBoundingClientRect().width : 24

    // ---- 准备：造幽灵、隐藏原件 ----（都在首帧绘制前完成，不会闪）
    const ghost = source.cloneNode(true) as HTMLElement
    ghost.classList.add('handoff-ghost')
    // 登录页按钮的文字不参与飞行（见头部注释）
    ghost.querySelector('.auth__cta-label')?.remove()
    const ghostSvg = ghost.querySelector('svg')
    document.body.appendChild(ghost)
    gsap.set(ghost, {
      x: srcRect.left,
      y: srcRect.top,
      width: srcRect.width,
      height: srcRect.height,
      transformOrigin: '50% 50%',
    })
    gsap.set(source, { autoAlpha: 0 })
    gsap.set(btns, { autoAlpha: 0 })
    if (pill) pill.style.visibility = 'hidden'
    // 光点先藏起来：等发射阶段从各自位置的上方向下「划入」
    if (edgeLights.length) gsap.set(edgeLights, { autoAlpha: 0, y: -36 })

    // 正文整体：待发射阶段沿「垂直于边栏」的方向弹出 ——
    // 桌面边栏是左竖条 → 正文从左侧横向滑出；手机边栏是底横条 → 正文从下方纵向滑出
    const viewport = document.querySelector<HTMLElement>('.viewport')
    if (viewport) {
      gsap.set(viewport, isDesktop ? { x: -100, autoAlpha: 0 } : { y: 100, autoAlpha: 0 })
    }

    const accent =
      getComputedStyle(document.documentElement).getPropertyValue('--accent').trim() || '#0284c7'

    const flyAt = 0.12
    const flyEnd = flyAt + durations.handoffFly
    const launchAt = flyEnd + 0.06

    const tl = gsap.timeline({ onComplete: finish })

    // 1) 出场页内容整体左移出屏（按屏宽给足距离，不被元素自身宽度卡住）
    tl.to(
      inner,
      {
        x: () => -window.innerWidth * 0.85,
        duration: durations.handoffExit,
        ease: easings.snappy,
      },
      0,
    )
    if (welcomeOrbs) {
      tl.to(welcomeOrbs, { autoAlpha: 0, duration: durations.handoffExit * 0.8, ease: 'power1.out' }, 0)
    }

    // 2) 幽灵飞行 + 变形（壳 → 侧栏按钮尺寸；箭头 → 图标尺寸）
    tl.to(
      ghost,
      {
        x: dstRect.left,
        y: dstRect.top,
        width: dstRect.width,
        height: dstRect.height,
        duration: durations.handoffFly,
        ease: easings.snappy,
      },
      flyAt,
    )
    if (ghostSvg) {
      tl.to(
        ghostSvg,
        { width: dstIconSize, height: dstIconSize, duration: durations.handoffFly, ease: easings.snappy },
        flyAt,
      )
      // 3) 落位前收尾的那段：箭头转向"下一个按钮"的方向
      if (isDesktop) {
        tl.to(
          ghostSvg,
          { rotation: 90, duration: durations.handoffSpin, ease: easings.soft },
          flyEnd - durations.handoffSpin * 0.55,
        )
      }
    }

    // 4) 发射：箭头沿排列方向飞出，幽灵壳随之淡出
    if (ghostSvg) {
      tl.to(
        ghostSvg,
        {
          x: isDesktop ? 0 : 260,
          y: isDesktop ? 260 : 0,
          autoAlpha: 0,
          duration: durations.handoffLaunch,
          ease: 'power2.in',
        },
        launchAt,
      )
    }
    tl.to(ghost, { autoAlpha: 0, duration: 0.24, ease: 'power1.out' }, launchAt + 0.05)

    // 假边栏端点的虚拟光标光点：跟随箭头向下的动作，从上方划入到位
    if (edgeLights.length) {
      tl.to(edgeLights, { y: 0, autoAlpha: 1, duration: 0.5, ease: 'power2.out' }, launchAt + 0.06)
    }

    // 正文比按钮弹出延后 0.3s（方向见准备段的说明）：expo.out 给出「弹出」的冲力
    if (viewport) {
      tl.to(viewport, { x: 0, y: 0, autoAlpha: 1, duration: 0.72, ease: easings.out }, launchAt + 0.32)
    }

    // 5) 按钮依次弹出：指示条 + 首页先就位，其余沿排列方向错开涌出
    if (pill) tl.set(pill, { visibility: 'visible' }, launchAt + 0.04)
    tl.set(home, { autoAlpha: 1 }, launchAt + 0.04)
    tl.fromTo(
      home,
      { scale: 0.78 },
      {
        scale: 1,
        duration: durations.handoffButtons,
        ease: 'back.out(2.4)',
        // ⚠️ 起始值不要提前渲染：否则 home 在「准备阶段」就被缩到 0.78，
        // 而 Sidebar 的 placePill 稍后（passive effect）会按这个被污染的 rect
        // 摆放指示条 —— 胶囊会缩成按钮的 78%（比面板还小），表现是「被收缩」
        immediateRender: false,
      },
      launchAt + 0.04,
    )
    tl.fromTo(
      rest,
      { autoAlpha: 0, scale: 0.7 },
      {
        autoAlpha: 1,
        scale: 1,
        duration: durations.handoffButtons,
        ease: 'back.out(2.2)',
        stagger: durations.handoffStagger,
        immediateRender: false,
      },
      launchAt + 0.12,
    )

    // 光晕：每个按钮的图标出现时闪一次主题色辉光
    const restIcons = rest
      .map((b) => b.querySelector('svg'))
      .filter((el): el is SVGSVGElement => el !== null)
    if (restIcons.length > 0) {
      tl.fromTo(
        restIcons,
        { filter: `drop-shadow(0 0 14px ${accent})` },
        {
          filter: `drop-shadow(0 0 0px ${accent})`,
          duration: 0.55,
          ease: 'power2.out',
          stagger: durations.handoffStagger,
        },
        launchAt + 0.12,
      )
    }

    return () => {
      // 演出件移除；按钮/指示条恢复如实（终值与默认一致，不会跳变）。
      // kill 是给 StrictMode 的双跑兜底：旧时间线必须停手，否则会继续写已被清理的样式
      tl.kill()
      ghost.remove()
      gsap.set(btns, { clearProps: 'opacity,visibility,transform,filter' })
      if (edgeLights.length) gsap.set(edgeLights, { clearProps: 'opacity,visibility,transform' })
      if (viewport) gsap.set(viewport, { clearProps: 'opacity,visibility,transform' })
      if (pill) {
        pill.style.visibility = ''
        gsap.set(pill, { autoAlpha: 1 })
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return null
}
