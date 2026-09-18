import { useLayoutEffect, useRef } from 'react'
import gsap from 'gsap'
import { durations, easings } from '../motion/tokens'

/**
 * 「欢迎页 → 登录页」的过场编排（不渲染任何 UI，由 App 在 authIntro 期间挂载）。
 *
 * 序列（点箭头、转圈之后）：
 *   1. 欢迎页的标题 / 副标题 / 按钮壳淡出；XR 放大到屏幕正中心 —— 阶段性"底衬"，
 *      登录卡片淡入后它再淡出（登录页的静止外观不变）；
 *   2. 欢迎页的箭头"离开按钮"（幽灵克隆），**向左下**移动到登录卡片左侧外；
 *      移动途中，它的正下方浮现一个"影子"副箭头（模糊 + 低透明，随后慢慢显影）；
 *      （两箭头的垂直间距 = 两个按钮的箭头间距，之后只需水平右移即可各就各位）
 *   3. 卡片淡入；两个箭头**同时向右平移**，把「登录」「访客登录」两个按钮
 *      "带出来"—— 按钮从左侧随行滑入，箭头在各自按钮的右端飞行；
 *   4. 落位：幽灵隐去、按钮里的真箭头亮出 —— 主箭头 = 登录、副箭头 = 访客登录。
 *
 * ⚠️ Auth 的自播入场动画必须被跳过（intro=true），否则会和这里抢卡片的 opacity。
 * ⚠️ 与 Handoff 同款：出场页（欢迎页）先提出文档流（absolute），否则两个 height:100%
 *    的块会上下堆叠，幽灵的测量目标整体偏一屏。
 * ⚠️ 起点 rect 必须用"中心 + 布局尺寸"换算：此刻箭头可能还带着 [data-busy] 的
 *    scale 过渡（点击后缩到 0.3，正在恢复），getBoundingClientRect 会量到缩放后的盒子。
 */
