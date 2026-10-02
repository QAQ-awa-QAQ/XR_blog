/**
 * 站点内容的类型契约与本地默认值。
 *
 * 运行时的唯一数据源是服务端 data/site.json：
 *   - 直接改该文件（热加载，无需重启服务）；
 *   - 或在管理后台「站点配置」里可视化编辑。
 * 前端启动时经 /api/site 拉取；本文件只在接口不可用时兜底，
 * 所以默认值必须保持占位内容 —— 真实个人信息不要写进仓库。
 *
 * 注意：本文件被校验脚本以"类型擦除"方式直接导入，请避免使用需要转译的语法
 * （只能出现类型注解、interface/type、as 断言；不要用 enum、装饰器、命名空间）。
 */

export type SectionType = 'intro' | 'features' | 'contact' | 'more'

export type StatItem = { value: string; label: string }

export type ContactItem = {
  title: string
  value: string
  hint: string
  href?: string
}

export type MoreSettings = {
  follow: { name: string; hint: string }
  slot: { name: string; hintAuto: string; hintManual: string }
}

export type SectionConfig = {
  type: SectionType
  /** 侧栏按钮 / 分页器的名字（1-6 字，动作口径） */
  label: string
  visible: boolean
  eyebrow: string
  title: string
  desc: string
  /** 仅 intro 使用：数据卡 */
  stats?: StatItem[]
  /** 仅 contact 使用：联系方式条目 */
  items?: ContactItem[]
  /** 仅 more 使用：账户设置区的文案 */
  settings?: MoreSettings
}

export type SiteConfig = {
  version: number
  brand: string
  welcome: { title: string; subtitle: string }
  sections: SectionConfig[]
}

/** 「更多」页设置区的默认文案（缺省时会由服务端补齐；这里兼作类型兜底） */
export const DEFAULT_MORE_SETTINGS: MoreSettings = {
  follow: {
    name: '跟随时间',
    hint: '每分钟按本地时间重新取色，背景连续过渡，不会到点突跳。',
  },
  slot: {
    name: '主题时段',
    hintAuto: '跟随时间中 · 当前',
    hintManual: '已固定为',
  },
}

/** 全占位的兜底默认值：不含任何真实个人信息，可安全进入公开仓库 */
export const DEFAULT_SITE: SiteConfig = {
  version: 1,
  brand: 'XR 个人站',
  welcome: {
    title: '欢迎访问XR个人站',
    subtitle: '一个关于我、我的作品与联系方式的地方',
  },
  sections: [
    {
      type: 'intro',
      label: '首页',
      visible: true,
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
    {
      type: 'features',
      label: '功能',
      visible: true,
      eyebrow: 'Features',
      title: '功能入口',
      desc: '以下为占位内容，接口就绪后会替换成真实入口。',
    },
    {
      type: 'contact',
      label: '联系',
      visible: true,
      eyebrow: 'Contact',
      title: '联系方式',
      desc: '本站不提供表单与评论，直接通过下列方式联系即可。',
      items: [
        { title: '电子邮箱', value: 'hi@example.com', hint: '工作日 24 小时内回复', href: 'mailto:hi@example.com' },
        { title: '代码仓库', value: 'github.com/example', hint: '开源项目与提交记录', href: 'https://github.com' },
      ],
    },
    {
      type: 'more',
      label: '更多',
      visible: true,
      eyebrow: 'More',
      title: '更多',
      desc: '本站的基本信息与当前账户。主题色默认跟随本地时间连续过渡，也可以在这里手动指定时段。',
      settings: DEFAULT_MORE_SETTINGS,
    },
  ],
}
