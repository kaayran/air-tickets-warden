import { describe, expect, it } from 'vitest'
import type { Subscription } from '../api'
import {
  addDays,
  emptyForm,
  eurosToMinor,
  firstInvalidStep,
  fromSubscription,
  isValid,
  stepErrors,
  toCreatePayload,
  toPatchPayload,
  todayISO,
  validate,
  type SubscriptionFormValues,
} from './subscriptionForm'

const TODAY = '2030-01-01'

function validForm(): SubscriptionFormValues {
  return {
    ...emptyForm,
    origin: 'BEG',
    destinations: ['BCN', 'MAD'],
    dateFrom: '2030-07-01',
    dateTo: '2030-07-15',
    maxPriceEur: '150',
  }
}

function subscription(overrides: Partial<Subscription> = {}): Subscription {
  return {
    id: '00000000-0000-0000-0000-000000000001',
    origin: 'BEG',
    origin_alternatives: [],
    destinations: ['BCN', 'MAD'],
    date_from: '2030-07-01',
    date_to: '2030-07-15',
    return_date_from: null,
    return_date_to: null,
    trip_length_min: null,
    trip_length_max: null,
    max_price_minor: 15000,
    max_stops: null,
    max_duration_minutes: null,
    airlines_whitelist: [],
    airlines_blacklist: [],
    alert_strategy: 'absolute_threshold',
    cooldown_hours: null,
    drop_pct: null,
    stable_price_band_pct: null,
    muted_until: null,
    status: 'active',
    next_check_at: '2030-01-01T00:00:00Z',
    created_at: '2030-01-01T00:00:00Z',
    updated_at: '2030-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('validate', () => {
  it('accepts a complete one-way form', () => {
    expect(validate(validForm(), TODAY)).toEqual({})
  })

  it('requires origin, destinations, dates, and price', () => {
    const errors = validate(emptyForm, TODAY)
    expect(errors.origin).toBeDefined()
    expect(errors.destinations).toBeDefined()
    expect(errors.dateFrom).toBeDefined()
    expect(errors.maxPriceEur).toBeDefined()
    expect(isValid(errors)).toBe(false)
  })

  it('rejects destination equal to origin', () => {
    const v = { ...validForm(), destinations: ['BEG'] }
    expect(validate(v, TODAY).destinations).toMatch(/origin/)
  })

  it('rejects an inverted date range', () => {
    const v = { ...validForm(), dateFrom: '2030-07-15', dateTo: '2030-07-01' }
    expect(validate(v, TODAY).dateTo).toBeDefined()
  })

  it('rejects a window entirely in the past', () => {
    const v = { ...validForm(), dateFrom: '2029-01-01', dateTo: '2029-01-05' }
    expect(validate(v, TODAY).dateTo).toMatch(/past/)
  })

  it('requires the return range when round trip is on', () => {
    const v = { ...validForm(), roundTrip: true }
    expect(validate(v, TODAY).returnDateFrom).toBeDefined()
  })

  it('rejects a return window starting before departure', () => {
    const v = {
      ...validForm(),
      roundTrip: true,
      returnDateFrom: '2030-06-01',
      returnDateTo: '2030-07-20',
    }
    expect(validate(v, TODAY).returnDateFrom).toMatch(/before departure/)
  })

  it('accepts a consistent round trip', () => {
    const v = {
      ...validForm(),
      roundTrip: true,
      returnDateFrom: '2030-07-05',
      returnDateTo: '2030-07-25',
    }
    expect(validate(v, TODAY)).toEqual({})
  })

  it('rejects zero, negative, and fractional prices', () => {
    for (const bad of ['0', '-5', '10.5', 'abc', '']) {
      expect(validate({ ...validForm(), maxPriceEur: bad }, TODAY).maxPriceEur).toBeDefined()
    }
  })

  it('does not require a price for the historical-minimum strategy', () => {
    const v = { ...validForm(), alertStrategy: 'historical_minimum', maxPriceEur: '' }
    expect(validate(v, TODAY)).toEqual({})
  })

  it('still rejects a malformed price under historical minimum', () => {
    const v = { ...validForm(), alertStrategy: 'historical_minimum', maxPriceEur: '-5' }
    expect(validate(v, TODAY).maxPriceEur).toBeDefined()
  })
})

