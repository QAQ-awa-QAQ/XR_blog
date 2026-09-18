import { useEffect, useRef } from 'react'
import gsap from 'gsap'
import { Orbs } from '../components/Orbs'
import { SITE } from '../content/site'
import { durations, easings } from '../motion/tokens'

const TITLE = SITE.welcome.title

type Props = {
  /** 点击「登录 →」后由外层校验登录态 */
  onEnter: () => void
  busy: boolean
}

/**
 * 欢迎页时间轴（时序）：
 *   1. 浏览器默认色（浅色白 / 深色近黑）的幕布淡出 → 露出随本地时间取色的主题背景；
 *   2. 「XR」大 logo 在屏幕中心弹性浮现（阴影加重 = 纵深感加强）；
 *   3. logo 沿一条微拱的弧线向左平移；标题从 logo 底下向右「钻出」——
 *      裁剪边界始终钉在 logo 右缘，所以右端的「站」第一个露头，logo 左侧不出现文字；
 *      登录按钮从「站」字下方向右滑出；
 *   4. 按钮暴露在文字外后，里面的横线向右画出、箭头尖从线端展开 —— 最后呈「→」。
 *
 * 三个元素的横向起点不写死像素：都在 JS 里量出「终态位置与屏幕中心的差」，
 * 再从那里起步，所以换字号 / 换文案 / 换视口都不用改动画；
 * 窄屏竖排时量出的差趋近 0，动画自动退化为淡入，不会错位。
 *
 * 点击任意处可跳过；prefers-reduced-motion 下直接落到终态。
 */
