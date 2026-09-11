import { SITE, type IconName } from '../../content/site'
import type { User } from '../../api/client'

/** 「更多」页需要的东西：当前账户与退出动作 */
type MoreProps = {
  user: User
  themeLabel: string
  accent: string
  onLogout: () => void
}

/**
 * 三个页面的内容。
 *
 * 文案统一来自 src/content/site.ts（改文案不必碰组件）；
 * 这里只负责把数据渲染成 DOM，以及给每个条目打上 data-reveal
 * ——顺序即波包式入场的距离顺序。
 */

const ICON_PATHS: Record<IconName, string> = {
  terminal: 'M4 7l4 5-4 5M12 17h8',
  chart: 'M4 19V5M4 19h16M8 19v-6M12 19V9M16 19v-3',
  cloud: 'M7 18h9a4 4 0 0 0 .6-7.96A5.5 5.5 0 0 0 6.5 9.5A4.25 4.25 0 0 0 7 18Z',
  shield: 'M12 3l7 3v6c0 4.5-3 7.6-7 9-4-1.4-7-4.5-7-9V6z',
  book: 'M4 5.5A2.5 2.5 0 0 1 6.5 3H19v15H6.5A2.5 2.5 0 0 0 4 20.5z',
  wrench: 'M14.5 4.5a4.5 4.5 0 0 0-6 6L4 15v3h3l4.5-4.5a4.5 4.5 0 0 0 6-6l-2.5 2.5-2.5-2.5z',
}

function Icon({ name }: { name: IconName }) {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={ICON_PATHS[name]} />
    </svg>
  )
}

export function IntroSection() {
  const { intro } = SITE

  return (
    <div className="intro">
      <header className="section__head">
        <span className="section__eyebrow" data-reveal>
          {intro.eyebrow}
        </span>
        <h2 className="section__title" data-reveal>
          {intro.title}
        </h2>
        <p className="section__desc" data-reveal>
          {intro.desc}
        </p>
      </header>

      <div className="intro__stats">
        {intro.stats.map((stat) => (
          <div className="glass stat" key={stat.label} data-reveal>
            <span className="stat__value">{stat.value}</span>
            <span className="stat__label">{stat.label}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

export function FeaturesSection() {
  const { features } = SITE

  return (
    <div>
      <header className="section__head">
        <span className="section__eyebrow" data-reveal>
          {features.eyebrow}
        </span>
        <h2 className="section__title" data-reveal>
          {features.title}
        </h2>
        <p className="section__desc" data-reveal>
          {features.desc}
        </p>
      </header>

      <div className="feature-grid">
        {features.items.map((feature) => (
          <article className="glass feature-card" key={feature.title} data-reveal>
            <span className="feature-card__icon">
              <Icon name={feature.icon} />
            </span>
            <h3 className="feature-card__title">{feature.title}</h3>
            <p className="feature-card__desc">{feature.desc}</p>
            <span className="tag">{feature.tag}</span>
          </article>
        ))}
      </div>
    </div>
  )
}

export function ContactSection() {
  const { contact } = SITE

  return (
    <div>
      <header className="section__head">
        <span className="section__eyebrow" data-reveal>
          {contact.eyebrow}
        </span>
        <h2 className="section__title" data-reveal>
          {contact.title}
        </h2>
        <p className="section__desc" data-reveal>
          {contact.desc}
        </p>
      </header>

      <div className="contact">
        {contact.items.map((item) => (
          <div className="glass contact__card" key={item.title} data-reveal>
            <span className="tag">{item.title}</span>
            {item.href ? (
              <a className="contact__value" href={item.href}>
                {item.value}
              </a>
            ) : (
              <span className="contact__value">{item.value}</span>
            )}
            <span className="field__hint">{item.hint}</span>
          </div>
        ))}

        <div className="glass contact__card" data-reveal>
          <span className="tag">{contact.qr.label}</span>
          <div className="qr">{contact.qr.note}</div>
          <span className="field__hint">放置微信或名片二维码</span>
        </div>
      </div>
    </div>
  )
}

/** 「更多」页：站名、当前账户，以及两个动作（设计稿里“其他信息”的安置处） */
export function MoreSection({ user, themeLabel, accent, onLogout }: MoreProps) {
  const { more } = SITE

  return (
    <div>
      <header className="section__head">
        <span className="section__eyebrow" data-reveal>
          {more.eyebrow}
        </span>
        <h2 className="section__title" data-reveal>
          {more.title}
        </h2>
        <p className="section__desc" data-reveal>
          {more.desc}
        </p>
      </header>

      <div className="more">
        <div className="glass more__card" data-reveal>
          <span className="tag">本站</span>
          <span className="more__brand">{SITE.brand}</span>
          <span className="field__hint">{SITE.welcome.subtitle}</span>
        </div>

        <div className="glass more__card" data-reveal>
          <span className="tag">账户</span>
          <span className="more__identity" title={`主题色 ${accent}（随本地时间连续过渡）`}>
            <span className="more__swatch" style={{ background: accent }} aria-hidden="true" />
            <span className="more__who">
              {themeLabel} · {user.nickname}
            </span>
          </span>
          <span className="field__hint">
            {user.role === 'admin' ? '管理员：可进入后台管理内容' : '普通用户'}
          </span>
        </div>
      </div>

      <div className="more__actions" data-reveal>
        {user.role === 'admin' ? (
          <a className="btn btn--glass" href="/admin">
            管理后台
          </a>
        ) : null}
        <button type="button" className="btn btn--ghost" onClick={onLogout}>
          退出登录
        </button>
      </div>
    </div>
  )
}
