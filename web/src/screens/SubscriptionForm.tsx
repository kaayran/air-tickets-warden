import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Banner, Button, ButtonCell, Input, List, Section, Select, Switch } from '@telegram-apps/telegram-ui'
import { ApiError, createSubscription, patchSubscription, type Subscription } from '../api'
import { AirportPicker } from '../components/AirportPicker'
import { PriceStepper } from '../components/PriceStepper'
import {
  emptyForm,
  fromSubscription,
  isValid,
  toCreatePayload,
  toPatchPayload,
  todayISO,
  validate,
  type FormErrors,
  type SubscriptionFormValues,
} from '../lib/subscriptionForm'

export interface SubscriptionFormProps {
  /** Present when editing; absent when creating. */
  initial?: Subscription
  onDone: () => void
}

// SubscriptionForm is the create/edit screen. Client-side validation mirrors
// the server for instant feedback; the server remains the authority and its
// field errors are surfaced too. On edit, only changed fields are PATCHed —
// fields this form does not expose are never clobbered.
export function SubscriptionForm({ initial, onDone }: SubscriptionFormProps) {
  const queryClient = useQueryClient()
  const [values, setValues] = useState<SubscriptionFormValues>(
    initial ? fromSubscription(initial) : emptyForm,
  )
  const [errors, setErrors] = useState<FormErrors>({})
  const [serverError, setServerError] = useState<string | null>(null)

  const set = <K extends keyof SubscriptionFormValues>(key: K, value: SubscriptionFormValues[K]) => {
    setValues((v) => ({ ...v, [key]: value }))
    setErrors((e) => ({ ...e, [key]: undefined }))
  }

  const saveMutation = useMutation({
    mutationFn: () =>
      initial ? patchSubscription(initial.id, toPatchPayload(values, initial)) : createSubscription(toCreatePayload(values)),
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

  const submit = () => {
    setServerError(null)
    const errs = validate(values)
    setErrors(errs)
    if (!isValid(errs)) return
    if (initial && Object.keys(toPatchPayload(values, initial)).length === 0) {
      onDone() // nothing changed
      return
    }
    saveMutation.mutate()
  }

  const today = todayISO()

  return (
    <List>
      <Section header={initial ? 'Edit subscription' : 'New subscription'}>
        <ButtonCell onClick={onDone}>← Back</ButtonCell>
      </Section>

      <AirportPicker
        header="From"
        placeholder="City or IATA code"
        selected={values.origin ? [values.origin] : []}
        max={1}
        error={errors.origin}
        onChange={(codes) => set('origin', codes[0] ?? '')}
      />
      <AirportPicker
        header="Alternative departures (optional)"
        placeholder="e.g. nearby airports"
        selected={values.originAlternatives}
        max={5}
        exclude={values.origin ? [values.origin] : []}
        onChange={(codes) => set('originAlternatives', codes)}
      />
      <AirportPicker
        header="To"
        placeholder="City or IATA code"
        selected={values.destinations}
        max={10}
        exclude={values.origin ? [values.origin] : []}
        error={errors.destinations}
        onChange={(codes) => set('destinations', codes)}
      />

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

      <Section header="Round trip" footer={errors.returnDateFrom ?? errors.returnDateTo}>
        <Input
          header="Return window"
          readOnly
          value={values.roundTrip ? 'on' : 'off'}
          after={<Switch checked={values.roundTrip} onChange={(e) => set('roundTrip', e.target.checked)} />}
        />
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

      <Section header="Alert" footer={errors.maxPriceEur}>
        <PriceStepper value={values.maxPriceEur} error={errors.maxPriceEur} onChange={(v) => set('maxPriceEur', v)} />
        <Select header="Max stops" value={values.maxStops} onChange={(e) => set('maxStops', e.target.value)}>
          <option value="">Any</option>
          <option value="0">Direct only</option>
          <option value="1">Up to 1</option>
          <option value="2">Up to 2</option>
        </Select>
      </Section>

      {serverError && <Banner type="section" header="Could not save" subheader={serverError} />}

      <Section>
        <div style={{ padding: 16 }}>
          <Button size="l" stretched loading={saveMutation.isPending} onClick={submit}>
            {initial ? 'Save changes' : 'Create subscription'}
          </Button>
        </div>
      </Section>
    </List>
  )
}
