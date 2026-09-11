import { useEffect, useRef, type CSSProperties } from 'react'
import gsap from 'gsap'
import { durations, easings } from '../../motion/tokens'
import {
  ICON_SCALE_IDLE,
  ICON_SIZE,
  ICON_VIEWBOX,
  NAV_ICON_PATHS,
  type NavSectionKey,
} from '../../design/icons'

/** 侧栏导航里的四个分区。键就是 design/icons.ts 的图标名（那个联合类型只定义一次） */
export type SectionId = NavSectionKey

type Props = {
  items: { id: SectionId; label: string }[]
  active: number
  onSelect: (index: number) => void
}

const prefersReducedMotion = () =>
  window.matchMedia('(prefers-reduced-motion: reduce)').matches

/** 把图标的两个设计参数交给 CSS：基准尺寸与未选中态的倍率 */
type IconVars = CSSProperties & {
  '--icon-size': string
  '--icon-scale-idle': number
}

/** 导航图标。渲染口径与 sections.tsx 的功能卡片图标一致：只描边、圆头、1.7 */
function NavIcon({ paths }: { paths: readonly string[] }) {
  return (
    <svg
      width={ICON_SIZE}
      height={ICON_SIZE}
      viewBox={`0 0 ${ICON_VIEWBOX} ${ICON_VIEWBOX}`}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths.map((d) => (
        <path key={d} d={d} />
      ))}
    </svg>
  )
}

/**
 * 侧栏：竖排胶囊里四个等大的按钮 —— 首页 / 功能 / 联系 / 更多。
 *
 * 四处刻意的实现选择：
 * 1. 按钮**只剩图标**：侧栏收到 42px 后连「简介」两个字都排不下。
 *    代价是标签得换一条路可读 —— `aria-label` 给屏幕阅读器、`title` 给鼠标用户，
 *    两者都取 SECTIONS 里的中文标签。
 * 2. 四个按钮地位完全相同，都在指示条队列里：第四个「更多」也是一个**页面**
 *    （放站名、时段与身份、管理后台、退出登录），不是弹出菜单。
 * 3. 图标是设计参数，放在 `src/design/icons.ts`；未选中 / 选中靠**倍率**区分，
 *    所以这里把它写成 CSS 变量交给样式表，而不是在 TSX 里按 aria-current 分支。
 * 4. `aria-current` 同时驱动图标倍率与颜色，避免“选中”这件事散成好几处判断。
 * 5. 面板的壳（描边 / 内高光 / 内反光）不在面板本体上，而在 `.sidebar__edge` 上 ——
 *    胶囊比面板大一圈，画在本体上的框线会从胶囊内部穿过。壳层用一个跟随胶囊的
 *    「洞」（CSS mask 的 exclude）挖掉胶囊覆盖的区域：不求轮廓交点，**形状本身就是答案**。
 */
