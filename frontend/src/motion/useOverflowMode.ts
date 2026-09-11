import { useEffect, useState, type RefObject } from 'react'

/**
 * 判断整屏页的内容是否装不下。
 *
 * 这决定主页面的交互轴：
 *   · 装得下 → **纵向**整屏切换（滚轮 / 上下键 / 上下滑），正文不滚动
 *   · 装不下 → 转为手机逻辑：**横向**切换（左右滑 / 左右键 / 横向滚动），正文纵向可滚
 *
 * 之所以能安全地用它来切换模式，是因为两种模式下 `.section` 的几何完全一致
 * （宽高都等于视口），因此 scrollHeight 的判定与当前模式无关，不会来回抖动。
 */
export function useOverflowMode(
  hostRef: RefObject<HTMLElement | null>,
  deps: readonly unknown[] = [],
): boolean {
  const [overflow, setOverflow] = useState(false)

  useEffect(() => {
    const host = hostRef.current
    if (!host) return

    const sections = Array.from(host.querySelectorAll<HTMLElement>('.section'))
    if (sections.length === 0) return

    const measure = () => {
      const anyOverflow = sections.some((section) => {
        const inner = section.firstElementChild as HTMLElement | null
        if (!inner) return section.scrollHeight > section.clientHeight + 1

        // 用**布局尺寸**而不是 scrollHeight：
        // 滚动区域会把 transform 造成的溢出一起算进去，而入场的位移（translate/scale）
        // 会把 scrollHeight 撑大几百像素 —— 那会让这里误判成「装不下」，
        // 把轴切成横向，三个 section 重新排列，看起来就是「页面在左右移动」。
        // offsetHeight 是布局高度，不受 transform 影响。
        const style = getComputedStyle(section)
        const natural =
          inner.offsetHeight + parseFloat(style.paddingTop) + parseFloat(style.paddingBottom)
        return natural > section.clientHeight + 1
      })
      setOverflow((previous) => (previous === anyOverflow ? previous : anyOverflow))
    }

    measure()

    // 观察 section（视口尺寸变化）与内容本体（文案或字体变化）两种情况
    const observer = new ResizeObserver(measure)
    for (const section of sections) {
      observer.observe(section)
      if (section.firstElementChild) observer.observe(section.firstElementChild)
    }
    window.addEventListener('resize', measure)

    return () => {
      observer.disconnect()
      window.removeEventListener('resize', measure)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)

  return overflow
}
