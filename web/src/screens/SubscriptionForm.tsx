import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, createSubscription, patchSubscription, type Subscription } from '../api'
import { AirportPicker } from '../components/AirportPicker'
import { PriceStepper } from '../components/PriceStepper'
import { Button, Field, FieldGroup, SectionLabel, ToggleRow } from '../components/chart/ui'
import { RouteLine } from '../components/chart/RouteLine'
import { formatDateRange } from '../lib/format'
import {
  addDays,
  applyServerErrors,
  emptyForm,
  firstInvalidStep,
  fromSubscription,
  stepErrors,
  toCreatePayload,
  toPatchPayload,
  todayISO,
  validate,
  wizardSteps,
  type FormErrors,
  type SubscriptionFormValues,
  type WizardStep,
} from '../lib/subscriptionForm'

const STEP_NAMES: Record<WizardStep, string> = {
  from: 'Departure',
  to: 'Destination',
  dates: 'Dates',
  alert: 'Alert',
  summary: 'Confirm',
}

export interface SubscriptionFormProps {
  /** Present when editing an existing watch. */
  edit?: Subscription
  /** Present when duplicating: seed the fields but create a new watch. */
  seed?: Subscription
  onDone: () => void
}

// SubscriptionForm is the flight-plan wizard: Departure, Destination, Dates,
// Alert, and a Confirm page. Each page validates only its own fields on Next;
// server field errors map back onto the inputs and jump to the offending step.
// It creates, edits (edit), or duplicates (seed) a watch.
export function SubscriptionForm({ edit, seed, onDone }: SubscriptionFormProps) {
  const queryClient = useQueryClient()
  const source = edit ?? seed
  const [values, setValues] = useState<SubscriptionFormValues>(source ? fromSubscription(source) : emptyForm)
  const [step, setStep] = useState<WizardStep>('from')
  const [errors, setErrors] = useState<FormErrors>({})
  const [serverError, setServerError] = useState<string | null>(null)
  const [withAlternatives, setWithAlternatives] = useState((source?.origin_alternatives.length ?? 0) > 0)
  const [withExtraDestinations, setWithExtraDestinations] = useState((source?.destinations.length ?? 0) > 1)

  const stepIndex = wizardSteps.indexOf(step)

  const set = <K extends keyof SubscriptionFormValues>(key: K, value: SubscriptionFormValues[K]) => {
    setValues((v) => ({ ...v, [key]: value }))
    setErrors((e) => ({ ...e, [key]: undefined }))
  }

  const saveMutation = useMutation({
    mutationFn: () =>
      edit ? patchSubscription(edit.id, toPatchPayload(values, edit)) : createSubscription(toCreatePayload(values)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['subscriptions'] })
      onDone()
    },
    onError: (err) => {
      if (err instanceof ApiError && err.fields.length > 0) {
        const { errors: mapped, step: failing, unmapped } = applyServerErrors(err.fields)
        setErrors(mapped)
        if (failing) setStep(failing)
        setServerError(unmapped.length > 0 ? unmapped.join('; ') : null)
      } else {
        setServerError(err instanceof Error ? err.message : 'Request failed')
      }
    },
  })

  const next = () => {
    if (step === 'summary') {
      confirm()
      return
    }
    const errs = stepErrors(step, validate(values))
    setErrors(errs)
    if (Object.keys(errs).length > 0) return
    setStep(wizardSteps[stepIndex + 1])
  }

  const back = () => {
    if (stepIndex === 0) onDone()
    else setStep(wizardSteps[stepIndex - 1])
  }

  const confirm = () => {
    setServerError(null)
    const errs = validate(values)
    setErrors(errs)
    const failing = firstInvalidStep(errs)
    if (failing) {
      setStep(failing)
      return
    }
    if (edit && Object.keys(toPatchPayload(values, edit)).length === 0) {
      onDone()
      return
    }
    saveMutation.mutate()
  }

  const toggleRoundTrip = (on: boolean) => {
    setValues((v) => {
      if (on && v.returnDateFrom === '' && v.dateTo !== '') {
        return { ...v, roundTrip: on, returnDateFrom: v.dateTo, returnDateTo: addDays(v.dateTo, 7) }
      }
      return { ...v, roundTrip: on }
    })
    setErrors((e) => ({ ...e, returnDateFrom: undefined, returnDateTo: undefined }))
  }

  const today = todayISO()
  const primaryDestination = values.destinations.slice(0, 1)
  const extraDestinations = values.destinations.slice(1)

  return (
    <>
      <div className="screen screen--wizard">
        {/* Labeled step scale — never bare dots. */}
        <div className="stepscale">
          <div className="stepscale__head">
            <span className="stepscale__name">{STEP_NAMES[step]}</span>
            <span className="stepscale__count">
              STEP {stepIndex + 1} / {wizardSteps.length}
            </span>
          </div>
          <div className="stepscale__track">
            {wizardSteps.map((s, i) => (
              <button
                key={s}
                type="button"
                aria-label={`Go to ${STEP_NAMES[s]}`}
                className={`stepscale__seg stepscale__seg--btn${i <= stepIndex ? ' stepscale__seg--done' : ''}`}
                onClick={() => i < stepIndex && setStep(s)}
                disabled={i > stepIndex}
              />
            ))}
          </div>
        </div>

        {step === 'from' && (
          <div className="stack">
            <FieldGroup>
              <AirportPicker
                label="Where do you fly from?"
                placeholder="City or IATA code"
                selected={values.origin ? [values.origin] : []}
                max={1}
                error={errors.origin}
                onChange={(codes) => set('origin', codes[0] ?? '')}
              />
              <ToggleRow
                checked={withAlternatives}
                onChange={(on) => {
                  setWithAlternatives(on)
                  if (!on) set('originAlternatives', [])
                }}
              >
                Add alternative departure airports
              </ToggleRow>
              {withAlternatives && (
                <AirportPicker
                  label="Alternative departures"
                  placeholder="Nearby airports"
                  selected={values.originAlternatives}
                  max={5}
                  exclude={values.origin ? [values.origin] : []}
                  onChange={(codes) => set('originAlternatives', codes)}
                />
              )}
            </FieldGroup>
          </div>
        )}

        {step === 'to' && (
          <div className="stack">
            <FieldGroup>
              <AirportPicker
                label="Where do you fly to?"
                placeholder="City or IATA code"
                selected={primaryDestination}
                max={1}
                exclude={[values.origin, ...extraDestinations].filter(Boolean)}
                error={errors.destinations}
                onChange={(codes) => set('destinations', [...codes, ...extraDestinations])}
              />
              <ToggleRow
                checked={withExtraDestinations}
                onChange={(on) => {
                  setWithExtraDestinations(on)
                  if (!on) set('destinations', primaryDestination)
                }}
              >
                Watch more destinations
              </ToggleRow>
              {withExtraDestinations && (
                <AirportPicker
                  label="More destinations"
                  placeholder="Watch several cities at once"
                  selected={extraDestinations}
                  max={9}
                  exclude={[values.origin, ...primaryDestination].filter(Boolean)}
                  onChange={(codes) => set('destinations', [...primaryDestination, ...codes])}
                />
              )}
            </FieldGroup>
          </div>
        )}

        {step === 'dates' && (
          <div className="stack">
            <SectionLabel>Departure window</SectionLabel>
            <FieldGroup>
              <Field label="Earliest" error={errors.dateFrom}>
                <input
                  className={`input input--data${errors.dateFrom ? ' input--error' : ''}`}
                  type="date"
                  min={today}
                  value={values.dateFrom}
                  onChange={(e) => set('dateFrom', e.target.value)}
                />
              </Field>
              <Field label="Latest" error={errors.dateTo}>
                <input
                  className={`input input--data${errors.dateTo ? ' input--error' : ''}`}
                  type="date"
                  min={values.dateFrom || today}
                  value={values.dateTo}
                  onChange={(e) => set('dateTo', e.target.value)}
                />
              </Field>
            </FieldGroup>
            <FieldGroup>
              <ToggleRow variant="switch" checked={values.roundTrip} onChange={toggleRoundTrip}>
                Round trip
              </ToggleRow>
              {values.roundTrip && (
                <>
                  <Field label="Return earliest" error={errors.returnDateFrom}>
                    <input
                      className={`input input--data${errors.returnDateFrom ? ' input--error' : ''}`}
                      type="date"
                      min={values.dateFrom || today}
                      value={values.returnDateFrom}
                      onChange={(e) => set('returnDateFrom', e.target.value)}
                    />
                  </Field>
                  <Field label="Return latest" error={errors.returnDateTo}>
                    <input
                      className={`input input--data${errors.returnDateTo ? ' input--error' : ''}`}
                      type="date"
                      min={values.returnDateFrom || values.dateFrom || today}
                      value={values.returnDateTo}
                      onChange={(e) => set('returnDateTo', e.target.value)}
                    />
                  </Field>
                </>
              )}
            </FieldGroup>
          </div>
        )}

        {step === 'alert' && (
          <div className="stack">
            <SectionLabel>When should we alert you?</SectionLabel>
            <FieldGroup>
              <Field
                label="Alert type"
                help={
                  values.alertStrategy === 'historical_minimum'
                    ? 'Alerts when the fare undercuts the route’s own observed low. The first days just collect history — no threshold needed.'
                    : 'Alerts when the fare drops below a price you set.'
                }
              >
                <select
                  className="select"
                  value={values.alertStrategy}
                  onChange={(e) => set('alertStrategy', e.target.value)}
                >
                  <option value="absolute_threshold">Below a fixed price</option>
                  <option value="historical_minimum">Significant price drop</option>
                </select>
              </Field>
              {values.alertStrategy === 'absolute_threshold' && (
                <Field label="Alert below (€)" error={errors.maxPriceEur}>
                  <PriceStepper value={values.maxPriceEur} error={errors.maxPriceEur} onChange={(v) => set('maxPriceEur', v)} />
                </Field>
              )}
              <Field label="Max stops">
                <select className="select" value={values.maxStops} onChange={(e) => set('maxStops', e.target.value)}>
                  <option value="">Any</option>
                  <option value="0">Direct only</option>
                  <option value="1">Up to 1</option>
                  <option value="2">Up to 2</option>
                </select>
              </Field>
            </FieldGroup>
          </div>
        )}

        {step === 'summary' && (
          <div className="stack">
            <SectionLabel>Check and confirm</SectionLabel>
            <div className="callout">
              <RouteLine
                origin={values.origin}
                originAlternatives={values.originAlternatives}
                destinations={values.destinations}
              />
              <dl style={{ margin: '12px 0 0', display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '6px 16px' }}>
                <dt className="chart-label">Depart</dt>
                <dd className="data t-sub" style={{ margin: 0 }}>
                  {values.dateFrom && values.dateTo ? formatDateRange(values.dateFrom, values.dateTo) : '—'}
                </dd>
                <dt className="chart-label">Return</dt>
                <dd className="data t-sub" style={{ margin: 0 }}>
                  {values.roundTrip && values.returnDateFrom && values.returnDateTo
                    ? formatDateRange(values.returnDateFrom, values.returnDateTo)
                    : 'One-way'}
                </dd>
                <dt className="chart-label">Alert</dt>
                <dd className="data t-sub" style={{ margin: 0 }}>
                  {values.alertStrategy === 'absolute_threshold'
                    ? `Below €${values.maxPriceEur || '—'}`
                    : 'On a significant price drop'}
                </dd>
                <dt className="chart-label">Stops</dt>
                <dd className="data t-sub" style={{ margin: 0 }}>
                  {values.maxStops === '' ? 'Any' : values.maxStops === '0' ? 'Direct only' : `Up to ${values.maxStops}`}
                </dd>
              </dl>
              <p className="await-note">
                The warden checks several live sources on a schedule and messages you in Telegram when the alert fires.
                Wizz Air isn’t covered yet.
              </p>
            </div>
            {serverError && (
              <div className="notice" role="alert">
                {serverError}
              </div>
            )}
          </div>
        )}
      </div>

      <div className="actionbar">
        <Button variant="secondary" onClick={back}>
          {stepIndex === 0 ? 'Cancel' : 'Back'}
        </Button>
        <Button variant="primary" className="btn--advance" disabled={saveMutation.isPending} onClick={next}>
          {step === 'summary' ? (edit ? 'Save changes' : 'File watch') : 'Next'}
        </Button>
      </div>
    </>
  )
}