export function Sidebar({ items, active, onSelect }: Props) {
  const asideRef = useRef<HTMLElement>(null)
  const navRef = useRef<HTMLElement>(null)
  const pillRef = useRef<HTMLSpanElement>(null)
  const buttonRefs = useRef<(HTMLButtonElement | null)[]>([])

  // 壳层上那个「洞」的当前值。用一个普通对象交给 GSAP tween，
  // 再在 onUpdate 里写进 CSS 变量 —— 这样洞与指示条**同一条曲线、同一时刻**移动，
  // 不会各走各的露出半截断口
  const cutRef = useRef({ x: 0, y: 0, width: 0, height: 0 })

  // 图标参数来自 design/icons.ts，这里只负责把它交给样式表
  const iconVars: IconVars = {
    '--icon-size': `${ICON_SIZE}px`,
    '--icon-scale-idle': ICON_SCALE_IDLE,
  }

  const writeCut = () => {
    const style = asideRef.current?.style
    if (!style) return
    const cut = cutRef.current
    style.setProperty('--cut-x', `${cut.x}px`)
    style.setProperty('--cut-y', `${cut.y}px`)
    style.setProperty('--cut-w', `${cut.width}px`)
    style.setProperty('--cut-h', `${cut.height}px`)
  }

  // 指示条落位：animate=true 时滑过去，false 时直接放置。
  //
  // 用 getBoundingClientRect 的**差值**，而不是 offsetLeft / offsetTop / offsetWidth：
  // 后者是整数 API，而胶囊高 75.6px —— 取整会让指示条比按钮高出 0.4px，
  // 四个按钮的位置误差还会逐步累积。rect 拿到的是真实布局值，两者像素级重合。
  // （安全：.shell / .sidebar / .sidebar__nav 上都没有 transform，
  //   所以 rect 不会被祖先的变换污染。）
  const placePill = (animate: boolean) => {
    const pill = pillRef.current
    const nav = navRef.current
    const aside = asideRef.current
    const target = buttonRefs.current[active]
    if (!pill || !nav || !aside || !target) return

    const navBox = nav.getBoundingClientRect()
    const asideBox = aside.getBoundingClientRect()
    const box = target.getBoundingClientRect()

    const next = {
      x: box.left - navBox.left,
      y: box.top - navBox.top,
      width: box.width,
      height: box.height,
      opacity: 1,
    }

    // 壳层的「洞」是相对 .sidebar 的 padding box 定位的（壳层就铺在那里）
    const cut = {
      x: box.left - asideBox.left,
      y: box.top - asideBox.top,
      width: box.width,
      height: box.height,
    }

    if (!animate || prefersReducedMotion()) {
      gsap.set(pill, { ...next, autoRound: false })
      Object.assign(cutRef.current, cut)
      writeCut()
      return
    }

    gsap.to(cutRef.current, {
      ...cut,
      duration: durations.navSlide,
      ease: easings.snappy,
      onUpdate: writeCut,
      // 注意：这里**不能**传 autoRound —— 它是 CSSPlugin 的专属属性，
      // 对普通 JS 对象会被 GSAP 当成未知属性，直接报
      // 「Invalid property autoRound ... Missing plugin?」。
      // 普通对象的 x/y/w/h 是纯数字，本来也没有取整问题
    })
    gsap.to(pill, {
      ...next,
      duration: durations.navSlide,
      ease: easings.snappy,
      overwrite: 'auto',
      // GSAP 默认 autoRound: true，会把 px 值舍入到**整数**。
      // 而胶囊高 75.6px —— 舍入后指示条会比按钮高出 0.4px，位置也会差 0.2px。
      // 指示条必须与按钮像素级重合，所以关掉它
      autoRound: false,
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

  return (
    <aside className="glass sidebar" ref={asideRef} style={iconVars}>
      {/* 侧栏的「壳」（描边 + 内高光 + 内反光）。为什么不是面板自己的 box-shadow：
          胶囊比面板大一圈，面板本体上画的框线会从胶囊**内部**穿过去。
          所以壳单独一层，并用一个跟随胶囊的「洞」把胶囊盖住的区域挖掉。
          洞的位置由 placePill 同步写在 --cut-* 上 */}
      <span className="sidebar__edge" aria-hidden="true">
        {/* 假边线被胶囊捏断的那 4 个「头」上的固定亮点。
            放在壳**里面**是为了白捡一层裁剪：壳已经有一个跟随胶囊的洞，
            胶囊盖住的部分自动被挖掉 —— 亮点正好在胶囊口收住。
            不跟光标（光标那圈在壳的 ::after / ::before 上），只看 --cut-* */}
        <span className="sidebar__edge-heads" />
        <span className="sidebar__edge-heads-glow" />
      </span>

      <nav className="sidebar__nav" aria-label="主导航" ref={navRef}>
        <span className="sidebar__pill" ref={pillRef} aria-hidden="true" />
        {items.map((item, index) => (
          <button
            key={item.id}
            type="button"
            className="sidebar__btn"
            aria-current={active === index}
            /* 按钮只剩图标了，标签必须是可读的：屏幕阅读器读 aria-label，
               鼠标用户悬停看 title */
            aria-label={item.label}
            title={item.label}
            ref={(element) => {
              buttonRefs.current[index] = element
            }}
            onClick={() => onSelect(index)}
          >
            <NavIcon paths={NAV_ICON_PATHS[item.id]} />
          </button>
        ))}
      </nav>
    </aside>
  )
}
