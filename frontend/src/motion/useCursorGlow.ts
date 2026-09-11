import { useEffect } from 'react'
import { clamp01 } from './math.ts'

/** 高光可见范围（px）：光标离元素超过这个距离就完全不亮 */
const GLOW_RADIUS = 210

/** 参与光晕的元素 —— 只给**本身就有可见边界**的元素（玻璃卡片、实心按钮、侧栏面板）。
    侧栏里那几个按钮是透明无边框的，给它们画边缘光等于凭空长出一圈边框。
    ⚠️ 这份清单必须与 global.css 里那组 ::after / ::before 选择器**逐个对应** ——
    漏一个，那个元素的两层光就是死的（--glow 永远是 0） */
const GLOW_SELECTOR = '.feature-card, .stat, .contact__card, .more__card, .btn, .sidebar'

/**
 * 光标「手电筒」式的边缘高光（iOS 26 Liquid Glass 里的 specular 高光）。
 *
 * 亮斑出现在**朝向光标的那一侧边缘**，不是元素中心自己发光；并且按距离衰减，
 * 所以鼠标扫过一片卡片时，光会像手电筒一样依次擦过它们的边。
 *
 * 这里只算三件事：光标到元素的最短距离 → 强度 → 光标在元素上的相对位置，
 * 全部写成 CSS 变量。亮斑长什么样完全交给 CSS（见 global.css 里的那组 ::after
 * 硬高光 / ::before 外侧弥散光），于是调整观感不需要动 JS。
 *
 * 只挂一次全局 pointermove（rAF 合并），不逐元素绑事件；触屏设备不启用。
 */
export function useCursorGlow() {
  useEffect(() => {
    // 触屏上没有「光标靠近」这回事
    if (!window.matchMedia('(hover: hover)').matches) return

    const findTargets = () => Array.from(document.querySelectorAll<HTMLElement>(GLOW_SELECTOR))
    const dim = (el: HTMLElement) => el.style.setProperty('--glow', '0')

    let raf = 0
    let pointerX = 0
    let pointerY = 0
    let active = false

    const apply = () => {
      raf = 0
      for (const el of findTargets()) {
        if (!active) {
          dim(el)
          continue
        }
        const rect = el.getBoundingClientRect()
        // 光标到矩形的最短距离；落在矩形内部时算 0
        const dx = Math.max(rect.left - pointerX, 0, pointerX - rect.right)
        const dy = Math.max(rect.top - pointerY, 0, pointerY - rect.bottom)
        const strength = clamp01(1 - Math.hypot(dx, dy) / GLOW_RADIUS)

        el.style.setProperty('--glow', strength.toFixed(3))
        if (strength > 0) {
          // 用百分比而不是像素：元素尺寸变化时亮斑位置不用重算
          const px = ((pointerX - rect.left) / rect.width) * 100
          const py = ((pointerY - rect.top) / rect.height) * 100
          el.style.setProperty('--glow-x', `${px.toFixed(1)}%`)
          el.style.setProperty('--glow-y', `${py.toFixed(1)}%`)
        }
      }
    }

    const schedule = () => {
      if (!raf) raf = requestAnimationFrame(apply)
    }

    const onPointerMove = (event: PointerEvent) => {
      if (event.pointerType !== 'mouse') return
      pointerX = event.clientX
      pointerY = event.clientY
      active = true
      schedule()
    }

    // 光标离开窗口 / 窗口失焦时全部熄灭，避免残留一片高光
    const onPointerOut = () => {
      active = false
      schedule()
    }

    window.addEventListener('pointermove', onPointerMove, { passive: true })
    document.addEventListener('pointerleave', onPointerOut)
    window.addEventListener('blur', onPointerOut)

    return () => {
      window.removeEventListener('pointermove', onPointerMove)
      document.removeEventListener('pointerleave', onPointerOut)
      window.removeEventListener('blur', onPointerOut)
      if (raf) cancelAnimationFrame(raf)
      for (const el of findTargets()) dim(el)
    }
  }, [])
}
