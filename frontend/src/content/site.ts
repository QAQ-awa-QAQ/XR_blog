/**
 * 站点文案与内容。
 *
 * 与表现分离：改文案不需要碰组件；`npm run check:content` 会校验结构，
 * 并在仍是占位内容时给出提醒。
 *
 * 注意：本文件被校验脚本以"类型擦除"方式直接导入，请避免使用需要转译的语法
 * （只能出现类型注解、interface/type、as 断言；不要用 enum、装饰器、命名空间）。
 */

export type IconName = 'terminal' | 'chart' | 'cloud' | 'shield' | 'book' | 'wrench'

export type StatItem = { value: string; label: string }

export type FeatureItem = {
  icon: IconName
  title: string
  desc: string
  tag: string
}

export type ContactItem = {
  title: string
  value: string
  hint: string
  href?: string
}

export type SectionCopy = {
  eyebrow: string
  title: string
  desc: string
}

export type SiteContent = {
  /** 全站是否仍在用占位内容 */
  placeholder: boolean
  brand: string
  welcome: {
    title: string
    subtitle: string
  }
  intro: SectionCopy & { stats: StatItem[] }
  features: SectionCopy & { items: FeatureItem[] }
  contact: SectionCopy & {
    items: ContactItem[]
    qr: { label: string; note: string }
  }
}

export const SITE: SiteContent = {
  placeholder: true,
  brand: 'XR 个人站',
  welcome: {
    title: '欢迎访问XR个人站',
    subtitle: '一个关于我、我的作品与联系方式的地方',
  },
  intro: {
    eyebrow: 'Intro',
    title: '你好，我是 XR',
    desc: '做后端与前端之间的事：Go 服务、TypeScript 界面，以及把它们可靠地装进容器里。这个站点既是名片，也是一些自用小工具的入口。',
    stats: [
      { value: '6', label: '年工程经验' },
      { value: '18', label: '已交付项目' },
      { value: 'Go / TS', label: '主力技术栈' },
      { value: 'UTC+8', label: '所在时区' },
    ],
  },
  features: {
    eyebrow: 'Features',
    title: '功能入口',
    desc: '以下为占位内容，接口就绪后会替换成真实入口。',
    items: [
      { icon: 'terminal', title: '在线终端', desc: '把常用脚本收进浏览器，随时执行。', tag: '开发' },
      { icon: 'chart', title: '数据看板', desc: '把散落的指标汇总成一张图。', tag: '分析' },
      { icon: 'cloud', title: '资源托管', desc: '静态资源与文件的分发入口。', tag: '基建' },
      { icon: 'shield', title: '安全工具', desc: '限流、封禁与访问审计。', tag: '安全' },
      { icon: 'book', title: '笔记归档', desc: '长期沉淀的技术笔记索引。', tag: '内容' },
      { icon: 'wrench', title: '实验工坊', desc: '还没定型的小玩意都放这儿。', tag: '实验' },
    ],
  },
  contact: {
    eyebrow: 'Contact',
    title: '联系方式',
    desc: '本站不提供表单与评论，直接通过下列方式联系即可。',
    items: [
      { title: '电子邮箱', value: 'hi@example.com', hint: '工作日 24 小时内回复', href: 'mailto:hi@example.com' },
      { title: '代码仓库', value: 'github.com/example', hint: '开源项目与提交记录', href: 'https://github.com' },
      { title: '社交账号', value: '@example', hint: '日常碎碎念' },
    ],
    qr: { label: '二维码', note: '待替换' },
  },
}
