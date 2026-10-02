import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import './styles/global.css'

// iOS Safari 默认不给 :active 样式触发（除非页面注册过 touch 监听）。
// 空监听解锁全局的按压反馈（控件缩放，见 global.css 的 :active 规则）
document.addEventListener('touchstart', () => {}, { passive: true })

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
