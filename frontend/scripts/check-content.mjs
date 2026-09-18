import { test } from 'node:test'
import assert from 'node:assert/strict'
import { SITE } from '../src/content/site.ts'

/**
 * 内容校验：把"文案写错"挡在构建之前。
 * 规则只覆盖可机械判定的部分（结构、长度上限、链接形态），不评判文笔。
 */

const FORBIDDEN = [/lorem/i, /TODO/, /XXX/, /待填/, /占位符/]

const sections = [
  ['intro', SITE.intro],
  ['features', SITE.features],
  ['contact', SITE.contact],
  ['more', SITE.more],
]

test('每个板块都有 eyebrow / title / desc', () => {
  for (const [name, section] of sections) {
    for (const key of ['eyebrow', 'title', 'desc']) {
      assert.equal(typeof section[key], 'string', `${name}.${key} 必须是字符串`)
      assert.ok(section[key].trim().length > 0, `${name}.${key} 不能为空`)
    }
    assert.ok(section.desc.length <= 160, `${name}.desc 过长（${section.desc.length} 字，上限 160）`)
  }
})

test('欢迎页文案长度合理', () => {
  assert.ok(SITE.welcome.title.length >= 4 && SITE.welcome.title.length <= 24)
  assert.ok(SITE.welcome.subtitle.length > 0 && SITE.welcome.subtitle.length <= 60)
})

test('数据卡片非空且字段完整', () => {
  assert.ok(SITE.intro.stats.length > 0, '简介页至少一个数据卡片')
  for (const stat of SITE.intro.stats) {
    assert.ok(stat.value.trim().length > 0 && stat.label.trim().length > 0)
  }
})

test('联系方式的链接形态合法', () => {
  assert.ok(SITE.contact.items.length > 0)
  for (const item of SITE.contact.items) {
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
  const text = JSON.stringify(SITE)
  for (const pattern of FORBIDDEN) {
    assert.ok(!pattern.test(text), `内容里出现了未处理的占位标记：${pattern}`)
  }
})

test('仍是占位内容时给出提醒（不算失败）', () => {
  if (SITE.placeholder) {
    console.warn('  ⚠ src/content/site.ts 仍是占位内容，替换真实文案后请把 placeholder 改为 false')
  }
})
