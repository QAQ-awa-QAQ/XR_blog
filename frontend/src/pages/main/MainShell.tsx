import { useCallback, useEffect, useRef, useState } from 'react'
import gsap from 'gsap'
import { Orbs } from '../../components/Orbs'
import { Sidebar, type SectionId } from './Sidebar'
import { ContactSection, FeaturesSection, IntroSection, MoreSection } from './sections'
import { springStep, waveAssign } from '../../motion/math'
import { durations, easings, omega } from '../../motion/tokens'
import { useOverflowMode } from '../../motion/useOverflowMode'
import type { OrbVariant } from '../../motion/orbMotion'
import type { User } from '../../api/client'

const SECTIONS: { id: SectionId; label: string; orb: OrbVariant }[] = [
  // label 现在同时是侧栏图标按钮的 aria-label / title（按钮只剩图标了），
  // 以及分页器的「前往 XX」。所以它得是一个**动作**的说法，不只是分区名
  { id: 'intro', label: '首页', orb: 'drift' },
  { id: 'features', label: '功能', orb: 'pulse' },
  { id: 'contact', label: '联系', orb: 'wave' },
  // 「更多」是第四个**页面**（站名 / 账户 / 后台 / 退出），不是一个弹出菜单。
  // 光斑复用 drift：这一页全是账户信息，背景不该抢戏
  { id: 'more', label: '更多', orb: 'drift' },
]

/** 触控板细碎抖动过滤（不是时间锁，只用来判定"这一次算不算一次意图"） */
const GESTURE_THRESHOLD = 24
/** 触屏滑动的最小距离与主方向倍数 */
const SWIPE_MIN = 60
const SWIPE_RATIO = 1.2

/** 弹簧的落位阈值（单位：屏）。
    临界阻尼是指数收敛，最后零点几个百分点会拖上一大段，看着像「停不下来」；
    剩余不足 0.05% 屏（任何屏上都不到一个像素）就直接落位。 */
const SNAP_EPSILON = 0.0005

/** 登录后**首次**进入主页时，弹簧只用这么多幅度（正文之间互切时给满） */
const FIRST_REVEAL_SCALE = 0.4

/** 元素涌入的起始距离（px）。够远才读得出流动感 */
const REVEAL_RISE = 640

/** 波包式入场按 3 列估算距离（用于错开与幅度） */
const REVEAL_COLUMNS = 3

type Props = {
  user: User
  themeLabel: string
  accent: string
  onLogout: () => void
}

/**
 * 主页面：整屏切换。
 *
 * 两条轴，按内容装不装得下自动选择：
 *   装得下 → 纵向切换（滚轮 / 上下键 / 上下滑），正文不滚动 —— 设计稿的默认形态
 *   装不下 → 手机逻辑：横向切换（左右滑 / 左右键 / 横向滚动），正文纵向自己滚
 *
 * 切换本身不靠"事件 + 时间锁"：滚轮只更新目标值，画面由临界阻尼弹簧逐帧追过去，
 * 所以连续快滚会合并成最新目标，运动途中反向也能保留速度改向。
 */
