import { useEffect, useRef, useState } from 'react'
import gsap from 'gsap'
import { SITE } from '../../content/site'
import { durations, easings } from '../../motion/tokens'
import type { User } from '../../api/client'

export type SectionId = 'intro' | 'features' | 'contact'

type Props = {
  items: { id: SectionId; label: string }[]
  active: number
  onSelect: (index: number) => void
  user: User
  themeLabel: string
  accent: string
  onLogout: () => void
}

const prefersReducedMotion = () =>
  window.matchMedia('(prefers-reduced-motion: reduce)').matches

/** 「更多」图标：三个圆点的两种横向排布（未展开 / 已连成一条线） */
const DOTS_CLOSED = { x: [3, 10.3, 17.6], width: 3.4, rx: 1.7 }
/** 展开后三段圆角归零、相邻重叠 2.25 单位（≈1px CSS），融成**一条完整的线**。
    重叠要够大：只相切时抗锯齿会在接缝留一道半透明缝，看着仍是三个方块；
    而高分屏（dpr≥2）下 1px 的重叠才稳定盖住那条缝。同色不透明矩形重叠不会变深。
    整条线仍是 3→21，中心 12。 */
const DOTS_OPEN = { x: [3, 8.25, 13.5], width: 7.5, rx: 0 }
/** 视图中心与箭头顶点（SVG 用户坐标） */
const STEM_ORIGIN = '12 12'
const HEAD_ORIGIN = '12 3.5'
/** 菜单滑入的起始下移量（px）。
    只让高度从 0 变到自然高，曲线是「看不出来」的；
    配一个真实的 y 位移，滑动才有过程感 */
const MENU_SLIDE = 16

/**
 * 侧栏：由上到下为 简介 / 功能 / 联系（design.md 2.5）。
 *
 * 三处刻意的实现选择：
 * 1. 选中态是**滑动指示条**（GSAP + smootherstep），不是按钮之间的颜色淡入淡出。
 * 2. 页脚压成一行；点「更多」时**分隔线与身份行整体上抬**，菜单从下方被拉出来
 *    —— 因为页脚是 `margin-top: auto` 贴底，内容变高时顶边自然上移，不需要额外位移。
 * 3. 图标点击后形变：三个圆点连成一条横线 → 整体转 90° → 顶部展出箭头，最终呈 ↑。
 */