describe('payloads', () => {
  it('converts euros to integer minor units', () => {
    expect(eurosToMinor('150')).toBe(15000)
    const payload = toCreatePayload(validForm()) as { max_price_minor: number }
    expect(payload.max_price_minor).toBe(15000)
    expect(Number.isInteger(payload.max_price_minor)).toBe(true)
  })

  it('builds a complete create payload', () => {
    expect(toCreatePayload(validForm())).toEqual({
      origin: 'BEG',
      origin_alternatives: [],
      destinations: ['BCN', 'MAD'],
      date_from: '2030-07-01',
      date_to: '2030-07-15',
      return_date_from: null,
      return_date_to: null,
      alert_strategy: 'absolute_threshold',
      max_price_minor: 15000,
      max_stops: null,
    })
  })

  it('sends a null price (not 0) for a thresholdless drop subscription', () => {
    const v = { ...validForm(), alertStrategy: 'historical_minimum', maxPriceEur: '' }
    const payload = toCreatePayload(v) as { alert_strategy: string; max_price_minor: number | null }
    expect(payload.alert_strategy).toBe('historical_minimum')
    expect(payload.max_price_minor).toBeNull()
  })

  it('diffs the strategy and clears the price with null', () => {
    const sub = subscription()
    const edited = { ...fromSubscription(sub), alertStrategy: 'historical_minimum', maxPriceEur: '' }
    expect(toPatchPayload(edited, sub)).toEqual({
      alert_strategy: 'historical_minimum',
      max_price_minor: null,
    })
  })

  it('round-trips a subscription through the form without a diff', () => {
    const sub = subscription({ return_date_from: '2030-07-05', return_date_to: '2030-07-25', max_stops: 1 })
    expect(toPatchPayload(fromSubscription(sub), sub)).toEqual({})
  })

  it('diffs only the changed fields', () => {
    const sub = subscription()
    const edited = { ...fromSubscription(sub), maxPriceEur: '120', destinations: ['BCN'] }
    expect(toPatchPayload(edited, sub)).toEqual({
      max_price_minor: 12000,
      destinations: ['BCN'],
    })
  })

  it('clears the return window with nulls when round trip is switched off', () => {
    const sub = subscription({ return_date_from: '2030-07-05', return_date_to: '2030-07-25' })
    const edited = { ...fromSubscription(sub), roundTrip: false }
    expect(toPatchPayload(edited, sub)).toEqual({
      return_date_from: null,
      return_date_to: null,
    })
  })

  it('clears max stops back to any with null', () => {
    const sub = subscription({ max_stops: 1 })
    const edited = { ...fromSubscription(sub), maxStops: '' }
    expect(toPatchPayload(edited, sub)).toEqual({ max_stops: null })
  })
})

describe('todayISO', () => {
  it('formats the civil date', () => {
    expect(todayISO(new Date(2030, 0, 5))).toBe('2030-01-05')
  })
})

describe('addDays', () => {
  it('adds within a month', () => {
    expect(addDays('2030-07-15', 7)).toBe('2030-07-22')
  })
  it('crosses month and year boundaries', () => {
    expect(addDays('2030-07-28', 7)).toBe('2030-08-04')
    expect(addDays('2030-12-30', 7)).toBe('2031-01-06')
  })
})

describe('wizard steps', () => {
  it('filters errors down to the current page', () => {
    const errors = validate(emptyForm, TODAY)
    expect(Object.keys(stepErrors('from', errors))).toEqual(['origin'])
    expect(Object.keys(stepErrors('to', errors))).toEqual(['destinations'])
    expect(stepErrors('summary', errors)).toEqual({})
  })

  it('points the confirm at the earliest failing page', () => {
    expect(firstInvalidStep(validate(emptyForm, TODAY))).toBe('from')
    const noPrice = { ...validForm(), maxPriceEur: '' }
    expect(firstInvalidStep(validate(noPrice, TODAY))).toBe('alert')
    expect(firstInvalidStep(validate(validForm(), TODAY))).toBeNull()
  })
})