export function Welcome({ onEnter, busy }: Props) {
  const rootRef = useRef<HTMLDivElement>(null)
  const curtainRef = useRef<HTMLDivElement>(null)
  const beamRef = useRef<HTMLDivElement>(null)
  const titleRef = useRef<HTMLHeadingElement>(null)
  const ctaRef = useRef<HTMLButtonElement>(null)
  const shaftRef = useRef<SVGLineElement>(null)
  const headARef = useRef<SVGPathElement>(null)
  const headBRef = useRef<SVGPathElement>(null)
  const timelineRef = useRef<gsap.core.Timeline | null>(null)

  useEffect(() => {
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches

    const ctx = gsap.context(() => {
      const curtain = curtainRef.current
      const beam = beamRef.current
      const title = titleRef.current
      const cta = ctaRef.current
      const shaft = shaftRef.current
      const headA = headARef.current
      const headB = headBRef.current
      if (!curtain || !beam || !title || !cta || !shaft || !headA || !headB) return

      if (reduceMotion) {
        gsap.set(curtain, { opacity: 0 })
        gsap.set(title, { clipPath: 'none' })
        gsap.set(cta, { opacity: 1 })
        gsap.set([shaft, headA, headB], { attr: { 'stroke-dashoffset': 0 } })
        return
      }

      const chars = gsap.utils.toArray<HTMLElement>('.welcome__char')
      const lastChar = chars[chars.length - 1]
      /** 把元素平移到屏幕正中央所需的 x 位移（按终态布局量，不含任何补间） */
      const toCenterX = (el: Element) => {
        const rect = el.getBoundingClientRect()
        return window.innerWidth / 2 - (rect.left + rect.width / 2)
      }

      // 先量一遍终态布局（此刻还没有任何补间写过 transform）
      const beamRect = beam.getBoundingClientRect()
      const titleRect = title.getBoundingClientRect()
      const lastCharRect = lastChar?.getBoundingClientRect()
      const ctaRect = cta.getBoundingClientRect()

      // logo 起点 = 屏幕中心（「居中浮现 → 左移」）
      gsap.set(beam, { x: toCenterX(beam) })

      // 标题「从 logo 底下钻出」：起点让**文字右端**与 logo 的初始右缘对齐（整段藏在
      // logo 左侧外），之后每帧把裁剪边界钉在 logo 当前的右缘 —— 切口紧贴 logo 扩张，
      // 文字最右端的「站」第一个露头，logo 左侧全程不会出现文字。
      // 仅当终态是一行横排（标题整段在 logo 右侧）时启用；竖排退化为淡入
      const drillOut = titleRect.left >= beamRect.right - 1
      /** 分列阶段开始前锁死全裁 —— logo 浮现时在缩小，右缘会暂时内缩，不能提前露字 */
      let shifting = false
      const updateTitleClip = () => {
        if (!drillOut) return
        if (!shifting) {
          title.style.clipPath = 'inset(0 0 0 100%)'
          return
        }
        const cut = Math.max(
          0,
          beam.getBoundingClientRect().right - title.getBoundingClientRect().left,
        )
        title.style.clipPath = `inset(0 0 0 ${cut}px)`
      }
      if (drillOut) {
        const logoStartRight = window.innerWidth / 2 + beamRect.width / 2
        gsap.set(title, { x: logoStartRight - titleRect.right })
      } else {
        gsap.set(title, { clipPath: 'none', opacity: 0 })
      }
      updateTitleClip()

      // 按钮起点：横向中心对准末字「站」（再压低一点）——「从站字下面滑出，离开站字」
      gsap.set(cta, {
        x:
          (lastCharRect
            ? lastCharRect.left + lastCharRect.width / 2
            : titleRect.left + titleRect.width / 2) -
          (ctaRect.left + ctaRect.width / 2),
        y: 12,
        opacity: 0,
      })
      gsap.set(shaft, { attr: { 'stroke-dashoffset': 18 } })
      gsap.set([headA, headB], { attr: { 'stroke-dashoffset': 9 } })

      const tl = gsap.timeline({ defaults: { ease: easings.smooth }, onUpdate: updateTitleClip })
      timelineRef.current = tl

      tl.to(curtain, { opacity: 0, duration: durations.welcomeCurtain, ease: easings.soft }, 0)
        .from(
          beam,
          {
            scale: 0.55,
            opacity: 0,
            y: -16,
            duration: durations.welcomeBeam,
            ease: 'elastic.out(1, 0.62)',
          },
          0.1,
        )
        // 同一时间窗：logo 沿微拱弧线左移（标题裁剪随之逐帧扩张）；按钮从「站」字下向右滑出
        .to(
          beam,
          {
            x: 0,
            duration: durations.welcomeShift,
            ease: easings.snappy,
            onStart: () => {
              shifting = true
            },
          },
          '+=0.15',
        )
        .to(
          beam,
          {
            keyframes: [
              { y: -12, duration: durations.welcomeShift * 0.45, ease: 'power2.out' },
              { y: 0, duration: durations.welcomeShift * 0.55, ease: 'power2.in' },
            ],
          },
          '<',
        )

      if (drillOut) {
        tl.to(title, { x: 0, duration: durations.welcomeShift, ease: easings.snappy }, '<')
      } else {
        tl.fromTo(
          title,
          { opacity: 0 },
          { opacity: 1, duration: durations.welcomeShift, ease: easings.snappy },
          '<',
        )
      }

      tl.to(
        cta,
        { x: 0, y: 0, opacity: 1, duration: durations.welcomeShift, ease: easings.snappy },
        '<',
      )

      // 按钮暴露在文字外后：横线向右画出 → 箭头尖从线端张开。
      // 副标题与**箭头绘制同时开始** —— 用同一个锚点时刻，不要再挂在
      // 箭头 tween 的相对位置上（那会随箭头时长漂移）
      const arrowAt = tl.duration() + 0.08
      tl.to(
        shaft,
        { attr: { 'stroke-dashoffset': 0 }, duration: durations.welcomeDraw, ease: 'power1.inOut' },
        arrowAt,
      )
      tl.to(
        [headA, headB],
        { attr: { 'stroke-dashoffset': 0 }, duration: durations.welcomeArrow, ease: easings.out },
        arrowAt + durations.welcomeDraw - 0.04,
      )
      tl.from('.welcome__subtitle', { opacity: 0, y: 10, duration: 0.5 }, arrowAt)
    }, rootRef)

    return () => {
      timelineRef.current = null
      ctx.revert()
    }
  }, [])

  const skip = () => timelineRef.current?.progress(1)

  return (
    <div className="welcome" ref={rootRef} onClick={skip}>
      <Orbs variant="drift" />
      <div className="welcome__curtain" ref={curtainRef} aria-hidden="true" />

      <div className="welcome__inner">
        <div className="welcome__row">
          <div className="welcome__beam" ref={beamRef} aria-hidden="true">
            XR
          </div>

          <h1 className="welcome__title" ref={titleRef} aria-label={TITLE}>
            {TITLE.split('').map((char, index) => (
              <span className="welcome__char" key={`${char}-${index}`} aria-hidden="true">
                {char}
              </span>
            ))}
          </h1>

          <button
            type="button"
            className="btn btn--glass welcome__cta"
            ref={ctaRef}
            disabled={busy}
            aria-label="登录"
            data-busy={busy || undefined}
            onClick={(event) => {
              // 先让入场时间轴落位（等同「点击任意处跳过」）：保证里的箭头是画完的
              // 完整态 —— 后面的「欢迎页 → 登录页」过场会克隆它（否则会克隆到画一半的箭头）
              event.stopPropagation()
              skip()
              onEnter()
            }}
          >
            {/* 转圈常驻 DOM：显隐交给 [data-busy] 的 CSS 过渡（进出都有交接动画） */}
            <span className="spinner" aria-hidden="true" />
            {/* viewBox 的 x 起点取 -1.75：图形本体占 1.5..19，左移视窗后水平居中 */}
            <svg className="welcome__arrow" viewBox="-1.75 0 24 16" aria-hidden="true">
              <line
                ref={shaftRef}
                className="welcome__arrow-shaft"
                x1="1.5"
                y1="8"
                x2="19"
                y2="8"
                strokeDasharray="18"
                strokeDashoffset="18"
              />
              {/* 箭头尖 = 两条都从顶点 (19,8) 出发的短线：
                  用 dashoffset「画出」时就是「从线端张开」——与横线同一套机制 */}
              <path
                ref={headARef}
                className="welcome__arrow-head"
                d="M19 8 L12.2 2.8"
                strokeDasharray="9"
                strokeDashoffset="9"
              />
              <path
                ref={headBRef}
                className="welcome__arrow-head"
                d="M19 8 L12.2 13.2"
                strokeDasharray="9"
                strokeDashoffset="9"
              />
            </svg>
          </button>
        </div>

        <p className="welcome__subtitle">{SITE.welcome.subtitle}</p>
      </div>
    </div>
  )
}
