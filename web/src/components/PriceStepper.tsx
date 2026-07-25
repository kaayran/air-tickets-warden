const STEP_EUR = 10

export interface PriceStepperProps {
  value: string // whole euros as typed
  error?: string
  onChange: (next: string) => void
}

// PriceStepper is the alert-threshold input in chart grammar: a tabular euro
// field flanked by ±€10 chart controls. The live value is announced to screen
// readers via aria-live. Money stays a string until payload time.
export function PriceStepper({ value, error, onChange }: PriceStepperProps) {
  const current = Number(value) || 0
  const step = (delta: number) => onChange(String(Math.max(0, current + delta)))

  return (
    <div>
      <div className="stepper">
        <button type="button" className="stepper__btn" onClick={() => step(-STEP_EUR)} aria-label={`Decrease by ${STEP_EUR} euro`}>
          −
        </button>
        <input
          className={`input input--data${error ? ' input--error' : ''}`}
          type="number"
          inputMode="numeric"
          min={0}
          placeholder="150"
          value={value}
          aria-label="Alert threshold in euro"
          onChange={(e) => onChange(e.target.value)}
        />
        <button type="button" className="stepper__btn" onClick={() => step(+STEP_EUR)} aria-label={`Increase by ${STEP_EUR} euro`}>
          +
        </button>
      </div>
      <span aria-live="polite" style={{ position: 'absolute', width: 1, height: 1, overflow: 'hidden', clip: 'rect(0 0 0 0)' }}>
        {current > 0 ? `€${current}` : 'no threshold set'}
      </span>
    </div>
  )
}