export function Sidebar({
  items,
  active,
  onSelect,
  user,
  themeLabel,
  accent,
  onLogout,
}: Props) {
  const navRef = useRef<HTMLElement>(null)
  const pillRef = useRef<HTMLSpanElement>(null)
  const footRef = useRef<HTMLDivElement>(null)
  const buttonRefs = useRef<(HTMLButtonElement | null)[]>([])

  const menuWrapRef = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const stemRef = useRef<SVGGElement>(null)
  const headRef = useRef<SVGPathElement>(null)
  const dotRefs = useRef<(SVGRectElement | null)[]>([])

  const [menuOpen, setMenuOpen] = useState(false)

  // 指示条落位：animate=true 时滑过去，false 时直接放置
  const placePill = (animate: boolean) => {
    const pill = pillRef.current
    const target = buttonRefs.current[active]
    if (!pill || !target) return

    const box = {
      x: target.offsetLeft,
      y: target.offsetTop,
      width: target.offsetWidth,
      height: target.offsetHeight,
      opacity: 1,
    }

    if (!animate || prefersReducedMotion()) {
      gsap.set(pill, box)
      return
    }
    gsap.to(pill, {
      ...box,
      duration: durations.navSlide,
      ease: easings.snappy,
      overwrite: 'auto',
    })
  }

  useEffect(() => {
    const first = pillRef.current?.dataset.ready !== 'true'
    placePill(!first)
    if (pillRef.current) pillRef.current.dataset.ready = 'true'
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active])

  // 窗口尺寸变化（含桌面/移动横竖切换）后重新落位
  useEffect(() => {
    const onResize = () => placePill(false)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active])

  // 菜单拉出：
  // 外层高度 0 → 自然高度（页脚贴底，所以分隔线会跟着整体上抬），
  // 内层同时做一次 16px 的向上滑入。两道配合才有「拉出来」的完整观感。
  useEffect(() => {
    const wrap = menuWrapRef.current
    const menu = menuRef.current
    if (!wrap || !menu) return

    if (prefersReducedMotion()) {
      gsap.set(wrap, menuOpen ? { height: 'auto' } : { height: 0 })
      gsap.set(menu, menuOpen ? { y: 0, opacity: 1 } : { y: MENU_SLIDE, opacity: 0 })
      return
    }

    const tweens = menuOpen
      ? [
          gsap.fromTo(
            wrap,
            { height: 0 },
            { height: 'auto', duration: durations.moreReveal, ease: easings.soft },
          ),
          gsap.fromTo(
            menu,
            { y: MENU_SLIDE, opacity: 0 },
            { y: 0, opacity: 1, duration: durations.moreReveal, ease: easings.soft },
          ),
        ]
      : [
          // 收起不再抢快：与展开同长同曲线（0.42s / power2.out）。
          // 早先用 0.7 倍时长 + power3.out，起步极快，等于「啪」一下就没了，看不出过程。
          gsap.to(menu, {
            y: MENU_SLIDE * 0.6,
            opacity: 0,
            duration: durations.moreReveal,
            ease: easings.soft,
          }),
          gsap.to(wrap, {
            height: 0,
            duration: durations.moreReveal,
            ease: easings.soft,
          }),
        ]

    return () => {
      for (const tween of tweens) tween.kill()
    }
  }, [menuOpen])

  // 图标形变：三点连成横线 → 转 90° → 展出箭头
  useEffect(() => {
    const stem = stemRef.current
    const head = headRef.current
    const dots = dotRefs.current.filter((dot): dot is SVGRectElement => dot !== null)
    if (!stem || !head || dots.length !== 3) return

    const target = menuOpen ? DOTS_OPEN : DOTS_CLOSED
    const morphDots = () =>
      dots.map((dot, index) => ({ dot, x: target.x[index], width: target.width }))

    if (prefersReducedMotion()) {
      for (const { dot, x, width } of morphDots()) {
        gsap.set(dot, { attr: { x, width, rx: target.rx } })
      }
      gsap.set(stem, { rotation: menuOpen ? 90 : 0, svgOrigin: STEM_ORIGIN })
      gsap.set(head, {
        opacity: menuOpen ? 1 : 0,
        scale: menuOpen ? 1 : 0.3,
        svgOrigin: HEAD_ORIGIN,
      })
      return
    }

    // 与指示条、菜单拉出用同一条曲线，否则同一次点击里会看出两种「手感」
    const timeline = gsap.timeline({ defaults: { ease: easings.snappy } })

    if (menuOpen) {
      timeline
        // 1) 三个点连成一条线（圆角同步归零，否则转 90° 后看着是三个方块）
        .to(dots, {
          attr: {
            x: (index: number) => DOTS_OPEN.x[index],
            width: DOTS_OPEN.width,
            rx: DOTS_OPEN.rx,
          },
          duration: durations.moreMorph,
        })
        // 2) 同时整体转 90°（横线转成竖线，充当箭头杆）
        .to(stem, { rotation: 90, svgOrigin: STEM_ORIGIN, duration: durations.moreMorph + 0.04 }, 0.06)
        // 3) 从顶点向外展出箭头两撇
        .fromTo(
          head,
          { opacity: 0, scale: 0.3, svgOrigin: HEAD_ORIGIN },
          { opacity: 1, scale: 1, duration: 0.28 },
          0.3,
        )
    } else {
      // 收起走反向，但更快（退出时用户已知结果）
      timeline
        .to(head, { opacity: 0, scale: 0.3, svgOrigin: HEAD_ORIGIN, duration: 0.16 })
        .to(stem, { rotation: 0, svgOrigin: STEM_ORIGIN, duration: 0.24 }, 0.04)
        .to(
          dots,
          {
            attr: {
              x: (index: number) => DOTS_CLOSED.x[index],
              width: DOTS_CLOSED.width,
              rx: DOTS_CLOSED.rx,
            },
            duration: 0.22,
          },
          0.1,
        )
    }

    return () => {
      timeline.kill()
    }
  }, [menuOpen])

  // 弹出菜单：Esc 与点击外部关闭
  useEffect(() => {
    if (!menuOpen) return

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMenuOpen(false)
    }
    const onPointerDown = (event: MouseEvent) => {
      if (!footRef.current?.contains(event.target as Node)) setMenuOpen(false)
    }

    window.addEventListener('keydown', onKeyDown)
    // 延后一帧再监听，避免触发本次展开的点击立刻把它关掉
    const timer = window.setTimeout(() => document.addEventListener('mousedown', onPointerDown), 0)

    return () => {
      window.removeEventListener('keydown', onKeyDown)
      document.removeEventListener('mousedown', onPointerDown)
      window.clearTimeout(timer)
    }
  }, [menuOpen])

  return (
    <aside className="glass sidebar">
      <div className="sidebar__brand">{SITE.brand}</div>

      <nav className="sidebar__nav" aria-label="主导航" ref={navRef}>
        <span className="sidebar__pill" ref={pillRef} aria-hidden="true" />
        {items.map((item, index) => (
          <button
            key={item.id}
            type="button"
            className="sidebar__btn"
            aria-current={active === index}
            ref={(element) => {
              buttonRefs.current[index] = element
            }}
            onClick={() => onSelect(index)}
          >
            <span className="sidebar__dot" aria-hidden="true" />
            {item.label}
          </button>
        ))}
      </nav>

      <div className="sidebar__foot" ref={footRef}>
        <div className="sidebar__bar">
          <span className="sidebar__identity" title={`主题色 ${accent}（随本地时间连续过渡）`}>
            <span className="sidebar__swatch" style={{ background: accent }} aria-hidden="true" />
            <span className="sidebar__who">
              {themeLabel} · {user.nickname}
            </span>
          </span>

          <button
            type="button"
            className="sidebar__more"
            aria-haspopup="menu"
            aria-expanded={menuOpen}
            aria-label={menuOpen ? '收起更多操作' : '更多操作'}
            onClick={() => setMenuOpen((open) => !open)}
          >
            <svg
              className="sidebar__more-icon"
              width="16"
              height="16"
              viewBox="0 0 24 24"
              aria-hidden="true"
            >
              {/* 三个圆点：展开时连成一条线，再由父级整体转 90° 变成箭头杆 */}
              <g ref={stemRef} className="sidebar__more-stem">
                {[0, 1, 2].map((index) => (
                  <rect
                    key={index}
                    ref={(element) => {
                      dotRefs.current[index] = element
                    }}
                    className="sidebar__more-dot"
                    x={DOTS_CLOSED.x[index]}
                    y="10.3"
                    width={DOTS_CLOSED.width}
                    height="3.4"
                    rx={DOTS_CLOSED.rx}
                  />
                ))}
              </g>
              {/* 箭头两撇：从顶点向外展开 */}
              <path
                ref={headRef}
                className="sidebar__more-head"
                d="M5.5 10 L12 3.5 L18.5 10"
                fill="none"
                stroke="currentColor"
                strokeWidth="2.2"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
          </button>
        </div>

        {/* 高度由 GSAP 从 0 拉到自然高度；页脚贴底，因此分隔线会随之整体上抬.
            菜单不加任何背景/边框，只展出按钮本身 */}
        <div className="sidebar__menu-wrap" ref={menuWrapRef} inert={!menuOpen} aria-hidden={!menuOpen}>
          <div className="sidebar__menu" role="menu" ref={menuRef}>
            {user.role === 'admin' ? (
              <a className="sidebar__menu-item" role="menuitem" href="/admin">
                管理后台
              </a>
            ) : null}
            <button
              type="button"
              className="sidebar__menu-item"
              role="menuitem"
              onClick={() => {
                setMenuOpen(false)
                onLogout()
              }}
            >
              退出登录
            </button>
          </div>
        </div>
      </div>
    </aside>
  )
}
