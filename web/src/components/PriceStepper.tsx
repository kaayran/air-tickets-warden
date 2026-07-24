import { Button, Input } from '@telegram-apps/telegram-ui'

const STEP_EUR = 10

export interface PriceStepperProps {
  value: string // whole euros as typed
  error?: string
  onChange: (next: string) => void
}

// PriceStepper is the alert-threshold input: a numeric euro field with ±€10
// steppers. Values stay strings until payload time (money becomes integer
// minor units only at the API edge).
export function PriceStepper({ value, error, onChange }: PriceStepperProps) {
  const current = Number(value) || 0
  const step = (delta: number) => onChange(String(Math.max(0, current + delta)))

  return (
    <Input
      header="Alert when below (€)"
      type="number"
      inputMode="numeric"
      min={0}
      placeholder="150"
      value={value}
      status={error ? 'error' : undefined}
      onChange={(e) => onChange(e.target.value)}
      before={
        <Button size="s" mode="bezeled" onClick={() => step(-STEP_EUR)} aria-label="decrease">
          −{STEP_EUR}
        </Button>
      }
      after={
        <Button size="s" mode="bezeled" onClick={() => step(+STEP_EUR)} aria-label="increase">
          +{STEP_EUR}
        </Button>
      }
    />
  )
}
