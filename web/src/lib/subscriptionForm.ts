// Pure form logic for the subscription create/edit screen: values <-> API
// payloads and client-side validation. No React, no I/O — vitest covers it.
// The server revalidates everything; this mirror exists only for instant
// feedback in the form.
import type { Subscription } from '../api'

export interface SubscriptionFormValues {
  origin: string // IATA, '' when unset
  originAlternatives: string[]
  destinations: string[]
  dateFrom: string // YYYY-MM-DD, '' when unset
  dateTo: string
  roundTrip: boolean
  returnDateFrom: string
  returnDateTo: string
  alertStrategy: string // absolute_threshold | historical_minimum
  maxPriceEur: string // whole euros as typed; converted to minor units at the edge
  maxStops: string // '' = any, else '0' | '1' | '2'
}

export const emptyForm: SubscriptionFormValues = {
  origin: '',
  originAlternatives: [],
  destinations: [],
  dateFrom: '',
  dateTo: '',
  roundTrip: false,
  returnDateFrom: '',
  returnDateTo: '',
  alertStrategy: 'absolute_threshold',
  maxPriceEur: '',
  maxStops: '',
}

export type FormErrors = Partial<Record<keyof SubscriptionFormValues, string>>

// The create/edit flow is a wizard; each page validates only its own fields
// on "Next" (the summary page validates everything on confirm).
export type WizardStep = 'from' | 'to' | 'dates' | 'alert' | 'summary'

export const wizardSteps: WizardStep[] = ['from', 'to', 'dates', 'alert', 'summary']

const stepFields: Record<WizardStep, (keyof SubscriptionFormValues)[]> = {
  from: ['origin', 'originAlternatives'],
  to: ['destinations'],
  dates: ['dateFrom', 'dateTo', 'returnDateFrom', 'returnDateTo'],
  alert: ['alertStrategy', 'maxPriceEur', 'maxStops'],
  summary: [],
}

// stepErrors filters full-form errors down to the fields the given step shows.
export function stepErrors(step: WizardStep, errors: FormErrors): FormErrors {
  const out: FormErrors = {}
  for (const field of stepFields[step]) {
    if (errors[field]) out[field] = errors[field]
  }
  return out
}

// firstInvalidStep returns the earliest wizard page that has an error, so the
// summary's confirm can send the user back to the right place.
export function firstInvalidStep(errors: FormErrors): WizardStep | null {
  for (const step of wizardSteps) {
    if (Object.keys(stepErrors(step, errors)).length > 0) return step
  }
  return null
}

