import { createContext, useContext } from 'react'
import { DEFAULT_SITE, type SiteConfig } from './site'

/** 站点内容（由 App 顶层拉取 /api/site 后注入；初始值为兜底默认值） */
export const SiteContext = createContext<SiteConfig>(DEFAULT_SITE)

/** 读取当前站点内容配置 */
export function useSite(): SiteConfig {
  return useContext(SiteContext)
}
