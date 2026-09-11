import { useCallback, useEffect, useRef, useState } from 'react'
import gsap from 'gsap'
import { Orbs } from '../../components/Orbs'
import { Sidebar } from './Sidebar'
import { ContactSection, FeaturesSection, IntroSection } from './sections'
import type { User } from '../../api/client'

const SECTIONS = [
  { id: 'intro', label: '简介', orb: 'drift' },
  { id: 'features', label: '功能', orb: 'pulse' },
  { id: 'contact', label: '联系', orb: 'wave' },
] as const

type Props = {
  user: User
  themeLabel: string
  onLogout: () => void
}

const prefersReducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches

/**
 * 主页面：整屏切换。
 * 滚轮（含触控板）、方向键、PageUp/Down、Home/End、触屏滑动都能切页；
 * 若当前页内容超出视口，则先把内容滚到底再翻页。
 */
export function MainShell({ user, themeLabel, onLogout }: Props) {
  const [index, setIndex] = useState(0)
  const indexRef = useRef(0)
  const trackRef = useRef<HTMLDivElement>(null)
  const viewportRef = useRef<HTMLDivElement>(null)
  const lockedRef = useRef(false)
  const accRef = useRef(0)

  const goTo = useCallback((next: number) => {
    const clamped = Math.max(0, Math.min(SECTIONS.length - 1, next))
    if (clamped === indexRef.current) return
    indexRef.current = clamped
    setIndex(clamped)
  }, [])

  // 页面切换：位移 + 当前页元素错落入场
  useEffect(() => {
    const track = trackRef.current
    if (!track) return
    const reduce = prefersReducedMotion()

    gsap.to(track, {
      yPercent: -index * 100,
      duration: reduce ? 0 : 0.85,
      ease: 'expo.inOut',
      overwrite: true,
    })

    const current = track.children[index] as HTMLElement | undefined
    const items = current?.querySelectorAll('[data-reveal]')
    if (items?.length) {
      gsap.fromTo(
        items,
        { opacity: 0, y: 18 },
        {
          opacity: 1,
          y: 0,
          duration: reduce ? 0 : 0.5,
          stagger: 0.06,
          ease: 'power2.out',
          overwrite: true,
        },
      )
    }
  }, [index])

  // 滚轮 / 触控板
  useEffect(() => {
    const el = viewportRef.current
    if (!el) return

    const canInnerScroll = (deltaY: number) => {
      const section = el.querySelectorAll<HTMLElement>('.section')[indexRef.current]
      if (!section) return false
      if (section.scrollHeight - section.clientHeight <= 4) return false
      return deltaY < 0
        ? section.scrollTop > 0
        : section.scrollTop + section.clientHeight < section.scrollHeight - 1
    }

    const onWheel = (event: WheelEvent) => {
      if (canInnerScroll(event.deltaY)) return
      event.preventDefault()
      if (lockedRef.current) return

      accRef.current += event.deltaY
      if (Math.abs(accRef.current) < 30) return

      const direction = accRef.current > 0 ? 1 : -1
      accRef.current = 0
      lockedRef.current = true
      goTo(indexRef.current + direction)
      window.setTimeout(() => {
        lockedRef.current = false
      }, 700)
    }

    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [goTo])

  // 键盘与触屏
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      switch (event.key) {
        case 'ArrowDown':
        case 'PageDown':
        case ' ':
          event.preventDefault()
          goTo(indexRef.current + 1)
          break
        case 'ArrowUp':
        case 'PageUp':
          event.preventDefault()
          goTo(indexRef.current - 1)
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

    let startY = 0
    const onTouchStart = (event: TouchEvent) => {
      startY = event.touches[0]?.clientY ?? 0
    }
    const onTouchEnd = (event: TouchEvent) => {
      const endY = event.changedTouches[0]?.clientY ?? startY
      const delta = startY - endY
      if (Math.abs(delta) < 60) return
      const section = viewportRef.current?.querySelectorAll<HTMLElement>('.section')[indexRef.current]
      if (section && section.scrollHeight - section.clientHeight > 4) {
        const atBottom = section.scrollTop + section.clientHeight >= section.scrollHeight - 1
        if (delta > 0 && !atBottom) return
      }
      goTo(indexRef.current + (delta > 0 ? 1 : -1))
    }

    window.addEventListener('keydown', onKeyDown)
    window.addEventListener('touchstart', onTouchStart, { passive: true })
    window.addEventListener('touchend', onTouchEnd, { passive: true })
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('touchstart', onTouchStart)
      window.removeEventListener('touchend', onTouchEnd)
    }
  }, [goTo])

  const current = SECTIONS[index]

  return (
    <div className="shell">
      <Orbs variant={current.orb} />
      <Sidebar
        items={SECTIONS.map((s) => ({ id: s.id, label: s.label }))}
        active={index}
        onSelect={goTo}
        user={user}
        themeLabel={themeLabel}
        onLogout={onLogout}
      />

      <div className="viewport" ref={viewportRef}>
        <div className="viewport__track" ref={trackRef}>
          <section className="section" id="intro" aria-label="简介" inert={index !== 0}>
            <IntroSection />
          </section>
          <section className="section" id="features" aria-label="功能" inert={index !== 1}>
            <FeaturesSection />
          </section>
          <section className="section" id="contact" aria-label="联系" inert={index !== 2}>
            <ContactSection />
          </section>
        </div>

        <div className="pager">
          <span>
            {String(index + 1).padStart(2, '0')} / {String(SECTIONS.length).padStart(2, '0')}
          </span>
          <div className="pager__dots">
            {SECTIONS.map((section, i) => (
              <button
                key={section.id}
                type="button"
                className="pager__dot"
                aria-current={i === index}
                aria-label={`前往${section.label}`}
                onClick={() => goTo(i)}
              />
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
