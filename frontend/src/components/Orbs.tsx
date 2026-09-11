import { useOrbField } from '../motion/useOrbField'
import type { OrbVariant } from '../motion/orbMotion'

/** 背景光斑。运动由 useOrbField 按页面变体驱动，配色由时段主题变量提供。 */
export function Orbs({ variant }: { variant: OrbVariant }) {
  const hostRef = useOrbField(variant)

  return (
    <div className="orb-field" ref={hostRef} aria-hidden="true">
      <span className="orb orb--a" data-orb />
      <span className="orb orb--b" data-orb />
      <span className="orb orb--c" data-orb />
    </div>
  )
}
