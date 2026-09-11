import { useEffect, useRef } from 'react'
import { orbState, type OrbVariant } from '../motion/orbMotion'

const prefersReducedMotion = () =>
  window.matchMedia('(prefers-reduced-motion: reduce)').matches

/**
 * 驱动 .orb-field 里的三层光斑。
 *
 * 相比原先"三个固定周期的 CSS keyframes"，这里用互质周期叠加：
 * 长期不重复，看不出在循环；并且页面隐藏时完全停掉，不白烧 CPU。
 */
export function useOrbField(variant: OrbVariant) {
  const hostRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const host = hostRef.current
    if (!host) return

    const layers = Array.from(host.querySelectorAll<HTMLElement>('[data-orb]'))
    if (layers.length === 0) return

    // 减少动态效果：停在初始位置，不启动 rAF
    if (prefersReducedMotion()) {
      for (const layer of layers) layer.style.transform = ''
      return
    }

    let raf = 0
    let last = performance.now()
    let elapsed = 0

    const paint = () => {
      const unit = Math.min(window.innerWidth, window.innerHeight) / 100
      layers.forEach((layer, index) => {
        const { x, y, scale } = orbState(variant, elapsed, index, unit)
        layer.style.transform = `translate3d(${x.toFixed(2)}px, ${y.toFixed(2)}px, 0) scale(${scale.toFixed(4)})`
      })
    }

    const tick = (now: number) => {
      // 时间累加而非取时间戳：暂停恢复后不会瞬移
      elapsed += Math.min(0.05, (now - last) / 1000)
      last = now
      paint()
      raf = requestAnimationFrame(tick)
    }

    const start = () => {
      if (raf) return
      last = performance.now()
      raf = requestAnimationFrame(tick)
    }
    const stop = () => {
      cancelAnimationFrame(raf)
      raf = 0
    }
    const onVisibility = () => (document.hidden ? stop() : start())

    paint()
    start()
    document.addEventListener('visibilitychange', onVisibility)

    return () => {
      stop()
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [variant])

  return hostRef
}