export function MainShell({ user, themeLabel, accent, onLogout }: Props) {
  const [target, setTarget] = useState(0)

  const targetRef = useRef(0)
  const prevTargetRef = useRef(0)
  const firstRevealRef = useRef(true)
  const axisRef = useRef<'y' | 'x'>('y')
  const positionRef = useRef(0)
  const velocityRef = useRef(0)
  const accumulatorRef = useRef(0)
  const trackRef = useRef<HTMLDivElement>(null)
  const viewportRef = useRef<HTMLDivElement>(null)

  // 内容装不下时改走横向切换；两种模式下 section 几何一致，判定不会来回抖动
  const overflow = useOverflowMode(viewportRef, [target])
  const axis: 'y' | 'x' = overflow ? 'x' : 'y'
  axisRef.current = axis

  const goTo = useCallback((next: number) => {
    const clamped = Math.max(0, Math.min(SECTIONS.length - 1, next))
    if (clamped === targetRef.current) return
    targetRef.current = clamped
    setTarget(clamped)
  }, [])

  const step = useCallback((delta: number) => goTo(targetRef.current + delta), [goTo])

  // 唯一的时间源：临界阻尼弹簧
  useEffect(() => {
    const track = trackRef.current
    if (!track) return

    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    let raf = 0
    let last = performance.now()

    const tick = (now: number) => {
      const dt = Math.min(0.05, (now - last) / 1000)
      last = now

      if (reduce) {
        positionRef.current = targetRef.current
        velocityRef.current = 0
      } else {
        const [position, velocity] = springStep(
          positionRef.current,
          velocityRef.current,
          targetRef.current,
          omega.page,
          dt,
        )
        // 已经只剩亚像素级差距时直接吸附，否则会在目标附近以极小速度挪很久
        if (Math.abs(position - targetRef.current) < SNAP_EPSILON) {
          positionRef.current = targetRef.current
          velocityRef.current = 0
        } else {
          positionRef.current = position
          velocityRef.current = velocity
        }
      }

      const percent = (-positionRef.current * 100).toFixed(4)
      track.style.transform =
        axisRef.current === 'x'
          ? `translate3d(${percent}%, 0, 0)`
          : `translate3d(0, ${percent}%, 0)`
      raf = requestAnimationFrame(tick)
    }

    raf = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf)
  }, [])

  // 滚轮 / 触控板
  useEffect(() => {
    const viewport = viewportRef.current
    if (!viewport) return

    // 正文可否继续纵向滚动（横向模式下由浏览器处理，这里只用于纵向模式的让位判断）
    const canInnerScroll = (deltaY: number) => {
      const section = viewport.querySelectorAll<HTMLElement>('.section')[targetRef.current]
      if (!section) return false
      if (section.scrollHeight - section.clientHeight <= 4) return false
      return deltaY < 0
        ? section.scrollTop > 0
        : section.scrollTop + section.clientHeight < section.scrollHeight - 1
    }

    const onWheel = (event: WheelEvent) => {
      // 横向模式：纵向手势交给正文滚动，只有横向手势才切页
      if (axisRef.current === 'x') {
        const horizontal = event.shiftKey ? event.deltaY : event.deltaX
        const vertical = event.shiftKey ? 0 : event.deltaY
        if (Math.abs(horizontal) <= Math.abs(vertical)) return

        event.preventDefault()
        accumulatorRef.current += horizontal
        if (Math.abs(accumulatorRef.current) < GESTURE_THRESHOLD) return

        const direction = accumulatorRef.current > 0 ? 1 : -1
        accumulatorRef.current = 0
        step(direction)
        return
      }

      // 纵向模式：正文先滚完，再翻页
      if (canInnerScroll(event.deltaY)) return
      event.preventDefault()

      accumulatorRef.current += event.deltaY
      if (Math.abs(accumulatorRef.current) < GESTURE_THRESHOLD) return

      const direction = accumulatorRef.current > 0 ? 1 : -1
      accumulatorRef.current = 0
      step(direction)
    }

    viewport.addEventListener('wheel', onWheel, { passive: false })
    return () => viewport.removeEventListener('wheel', onWheel)
  }, [step])

  // 键盘与触屏
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (axisRef.current === 'x') {
        switch (event.key) {
          case 'ArrowRight':
            event.preventDefault()
            step(1)
            return
          case 'ArrowLeft':
            event.preventDefault()
            step(-1)
            return
          case 'Home':
            event.preventDefault()
            goTo(0)
            return
          case 'End':
            event.preventDefault()
            goTo(SECTIONS.length - 1)
            return
          default:
            // 上下键 / PageUp / PageDown / 空格：交给正文滚动
            return
        }
      }

      switch (event.key) {
        case 'ArrowDown':
        case 'PageDown':
        case ' ':
          event.preventDefault()
          step(1)
          break
        case 'ArrowUp':
        case 'PageUp':
          event.preventDefault()
          step(-1)
          break
        case 'Home':
          event.preventDefault()
          goTo(0)
          break
        case 'End':
          event.preventDefault()
          goTo(SECTIONS.length - 1)
          break
        default:
          break
      }
    }

    let startX = 0
    let startY = 0
    const onTouchStart = (event: TouchEvent) => {
      startX = event.touches[0]?.clientX ?? 0
      startY = event.touches[0]?.clientY ?? 0
    }
    const onTouchEnd = (event: TouchEvent) => {
      const endX = event.changedTouches[0]?.clientX ?? startX
      const endY = event.changedTouches[0]?.clientY ?? startY
      const dx = startX - endX
      const dy = startY - endY

      // 横向模式：横向滑动切页，纵向滑动交给正文
      if (axisRef.current === 'x') {
        if (Math.abs(dx) < SWIPE_MIN) return
        if (Math.abs(dx) <= Math.abs(dy) * SWIPE_RATIO) return
        step(dx > 0 ? 1 : -1)
        return
      }

      if (Math.abs(dy) < SWIPE_MIN) return
      const section = viewportRef.current?.querySelectorAll<HTMLElement>('.section')[targetRef.current]
      if (section && section.scrollHeight - section.clientHeight > 4) {
        const atBottom = section.scrollTop + section.clientHeight >= section.scrollHeight - 1
        if (dy > 0 && !atBottom) return
      }
      step(dy > 0 ? 1 : -1)
    }

    window.addEventListener('keydown', onKeyDown)
    window.addEventListener('touchstart', onTouchStart, { passive: true })
    window.addEventListener('touchend', onTouchEnd, { passive: true })
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('touchstart', onTouchStart)
      window.removeEventListener('touchend', onTouchEnd)
    }
  }, [goTo, step])

  // 目标页的卡片入场：波包式，延迟随到波源的距离增长
  useEffect(() => {
    const section = trackRef.current?.children[target] as HTMLElement | undefined
    if (!section) return

    const items = Array.from(section.querySelectorAll<HTMLElement>('[data-reveal]'))
    if (items.length === 0) return

    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches

    const duration = reduce ? 0 : durations.cardReveal
    // 首次进入（登录 → 简介）只用四成幅度：那一刻整页刚从登录页换过来，
    // 足幅度的涌入叠在「换了页面」这件事上会显得特别猛。站内互切时才给满。
    const k = firstRevealRef.current ? FIRST_REVEAL_SCALE : 1
    firstRevealRef.current = false

    // 流入方向跟随**翻页方向**：新 section 从哪边进来，元素就从哪边流入。
    // 方向固定时，反方向那次元素是「逆着容器」走的，读起来就是拉伸。
    const previous = prevTargetRef.current
    const direction = Math.sign(target - previous)
    prevTargetRef.current = target
    const dir = direction === 0 ? 1 : direction

    // 波包错开是「层次感」的来源，不要去掉：所有元素同时涌入会退化成整块平移，
    // 既没有彼此被推着走的感觉，也看不出幅度差异。
    const stagger = (index: number) => waveAssign(index, REVEAL_COLUMNS).delay

    // 刻意**不用** elastic：过冲会让相邻元素来回挤压，看着像「晃」而不是「流」。
    const flow = easings.soft
    const horizontal = axisRef.current === 'x'

    // 退场：旧页面朝**翻页去向**流走，与新页面的来向正好对称。
    // 少了这一步，切走的那一页只是被容器拖走，自己没有任何动作。
    if (direction !== 0) {
      const leaving = trackRef.current?.children[previous]?.querySelectorAll<HTMLElement>(
        '[data-reveal]',
      )
      if (leaving?.length) {
        const outShift = (index: number) =>
          -direction * REVEAL_RISE * waveAssign(index, REVEAL_COLUMNS).amplitude
        gsap.to(leaving, {
          opacity: 0,
          ...(horizontal ? { x: outShift } : { y: outShift }),
          duration,
          ease: flow,
          stagger,
          overwrite: 'auto',
        })
      }
    }

    // 缩放：从略小的状态「胀」回原位
    gsap.fromTo(
      items,
      { scale: 1 - 0.07 * k },
      { scale: 1, duration, ease: flow, stagger, overwrite: 'auto' },
    )

    // 涌入：从翻页方向那一侧流入。波包幅度随距离递减，
    // 所以靠后的元素起始更近 —— **起始间距比最终间距更紧**（压缩）；
    // 而错开带来的先后差，就是这个效果里「弹簧」的来源。
    //
    // 手机模式（横向轴）整屏是左右切换的，元素就得**左右**流入才对得上；
    // 两个轴都沿用同一个 dir，所以始终是「新的一屏从哪边进来，元素就从哪边进」。
    const shift = (index: number) =>
      dir * REVEAL_RISE * k * waveAssign(index, REVEAL_COLUMNS).amplitude
    const from: gsap.TweenVars = horizontal ? { opacity: 0, x: shift } : { opacity: 0, y: shift }

    gsap.fromTo(items, from, {
      opacity: 1,
      // 两个轴都归零：否则从纵向模式切到横向时会残留上一个轴的位移
      x: 0,
      y: 0,
      duration,
      ease: flow,
      stagger,
      overwrite: 'auto',
    })
  }, [target])

  return (
    <div className="shell">
      <Orbs variant={SECTIONS[target].orb} />

      <Sidebar
        items={SECTIONS.map((section) => ({ id: section.id, label: section.label }))}
        active={target}
        onSelect={goTo}
      />

      <div className="viewport" ref={viewportRef}>
        <div className="viewport__track" ref={trackRef} data-axis={axis}>
          <section className="section" id="intro" aria-label="首页" inert={target !== 0}>
            <div className="section__body">
              <IntroSection />
            </div>
          </section>
          <section className="section" id="features" aria-label="功能" inert={target !== 1}>
            <div className="section__body">
              <FeaturesSection />
            </div>
          </section>
          <section className="section" id="contact" aria-label="联系" inert={target !== 2}>
            <div className="section__body">
              <ContactSection />
            </div>
          </section>
          <section className="section" id="more" aria-label="更多" inert={target !== 3}>
            <div className="section__body">
              <MoreSection
                user={user}
                themeLabel={themeLabel}
                accent={accent}
                onLogout={onLogout}
              />
            </div>
          </section>
        </div>

        <div className="pager">
          <span>
            {String(target + 1).padStart(2, '0')} / {String(SECTIONS.length).padStart(2, '0')}
          </span>
          <div className="pager__dots">
            {SECTIONS.map((section, index) => (
              <button
                key={section.id}
                type="button"
                className="pager__dot"
                aria-current={index === target}
                aria-label={`前往${section.label}`}
                onClick={() => goTo(index)}
              />
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
