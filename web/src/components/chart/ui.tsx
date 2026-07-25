// Chart-world UI primitives. Presentational only — they draw the aeronautical
// chart surfaces (callouts, framed fields, buttons) from the token layer.
import type { ButtonHTMLAttributes, ReactNode } from 'react'

export function Screen({ children, wizard }: { children: ReactNode; wizard?: boolean }) {
  return <div className={wizard ? 'screen screen--wizard' : 'screen'}>{children}</div>
}

export function Masthead({ title, meta }: { title: string; meta?: ReactNode }) {
  return (
    <div className="masthead">
      <h1 className="masthead__title">{title}</h1>
      {meta != null && <span className="masthead__meta data">{meta}</span>}
    </div>
  )
}

export function SectionLabel({ children }: { children: ReactNode }) {
  return <span className="chart-label">{children}</span>
}

export function Callout({
  children,
  triggered,
  onClick,
  ariaLabel,
}: {
  children: ReactNode
  triggered?: boolean
  onClick?: () => void
  ariaLabel?: string
}) {
  const cls = `callout${triggered ? ' callout--triggered' : ''}`
  if (onClick) {
    return (
      <button type="button" className={`${cls} callout--tappable`} onClick={onClick} aria-label={ariaLabel}>
        {children}
      </button>
    )
  }
  return <div className={cls}>{children}</div>
}

export function FieldGroup({ children }: { children: ReactNode }) {
  return <div className="field-group">{children}</div>
}

export function Field({
  label,
  htmlFor,
  help,
  error,
  children,
}: {
  label?: string
  htmlFor?: string
  help?: ReactNode
  error?: string
  children: ReactNode
}) {
  return (
    <div className="field">
      {label && (
        <label className="field__label" htmlFor={htmlFor}>
          {label}
        </label>
      )}
      {children}
      {error ? <div className="field__error">{error}</div> : help ? <div className="field__help">{help}</div> : null}
    </div>
  )
}

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'ghost'
  block?: boolean
  sm?: boolean
}

export function Button({ variant = 'secondary', block, sm, className = '', ...rest }: ButtonProps) {
  const cls = [
    'btn',
    `btn--${variant}`,
    block ? 'btn--block' : '',
    sm ? 'btn--sm' : '',
    className,
  ]
    .filter(Boolean)
    .join(' ')
  return <button type="button" className={cls} {...rest} />
}

export function ToggleRow({
  checked,
  onChange,
  children,
  variant = 'check',
}: {
  checked: boolean
  onChange: (next: boolean) => void
  children: ReactNode
  variant?: 'check' | 'switch'
}) {
  return (
    <label className="toggle-row">
      <span className="toggle-row__text">{children}</span>
      {variant === 'switch' ? (
        <span className="switch">
          <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
          <span className="switch__track" />
          <span className="switch__knob" />
        </span>
      ) : (
        <>
          <input
            type="checkbox"
            checked={checked}
            onChange={(e) => onChange(e.target.checked)}
            style={{ position: 'absolute', opacity: 0, width: 0, height: 0 }}
          />
          <span className={`checkbox${checked ? ' checkbox--on' : ''}`} aria-hidden="true">
            {checked ? '×' : ''}
          </span>
        </>
      )}
    </label>
  )
}
