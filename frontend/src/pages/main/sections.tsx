/**
 * 三个页面的内容。
 * design.md 2.6 说明功能入口暂用 mock 数据，故此处内容为占位文案，待替换。
 */

type IconName = 'terminal' | 'chart' | 'cloud' | 'shield' | 'book' | 'wrench'

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

const STATS = [
  { value: '6', label: '年工程经验' },
  { value: '18', label: '已交付项目' },
  { value: 'Go / TS', label: '主力技术栈' },
  { value: 'UTC+8', label: '所在时区' },
]

const FEATURES: { icon: IconName; title: string; desc: string; tag: string }[] = [
  { icon: 'terminal', title: '在线终端', desc: '把常用脚本收进浏览器，随时执行。', tag: '开发' },
  { icon: 'chart', title: '数据看板', desc: '把散落的指标汇总成一张图。', tag: '分析' },
  { icon: 'cloud', title: '资源托管', desc: '静态资源与文件的分发入口。', tag: '基建' },
  { icon: 'shield', title: '安全工具', desc: '限流、封禁与访问审计。', tag: '安全' },
  { icon: 'book', title: '笔记归档', desc: '长期沉淀的技术笔记索引。', tag: '内容' },
  { icon: 'wrench', title: '实验工坊', desc: '还没定型的小玩意都放这儿。', tag: '实验' },
]

const CONTACTS = [
  { title: '电子邮箱', value: 'hi@example.com', hint: '工作日 24 小时内回复', href: 'mailto:hi@example.com' },
  { title: '代码仓库', value: 'github.com/example', hint: '开源项目与提交记录', href: 'https://github.com' },
  { title: '社交账号', value: '@example', hint: '日常碎碎念', href: undefined },
]

export function IntroSection() {
  return (
    <div className="intro">
      <header className="section__head">
        <span className="section__eyebrow" data-reveal>
          Intro
        </span>
        <h2 className="section__title" data-reveal>
          你好，我是 XR
        </h2>
        <p className="section__desc" data-reveal>
          做后端与前端之间的事：Go 服务、TypeScript 界面，以及把它们可靠地装进容器里。
          这个站点既是名片，也是一些自用小工具的入口。
        </p>
      </header>

      <div className="intro__stats">
        {STATS.map((stat) => (
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
  return (
    <div>
      <header className="section__head">
        <span className="section__eyebrow" data-reveal>
          Features
        </span>
        <h2 className="section__title" data-reveal>
          功能入口
        </h2>
        <p className="section__desc" data-reveal>
          以下为占位内容，接口就绪后会替换成真实入口。
        </p>
      </header>

      <div className="feature-grid">
        {FEATURES.map((feature) => (
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
  return (
    <div>
      <header className="section__head">
        <span className="section__eyebrow" data-reveal>
          Contact
        </span>
        <h2 className="section__title" data-reveal>
          联系方式
        </h2>
        <p className="section__desc" data-reveal>
          本站不提供表单与评论，直接通过下列方式联系即可。
        </p>
      </header>

      <div className="contact">
        {CONTACTS.map((item) => (
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
          <span className="tag">二维码</span>
          <div className="qr">待替换</div>
          <span className="field__hint">放置微信或名片二维码</span>
        </div>
      </div>
    </div>
  )
}
