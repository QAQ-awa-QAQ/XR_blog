type Variant = 'drift' | 'pulse' | 'wave'

/** 背景光斑。不同页面用不同动画，配色由时段主题变量驱动。 */
export function Orbs({ variant }: { variant: Variant }) {
  return (
    <div className={`orb-field orb-field--${variant}`} aria-hidden="true">
      <span className="orb orb--a" />
      <span className="orb orb--b" />
      <span className="orb orb--c" />
    </div>
  )
}
