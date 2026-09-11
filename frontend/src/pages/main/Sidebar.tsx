import type { User } from '../../api/client'

export type SectionId = 'intro' | 'features' | 'contact'

type Props = {
  items: { id: SectionId; label: string }[]
  active: number
  onSelect: (index: number) => void
  user: User
  themeLabel: string
  onLogout: () => void
}

/** 侧栏：由上到下为 简介 / 功能 / 联系（design.md 2.5）。 */
export function Sidebar({ items, active, onSelect, user, themeLabel, onLogout }: Props) {
  return (
    <aside className="glass sidebar">
      <div className="sidebar__brand">XR 个人站</div>

      <nav className="sidebar__nav" aria-label="主导航">
        {items.map((item, index) => (
          <button
            key={item.id}
            type="button"
            className="sidebar__btn"
            aria-current={active === index}
            onClick={() => onSelect(index)}
          >
            <span className="sidebar__dot" aria-hidden="true" />
            {item.label}
          </button>
        ))}
      </nav>

      <div className="sidebar__foot">
        <span title="主题按本地时间自动取色">
          {themeLabel} · {user.nickname}
        </span>
        {user.role === 'admin' ? (
          <a className="sidebar__link" href="/admin">
            管理后台
          </a>
        ) : null}
        <button type="button" className="sidebar__link" onClick={onLogout}>
          退出登录
        </button>
      </div>
    </aside>
  )
}
