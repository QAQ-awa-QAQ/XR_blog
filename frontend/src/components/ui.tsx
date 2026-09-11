import { useId, type InputHTMLAttributes } from 'react'

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