export function AuthIntro({ onDone }: { onDone: () => void }) {
  const onDoneRef = useRef(onDone)
  onDoneRef.current = onDone

  useLayoutEffect(() => {
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches

    const welcome = document.querySelector<HTMLElement>('.welcome')
    const card = document.querySelector<HTMLElement>('.auth__card')
    const source = document.querySelector<SVGElement>('.welcome__cta .welcome__arrow')
    const titleEl = document.querySelector<HTMLElement>('.welcome__title')
    const subtitle = document.querySelector<HTMLElement>('.welcome__subtitle')
    const cta = document.querySelector<HTMLElement>('.welcome__cta')
    const beam = document.querySelector<HTMLElement>('.welcome__beam')
    const loginBtn = document.querySelector<HTMLElement>('[data-cta="login-submit"]')
    const guestBtn = document.querySelector<HTMLElement>('[data-cta="login-guest"]')
    const loginArrow = loginBtn?.querySelector<SVGElement>('.auth__arrow') ?? null
    const guestArrow = guestBtn?.querySelector<SVGElement>('.auth__arrow') ?? null

    // 结构不对（或用户要求减弱动效）：不演出，直接收尾（Auth 的静态外观保持自然）
    if (
      reduce ||
      !welcome ||
      !card ||
      !source ||
      !titleEl ||
      !subtitle ||
      !cta ||
      !beam ||
      !loginBtn ||
      !guestBtn ||
      !loginArrow ||
      !guestArrow
    ) {
      onDoneRef.current()
      return
    }

    // ---- 欢迎页**先**提出文档流（同 Handoff 的坑）：两个 height:100% 的块会上下
    //      堆叠，登录页此刻被推到第二屏 —— 不先归位就测量，元素的 y 会整体偏一屏
    //      （实测箭头径直飞出屏幕下方一屏高）----
    gsap.set(welcome, {
      position: 'absolute',
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      zIndex: 5,
      pointerEvents: 'none',
    })

    // ---- 测量（必须赶在按钮被"挪走"之前；此刻页面已归位、Auth 已跳过自播）----
    const srcRect = source.getBoundingClientRect()
    const srcStyle = getComputedStyle(source)
    // 尺寸取布局值、位置取"中心"（scale 不影响中心）
    const srcW = parseFloat(srcStyle.width) || 72
    const srcH = parseFloat(srcStyle.height) || 48
    const srcCenter = { x: srcRect.left + srcRect.width / 2, y: srcRect.top + srcRect.height / 2 }

    const loginArrowRect = loginArrow.getBoundingClientRect()
    const guestArrowRect = guestArrow.getBoundingClientRect()
    const cardRect = card.getBoundingClientRect()
    const beamRect = beam.getBoundingClientRect()
    // 副箭头相对主箭头的高度差 = 两个按钮箭头的高度差 —— 与按钮间距同构，
    // 右移阶段两个箭头只走水平线
    const dy = guestArrowRect.top - loginArrowRect.top

    // ---- 幽灵箭头 ×2：从欢迎页箭头克隆（fixed、禁 transition，样式见 .auth-intro-ghost）----
    const makeGhost = () => {
      const ghost = source.cloneNode(true) as SVGElement
      ghost.classList.add('auth-intro-ghost')
      document.body.appendChild(ghost)
      gsap.set(ghost, {
        x: srcCenter.x - srcW / 2,
        y: srcCenter.y - srcH / 2,
        width: srcW,
        height: srcH,
      })
      return ghost
    }
    const mainGhost = makeGhost()
    const subGhost = makeGhost()
    // 双保险：无论欢迎页的画线进度到哪，幽灵都显示完整箭头
    const setFullArrow = (ghost: SVGElement) =>
      ghost.querySelectorAll('.welcome__arrow-shaft, .welcome__arrow-head').forEach((part) => {
        part.setAttribute('stroke-dashoffset', '0')
      })
    setFullArrow(mainGhost)
    setFullArrow(subGhost)

    // ---- 预置 ----
    // 卡片与按钮交给过场：卡片先隐身；按钮挪到左侧待"被带出"；真箭头由幽灵代替
    gsap.set(card, { autoAlpha: 0, y: 14, scale: 0.985 })
    gsap.set([loginBtn, guestBtn], { x: -560 })
    gsap.set([loginArrow, guestArrow], { autoAlpha: 0 })
    gsap.set(source, { autoAlpha: 0 })
    // 副箭头以"影子"形态在场：模糊、几乎不可见
    gsap.set(subGhost, { opacity: 0, filter: 'blur(8px)' })

    // ---- 时刻表（图标放大与箭头**同时起步**，两段都快速到位）----
    const leftEnd = durations.authIntroLeft
    const shadowAt = 0.15 // 影子在移动途中浮现
    const solidifyAt = leftEnd + 0.06 // 影子显影
    const cardAt = solidifyAt + 0.04
    const carryAt = cardAt + 0.08

    // 聚点（左移终点）：登录卡片左侧外，与主箭头终态同一条水平线。
    // 起点在欢迎页按钮（屏幕中右、偏上），所以这一程读起来是"向左下走"
    const gather = { x: cardRect.left - 180, y: loginArrowRect.top }

    const tl = gsap.timeline({ onComplete: () => onDoneRef.current() })

    // 1) 欢迎页文案与按钮壳淡出；XR 放大居中（随后淡出，不留在登录页）
    tl.to([titleEl, subtitle, cta], { autoAlpha: 0, duration: durations.authIntroFade, ease: 'power1.out' }, 0)
    tl.to(
      beam,
      {
        x: window.innerWidth / 2 - (beamRect.left + beamRect.width / 2),
        y: window.innerHeight / 2 - (beamRect.top + beamRect.height / 2),
        scale: 2.4,
        duration: durations.authIntroBeam,
        ease: easings.snappy,
      },
      0,
    )
    tl.to(beam, { autoAlpha: 0, duration: 0.5, ease: 'power1.out' }, 0.55)

    // 2) 主箭头向左下；副箭头（影子）同时在其正下方浮现并跟随
    tl.to(mainGhost, { x: gather.x, y: gather.y, duration: durations.authIntroLeft, ease: easings.snappy }, 0)
    tl.to(
      subGhost,
      { x: gather.x, y: gather.y + dy, duration: durations.authIntroLeft, ease: easings.snappy },
      0,
    )
    tl.to(subGhost, { opacity: 0.5, duration: durations.authIntroShadow, ease: 'power1.out' }, shadowAt)
    // 显影：影子变实 —— 它即将成为访客登录按钮的箭头
    tl.to(
      subGhost,
      { opacity: 1, filter: 'blur(0px)', duration: durations.authIntroShadow, ease: 'power1.inOut' },
      solidifyAt,
    )

    // 3) 卡片淡入；两个箭头同时向右平移，把两个按钮"带出来"
    tl.to(card, { autoAlpha: 1, y: 0, scale: 1, duration: durations.authIntroCard, ease: easings.soft }, cardAt)
    tl.to([loginBtn, guestBtn], { x: 0, duration: durations.authIntroCarry, ease: easings.snappy }, carryAt)
    tl.to(
      mainGhost,
      {
        x: loginArrowRect.left,
        y: loginArrowRect.top,
        width: loginArrowRect.width,
        height: loginArrowRect.height,
        duration: durations.authIntroCarry,
        ease: easings.snappy,
      },
      carryAt,
    )
    tl.to(
      subGhost,
      {
        x: guestArrowRect.left,
        y: guestArrowRect.top,
        width: guestArrowRect.width,
        height: guestArrowRect.height,
        duration: durations.authIntroCarry,
        ease: easings.snappy,
      },
      carryAt,
    )

    // 4) 换手：幽灵隐去、真箭头亮出（位置与尺寸一致，无缝）
    const landAt = carryAt + durations.authIntroCarry
    tl.set([mainGhost, subGhost], { autoAlpha: 0 }, landAt)
    tl.set([loginArrow, guestArrow], { autoAlpha: 1 }, landAt)

    return () => {
      tl.kill()
      mainGhost.remove()
      subGhost.remove()
      gsap.set([titleEl, subtitle, cta], { clearProps: 'opacity,visibility' })
      gsap.set(beam, { clearProps: 'opacity,visibility,transform' })
      gsap.set(card, { clearProps: 'opacity,visibility,transform' })
      gsap.set([loginBtn, guestBtn], { clearProps: 'transform' })
      gsap.set([loginArrow, guestArrow], { clearProps: 'opacity,visibility' })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return null
}
