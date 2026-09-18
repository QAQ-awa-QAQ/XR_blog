import { useEffect, useId, type InputHTMLAttributes, type ReactNode } from 'react'

type FieldProps = InputHTMLAttributes<HTMLInputElement> & {
  label: string
  hint?: string
}

/** 显式标签 + 就近提示，不用 placeholder 充当 label（UX 准则 #8）。 */
export function Field({ label, hint, ...inputProps }: FieldProps) {
  const id = useId()
  return (
    <div className="field">
      <label className="field__label" htmlFor={id}>
        {label}
      </label>
      <input id={id} className="field__input" {...inputProps} />
      {hint ? <span className="field__hint">{hint}</span> : null}
    </div>
  )
}

export function ErrorBanner({ message, tone = 'error' }: { message: string; tone?: 'error' | 'warn' }) {
  if (!message) return null
  return (
    <div className={`banner banner--${tone}`} role="alert">
      {message}
    </div>
  )
}

export function Spinner({ label = '加载中' }: { label?: string }) {
  return (
    <div className="center-screen">
      <span className="spinner" />
      <span>{label}</span>
    </div>
  )
}

/** 轻量模态：遮罩 + 玻璃卡片。点击遮罩或按 Esc 关闭。 */
export function Modal({
  title,
  onClose,
  children,
  footer,
}: {
  title: string
  onClose: () => void
  children?: ReactNode
  /** 自定义操作区；不传时给一个「知道了」按钮 */
  footer?: ReactNode
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div className="modal" role="dialog" aria-modal="true" aria-label={title}>
      <div className="modal__backdrop" onClick={onClose} />
      <div className="glass modal__card">
        <h3 className="modal__title">{title}</h3>
        {children ? <div className="modal__body">{children}</div> : null}
        <div className="modal__actions">
          {footer ?? (
            <button type="button" className="btn btn--primary btn--sm" onClick={onClose}>
              知道了
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
