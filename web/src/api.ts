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

// fetchMe calls GET /api/v1/me with the Telegram initData Authorization header.
export async function fetchMe(): Promise<UserSettings> {
  const initData = getRawInitData()
  if (!initData) {
    throw new Error('Open this app from the Telegram bot to sign in.')
  }
  const res = await fetch('/api/v1/me', {
    headers: { Authorization: `tma ${initData}` },
  })
  if (!res.ok) {
    throw new Error(`Request failed (${res.status})`)
  }
  return res.json() as Promise<UserSettings>
}
