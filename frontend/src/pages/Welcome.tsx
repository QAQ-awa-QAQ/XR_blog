import { useEffect, useRef } from 'react'
import gsap from 'gsap'
import { Orbs } from '../components/Orbs'

const TITLE = '欢迎访问XR个人站'

type Props = {
  /** 点击「登录 →」后由外层校验登录态 */
  onEnter: () => void
  busy: boolean
}

/**
 * 欢迎页：GSAP 时间轴按「水滴玻璃浮出 → 文字逐字凝聚 → 登录入口浮现」的顺序入场。
 * 点击任意处可跳过动画（尊重 prefers-reduced-motion 时直接落到终态）。
 */
export function Welcome({ onEnter, busy }: Props) {
  const rootRef = useRef<HTMLDivElement>(null)
  const beamRef = useRef<HTMLDivElement>(null)
  const ctaRef = useRef<HTMLButtonElement>(null)
  const timelineRef = useRef<gsap.core.Timeline | null>(null)

  useEffect(() => {
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches

    const ctx = gsap.context(() => {
      const chars = gsap.utils.toArray<HTMLElement>('.welcome__char')

      if (reduceMotion) {
        gsap.set([beamRef.current, ctaRef.current, ...chars], { opacity: 1, y: 0, scale: 1, filter: 'none' })
        return
      }

      const tl = gsap.timeline({ defaults: { ease: 'expo.out' } })
      timelineRef.current = tl

      tl.from(beamRef.current, {
        scale: 0.55,
        opacity: 0,
        duration: 1,
        ease: 'elastic.out(1, 0.62)',
      })
        .from(
          chars,
          { opacity: 0, y: 20, filter: 'blur(10px)', duration: 0.65, stagger: 0.035 },
          '-=0.35',
        )
        .from('.welcome__subtitle', { opacity: 0, y: 10, duration: 0.5 }, '-=0.3')
        .fromTo(
          ctaRef.current,
          { opacity: 0, y: 14 },
          { opacity: 1, y: 0, duration: 0.55 },
          '-=0.15',
        )
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
      <div className="welcome__inner">
        <div className="welcome__beam" ref={beamRef} aria-hidden="true">
          XR
        </div>

        <h1 className="welcome__title" aria-label={TITLE}>
          {TITLE.split('').map((char, index) => (
            <span className="welcome__char" key={`${char}-${index}`} aria-hidden="true">
              {char}
            </span>
          ))}
        </h1>

        <p className="welcome__subtitle">一个关于我、我的作品与联系方式的地方</p>

        <button
          type="button"
          className="btn btn--glass welcome__cta"
          ref={ctaRef}
          disabled={busy}
          onClick={(event) => {
            event.stopPropagation()
            onEnter()
          }}
        >
          {busy ? <span className="spinner" /> : null}
          登录 →
        </button>
      </div>
    </div>
  )
}
