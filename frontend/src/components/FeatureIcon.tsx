import { FEATURE_ICON_PATHS, type FeatureIconName } from '../design/featureIcons'

/**
 * 功能图标渲染组件：渲染口径（24 视角框 / stroke 1.7 / 圆头）与导航图标一致。
 * 图标名来自后端数据（白名单校验过），未知时回退到 terminal。
 */
export function FeatureIcon({ name, size = 20 }: { name: string; size?: number }) {
  const paths = FEATURE_ICON_PATHS[name as FeatureIconName] ?? FEATURE_ICON_PATHS.terminal
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
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
