import { retrieveRawInitData } from '@telegram-apps/sdk-react'

// The raw initData string Telegram injects into the WebView. Absent when the
// app is opened outside Telegram (e.g. a plain browser during development).
export function getRawInitData(): string | undefined {
  try {
    return retrieveRawInitData()
  } catch {
    return undefined
  }
}

export interface UserSettings {
  chat_id: number
  cooldown_hours: number | null
  drop_pct: number | null
  stable_price_band_pct: number | null
}

export interface Subscription {
  id: string
  origin: string
  origin_alternatives: string[]
  destinations: string[]
  date_from: string // YYYY-MM-DD
  date_to: string
  return_date_from: string | null
  return_date_to: string | null
  trip_length_min: number | null
  trip_length_max: number | null
  max_price_minor: number | null // integer EUR cents — never floats for money
  max_stops: number | null
  max_duration_minutes: number | null
  airlines_whitelist: string[]
  airlines_blacklist: string[]
  alert_strategy: string
  cooldown_hours: number | null
  drop_pct: number | null
  stable_price_band_pct: number | null
  muted_until: string | null // RFC 3339
  status: 'active' | 'paused' | 'archived'
  next_check_at: string
  created_at: string
  updated_at: string
}

export interface AirportHit {
  iata: string
  name: string
  city: string
  country: string
}

export interface FieldError {
  field: string
  message: string
}

// ApiError keeps the server's field-level validation details so the form can
// attach them to inputs.
export class ApiError extends Error {
  status: number
  fields: FieldError[]

  constructor(status: number, message: string, fields: FieldError[] = []) {
    super(message)
    this.status = status
    this.fields = fields
  }
}

// request performs an authenticated API call and normalizes error handling.
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const initData = getRawInitData()
  if (!initData) {
    throw new Error('Open this app from the Telegram bot to sign in.')
  }
  const res = await fetch(path, {
    ...init,
    headers: {
      Authorization: `tma ${initData}`,
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
    },
  })
  if (res.status === 401) {
    // initData expired (1h server-side TTL) or invalid; Telegram only mints a
    // fresh one on app open, so reopening is the only recovery.
    throw new ApiError(401, 'Session expired — close the app and reopen it from Telegram.')
  }
  if (!res.ok) {
    let message = `Request failed (${res.status})`
    let fields: FieldError[] = []
    try {
      const body = (await res.json()) as { error?: string; fields?: FieldError[] }
      if (body.error) message = body.error
      if (body.fields) fields = body.fields
    } catch {
      // non-JSON error body; keep the generic message
    }
    throw new ApiError(res.status, message, fields)
  }
  if (res.status === 204) {
    return undefined as T
  }
  return res.json() as Promise<T>
}

export const fetchMe = () => request<UserSettings>('/api/v1/me')

export const patchMe = (patch: Record<string, number | null>) =>
  request<UserSettings>('/api/v1/me', { method: 'PATCH', body: JSON.stringify(patch) })

export const fetchSubscriptions = () => request<Subscription[]>('/api/v1/subscriptions')

export const createSubscription = (payload: object) =>
  request<Subscription>('/api/v1/subscriptions', { method: 'POST', body: JSON.stringify(payload) })

export const patchSubscription = (id: string, patch: object) =>
  request<Subscription>(`/api/v1/subscriptions/${id}`, { method: 'PATCH', body: JSON.stringify(patch) })

export const deleteSubscription = (id: string) =>
  request<void>(`/api/v1/subscriptions/${id}`, { method: 'DELETE' })

export const searchAirports = (q: string) =>
  request<AirportHit[]>(`/api/v1/airports?q=${encodeURIComponent(q)}`)