// todayISO returns the device's current civil date as YYYY-MM-DD.
export function todayISO(now: Date = new Date()): string {
  const y = now.getFullYear()
  const m = String(now.getMonth() + 1).padStart(2, '0')
  const d = String(now.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

// addDays shifts a YYYY-MM-DD date by n days (UTC arithmetic — the local
// timezone must not bleed into civil-date math).
export function addDays(iso: string, days: number): string {
  const [y, m, d] = iso.split('-').map(Number)
  const shifted = new Date(Date.UTC(y, m - 1, d + days))
  return shifted.toISOString().slice(0, 10)
}

// validate mirrors the server's rules for the fields this form exposes.
// YYYY-MM-DD strings compare correctly as plain strings.
export function validate(v: SubscriptionFormValues, today: string = todayISO()): FormErrors {
  const errors: FormErrors = {}

  if (!v.origin) {
    errors.origin = 'Pick a departure airport'
  }
  if (v.destinations.length === 0) {
    errors.destinations = 'Pick at least one destination'
  } else if (v.destinations.includes(v.origin)) {
    errors.destinations = 'Destination equals origin'
  }

  if (!v.dateFrom || !v.dateTo) {
    errors.dateFrom = 'Pick the departure date range'
  } else if (v.dateTo < v.dateFrom) {
    errors.dateTo = 'Range ends before it starts'
  } else if (v.dateTo < today) {
    errors.dateTo = 'The window is entirely in the past'
  }

  if (v.roundTrip) {
    if (!v.returnDateFrom || !v.returnDateTo) {
      errors.returnDateFrom = 'Pick the return date range'
    } else if (v.returnDateTo < v.returnDateFrom) {
      errors.returnDateTo = 'Range ends before it starts'
    } else if (v.dateFrom && v.returnDateFrom < v.dateFrom) {
      errors.returnDateFrom = 'Return starts before departure'
    }
  }

  // The threshold is mandatory only for the fixed-price strategy; a
  // historical-minimum subscription may skip it, but a typed value must still
  // be a sane amount.
  const price = Number(v.maxPriceEur)
  if (!v.maxPriceEur.trim()) {
    if (v.alertStrategy === 'absolute_threshold') {
      errors.maxPriceEur = 'Set the price threshold'
    }
  } else if (!Number.isFinite(price) || price <= 0 || !Number.isInteger(price)) {
    errors.maxPriceEur = 'Whole euros, more than 0'
  }

  return errors
}

export const isValid = (errors: FormErrors) => Object.keys(errors).length === 0

// eurosToMinor converts the typed whole-euro string to integer minor units.
export function eurosToMinor(euros: string): number {
  return Number(euros) * 100
}

export function minorToEuros(minor: number): number {
  return Math.round(minor / 100)
}

// toCreatePayload shapes validated form values into the POST body.
export function toCreatePayload(v: SubscriptionFormValues): object {
  return {
    origin: v.origin,
    origin_alternatives: v.originAlternatives,
    destinations: v.destinations,
    date_from: v.dateFrom,
    date_to: v.dateTo,
    return_date_from: v.roundTrip ? v.returnDateFrom : null,
    return_date_to: v.roundTrip ? v.returnDateTo : null,
    alert_strategy: v.alertStrategy,
    max_price_minor: v.maxPriceEur.trim() === '' ? null : eurosToMinor(v.maxPriceEur),
    max_stops: v.maxStops === '' ? null : Number(v.maxStops),
  }
}

// fromSubscription seeds the form for editing.
export function fromSubscription(s: Subscription): SubscriptionFormValues {
  return {
    origin: s.origin,
    originAlternatives: s.origin_alternatives,
    destinations: s.destinations,
    dateFrom: s.date_from,
    dateTo: s.date_to,
    roundTrip: s.return_date_from != null,
    returnDateFrom: s.return_date_from ?? '',
    returnDateTo: s.return_date_to ?? '',
    alertStrategy: s.alert_strategy,
    maxPriceEur: s.max_price_minor != null ? String(minorToEuros(s.max_price_minor)) : '',
    maxStops: s.max_stops != null ? String(s.max_stops) : '',
  }
}

// toPatchPayload diffs edited values against the original subscription and
// returns only the changed keys — PATCH semantics: absent means untouched,
// null means clear. Returns {} when nothing changed.
export function toPatchPayload(v: SubscriptionFormValues, original: Subscription): Record<string, unknown> {
  const patch: Record<string, unknown> = {}
  const orig = fromSubscription(original)

  if (v.origin !== orig.origin) patch.origin = v.origin
  if (!sameList(v.originAlternatives, orig.originAlternatives)) {
    patch.origin_alternatives = v.originAlternatives
  }
  if (!sameList(v.destinations, orig.destinations)) patch.destinations = v.destinations
  if (v.dateFrom !== orig.dateFrom) patch.date_from = v.dateFrom
  if (v.dateTo !== orig.dateTo) patch.date_to = v.dateTo

  const retFrom = v.roundTrip ? v.returnDateFrom : ''
  const retTo = v.roundTrip ? v.returnDateTo : ''
  if (retFrom !== orig.returnDateFrom || retTo !== orig.returnDateTo) {
    patch.return_date_from = retFrom === '' ? null : retFrom
    patch.return_date_to = retTo === '' ? null : retTo
  }

  if (v.alertStrategy !== orig.alertStrategy) patch.alert_strategy = v.alertStrategy
  if (v.maxPriceEur !== orig.maxPriceEur) {
    patch.max_price_minor = v.maxPriceEur.trim() === '' ? null : eurosToMinor(v.maxPriceEur)
  }
  if (v.maxStops !== orig.maxStops) {
    patch.max_stops = v.maxStops === '' ? null : Number(v.maxStops)
  }
  return patch
}

function sameList(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((x, i) => x === b[i])
}
