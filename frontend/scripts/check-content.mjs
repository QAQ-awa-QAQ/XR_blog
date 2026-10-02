import { test } from 'node:test'
import assert from 'node:assert/strict'
import { DEFAULT_SITE } from '../src/content/site.ts'

/**
 * 内容校验：把"文案写错"挡在构建之前。
 * 校验对象是仓库里的兜底默认值（DEFAULT_SITE）——运行时真实内容在服务端
 * data/site.json，由同一套规则在 Go 侧全量校验（后台保存时也会被拦住）。
 * 这里只覆盖可机械判定的部分（结构、长度上限、链接形态），不评判文笔。
 */

const FORBIDDEN = [/lorem/i, /TODO/, /XXX/, /待填/, /占位符/]

const byType = (type) => DEFAULT_SITE.sections.find((section) => section.type === type)

test('四个渲染器类型齐全且各出现一次', () => {
  const types = DEFAULT_SITE.sections.map((section) => section.type)
  for (const type of ['intro', 'features', 'contact', 'more']) {
    assert.equal(types.filter((t) => t === type).length, 1, `默认值必须包含且只包含一个 ${type} 板块`)
  }
})

test('每个板块都有 eyebrow / title / desc（长度与后端校验一致）', () => {
  for (const section of DEFAULT_SITE.sections) {
    for (const key of ['eyebrow', 'title', 'desc']) {
      const value = section[key]
      assert.equal(typeof value, 'string', `${section.type}.${key} 必须是字符串`)
      assert.ok(value.trim().length > 0, `${section.type}.${key} 不能为空`)
    }
    assert.ok(section.label.trim().length <= 6, `${section.type}.label 过长（上限 6 字）`)
    assert.ok(section.desc.length <= 160, `${section.type}.desc 过长（${section.desc.length} 字，上限 160）`)
  }
})

test('欢迎页文案长度与后端校验一致', () => {
  const { title, subtitle } = DEFAULT_SITE.welcome
  assert.ok(title.length >= 4 && title.length <= 24, '欢迎页标题应为 4-24 字')
  assert.ok(subtitle.length > 0 && subtitle.length <= 60, '欢迎页副标题应为 1-60 字')
})

test('简介板块的数据卡片非空且字段完整', () => {
  const stats = byType('intro').stats ?? []
  assert.ok(stats.length > 0, '简介板块至少一个数据卡片')
  for (const stat of stats) {
    assert.ok(stat.value.trim().length > 0 && stat.label.trim().length > 0)
  }
})

test('联系方式的链接形态合法', () => {
  const items = byType('contact').items ?? []
  assert.ok(items.length > 0)
  for (const item of items) {
    assert.ok(item.title.trim().length > 0 && item.value.trim().length > 0)
    assert.ok(item.hint.trim().length > 0, `${item.title} 缺少说明文字`)

    if (!item.href) continue
    assert.match(
      item.href,
      /^(https?:\/\/|mailto:)/,
      `${item.title} 的链接必须以 http(s):// 或 mailto: 开头：${item.href}`,
    )
    if (item.href.startsWith('mailto:')) {
      assert.match(
        item.href.slice('mailto:'.length),
        /^[^\s@]+@[^\s@]+\.[^\s@]+$/,
        `${item.title} 的邮箱格式不合法：${item.href}`,
      )
    }
  }
})

test('没有遗漏的占位符标记', () => {
  const text = JSON.stringify(DEFAULT_SITE)
  for (const pattern of FORBIDDEN) {
    assert.ok(!pattern.test(text), `内容里出现了未处理的占位标记：${pattern}`)
  }
})
