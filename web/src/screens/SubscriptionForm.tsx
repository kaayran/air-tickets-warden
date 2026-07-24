import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Banner,
  Button,
  Cell,
  Checkbox,
  FixedLayout,
  Input,
  List,
  Section,
  Select,
  Steps,
  Switch,
} from '@telegram-apps/telegram-ui'
import { ApiError, createSubscription, patchSubscription, type Subscription } from '../api'
import { AirportPicker } from '../components/AirportPicker'
import { PriceStepper } from '../components/PriceStepper'
import { formatDateRange } from '../lib/format'
import {
  addDays,
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

export interface SubscriptionFormProps {
  /** Present when editing; absent when creating. */
  initial?: Subscription
  onDone: () => void
}

// SubscriptionForm is a five-page wizard: From, To, Dates, Alert, and a
// summary with the final confirm. Each page validates only its own fields on
// "Next"; the server stays the authority and its field errors surface on the
// summary. On edit, only changed fields are PATCHed — fields the wizard does
// not expose are never clobbered.
export function SubscriptionForm({ initial, onDone }: SubscriptionFormProps) {
  const queryClient = useQueryClient()
  const [values, setValues] = useState<SubscriptionFormValues>(
    initial ? fromSubscription(initial) : emptyForm,
  )
  const [step, setStep] = useState<WizardStep>('from')
  const [errors, setErrors] = useState<FormErrors>({})
  const [serverError, setServerError] = useState<string | null>(null)
  const [withAlternatives, setWithAlternatives] = useState(
    (initial?.origin_alternatives.length ?? 0) > 0,
  )
  const [withExtraDestinations, setWithExtraDestinations] = useState(
    (initial?.destinations.length ?? 0) > 1,
  )

  const stepIndex = wizardSteps.indexOf(step)

  const set = <K extends keyof SubscriptionFormValues>(key: K, value: SubscriptionFormValues[K]) => {
    setValues((v) => ({ ...v, [key]: value }))
    setErrors((e) => ({ ...e, [key]: undefined }))
  }

  const saveMutation = useMutation({
    mutationFn: () =>
      initial
        ? patchSubscription(initial.id, toPatchPayload(values, initial))
        : createSubscription(toCreatePayload(values)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['subscriptions'] })
      onDone()
    },
    onError: (err) => {
      if (err instanceof ApiError && err.fields.length > 0) {
        setServerError(err.fields.map((f) => `${f.field}: ${f.message}`).join('; '))
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
    if (stepIndex === 0) {
      onDone()
      return
    }
    setStep(wizardSteps[stepIndex - 1])
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
    if (initial && Object.keys(toPatchPayload(values, initial)).length === 0) {
      onDone() // nothing changed
      return
    }
    saveMutation.mutate()
  }

  // Round-trip toggle: on first enable, prefill the return window starting at
  // the last departure date.
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
      <div style={{ paddingBottom: 96 }}>
        <List>
          <Section>
            <div style={{ padding: '12px 16px' }}>
              <Steps count={wizardSteps.length} progress={stepIndex} />
            </div>
          </Section>

          {step === 'from' && (
            <>
              <AirportPicker
                header="Where do you fly from?"
                placeholder="City or IATA code"
                selected={values.origin ? [values.origin] : []}
                max={1}
                error={errors.origin}
                onChange={(codes) => set('origin', codes[0] ?? '')}
              />
              <Section>
                <Cell
                  Component="label"
                  before={
                    <Checkbox
                      checked={withAlternatives}
                      onChange={(e) => {
                        setWithAlternatives(e.target.checked)
                        if (!e.target.checked) set('originAlternatives', [])
                      }}
                    />
                  }
                  multiline
                >
                  Add alternative departure airports
                </Cell>
              </Section>
              {withAlternatives && (
                <AirportPicker
                  header="Alternative departures"
                  placeholder="e.g. nearby airports"
                  selected={values.originAlternatives}
                  max={5}
                  exclude={values.origin ? [values.origin] : []}
                  onChange={(codes) => set('originAlternatives', codes)}
                />
              )}
            </>
          )}

          {step === 'to' && (
            <>
              <AirportPicker
                header="Where do you fly to?"
                placeholder="City or IATA code"
                selected={primaryDestination}
                max={1}
                exclude={[values.origin, ...extraDestinations].filter(Boolean)}
                error={errors.destinations}
                onChange={(codes) => set('destinations', [...codes, ...extraDestinations])}
              />
              <Section>
                <Cell
                  Component="label"
                  before={
                    <Checkbox
                      checked={withExtraDestinations}
                      onChange={(e) => {
                        setWithExtraDestinations(e.target.checked)
                        if (!e.target.checked) set('destinations', primaryDestination)
                      }}
                    />
                  }
                  multiline
                >
                  Add more destinations
                </Cell>
              </Section>
              {withExtraDestinations && (
                <AirportPicker
                  header="More destinations"
                  placeholder="Watch several cities at once"
                  selected={extraDestinations}
                  max={9}
                  exclude={[values.origin, ...primaryDestination].filter(Boolean)}
                  onChange={(codes) => set('destinations', [...primaryDestination, ...codes])}
                />
              )}
            </>
          )}

          {step === 'dates' && (
            <>
              <Section header="Departure window" footer={errors.dateFrom ?? errors.dateTo}>
                <Input
                  header="From"
                  type="date"
                  min={today}
                  value={values.dateFrom}
                  status={errors.dateFrom ? 'error' : undefined}
                  onChange={(e) => set('dateFrom', e.target.value)}
                />
                <Input
                  header="To"
                  type="date"
                  min={values.dateFrom || today}
                  value={values.dateTo}
                  status={errors.dateTo ? 'error' : undefined}
                  onChange={(e) => set('dateTo', e.target.value)}
                />
              </Section>
              <Section footer={errors.returnDateFrom ?? errors.returnDateTo}>
                <Cell
                  Component="label"
                  after={<Switch checked={values.roundTrip} onChange={(e) => toggleRoundTrip(e.target.checked)} />}
                  multiline
                >
                  Round trip
                </Cell>
                {values.roundTrip && (
                  <>
                    <Input
                      header="Return from"
                      type="date"
                      min={values.dateFrom || today}
                      value={values.returnDateFrom}
                      status={errors.returnDateFrom ? 'error' : undefined}
                      onChange={(e) => set('returnDateFrom', e.target.value)}
                    />
                    <Input
                      header="Return to"
                      type="date"
                      min={values.returnDateFrom || values.dateFrom || today}
                      value={values.returnDateTo}
                      status={errors.returnDateTo ? 'error' : undefined}
                      onChange={(e) => set('returnDateTo', e.target.value)}
                    />
                  </>
                )}
              </Section>
            </>
          )}

          {step === 'alert' && (
            <Section
              header="When should we alert you?"
              footer={
                values.alertStrategy === 'historical_minimum'
                  ? 'Alerts when the price undercuts the observed minimum. The first days just collect history — no threshold needed.'
                  : errors.maxPriceEur
              }
            >
              <Select
                header="Alert type"
                value={values.alertStrategy}
                onChange={(e) => set('alertStrategy', e.target.value)}
              >
                <option value="absolute_threshold">Below a fixed price</option>
                <option value="historical_minimum">Significant price drop</option>
              </Select>
              {values.alertStrategy === 'absolute_threshold' && (
                <PriceStepper
                  value={values.maxPriceEur}
                  error={errors.maxPriceEur}
                  onChange={(v) => set('maxPriceEur', v)}
                />
              )}
              <Select header="Max stops" value={values.maxStops} onChange={(e) => set('maxStops', e.target.value)}>
                <option value="">Any</option>
                <option value="0">Direct only</option>
                <option value="1">Up to 1</option>
                <option value="2">Up to 2</option>
              </Select>
            </Section>
          )}

          {step === 'summary' && (
            <>
              <Section header="Check and confirm">
                <Cell subtitle="From" multiline>
                  {values.origin}
                  {values.originAlternatives.length > 0 && ` (also ${values.originAlternatives.join(', ')})`}
                </Cell>
                <Cell subtitle="To" multiline>
                  {values.destinations.join(', ')}
                </Cell>
                <Cell subtitle="Departure" multiline>
                  {values.dateFrom && values.dateTo ? formatDateRange(values.dateFrom, values.dateTo) : '—'}
                </Cell>
                <Cell subtitle="Return" multiline>
                  {values.roundTrip && values.returnDateFrom && values.returnDateTo
                    ? formatDateRange(values.returnDateFrom, values.returnDateTo)
                    : 'One-way'}
                </Cell>
                <Cell subtitle="Alert" multiline>
                  {values.alertStrategy === 'absolute_threshold'
                    ? `Below €${values.maxPriceEur || '—'}`
                    : 'On significant price drop'}
                </Cell>
                <Cell subtitle="Stops" multiline>
                  {values.maxStops === '' ? 'Any' : values.maxStops === '0' ? 'Direct only' : `Up to ${values.maxStops}`}
                </Cell>
              </Section>
              {serverError && <Banner type="section" header="Could not save" subheader={serverError} />}
            </>
          )}
        </List>
      </div>

      <FixedLayout vertical="bottom" style={{ padding: 16, background: 'var(--tgui--bg_color)' }}>
        <div style={{ display: 'flex', gap: 12, justifyContent: 'space-between' }}>
          <Button size="l" mode="bezeled" onClick={back}>
            {stepIndex === 0 ? 'Cancel' : 'Back'}
          </Button>
          <Button size="l" loading={saveMutation.isPending} onClick={next}>
            {step === 'summary' ? (initial ? 'Save changes' : 'Create') : 'Next'}
          </Button>
        </div>
      </FixedLayout>
    </>
  )
}
