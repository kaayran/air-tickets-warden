// Small pure display helpers shared by the screens.
import type { Subscription } from '../api'

// formatPrice renders integer EUR minor units ("€150"; cents shown only when
// non-zero, which for user-set thresholds they never are).
export function formatPrice(minor: number): string {
  const euros = minor / 100
  return `€${Number.isInteger(euros) ? euros : euros.toFixed(2)}`
}

// formatRoute renders "BEG → BCN, MAD" (+n alternatives when present).
export function formatRoute(s: Subscription): string {
  const alts = s.origin_alternatives.length > 0 ? `+${s.origin_alternatives.length}` : ''
  return `${s.origin}${alts} → ${s.destinations.join(', ')}`
}

// formatDateRange renders "1 – 15 Jul 2030" from YYYY-MM-DD bounds, and spells
// out both years when the window crosses New Year ("28 Dec 2030 – 3 Jan 2031").
export function formatDateRange(from: string, to: string): string {
  const f = new Date(`${from}T00:00:00`)
  const t = new Date(`${to}T00:00:00`)
  const day = (d: Date) => d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })
  if (f.getFullYear() !== t.getFullYear()) {
    return `${day(f)} ${f.getFullYear()} – ${day(t)} ${t.getFullYear()}`
  }
  return `${day(f)} – ${day(t)} ${t.getFullYear()}`
}

// isMuted mirrors the server rule: muted while muted_until is in the future.
export function isMuted(s: Subscription, now: Date = new Date()): boolean {
  return s.muted_until != null && new Date(s.muted_until) > now
}

// muteUntilISO returns an RFC 3339 timestamp `days` days from now.
export function muteUntilISO(days: number, now: Date = new Date()): string {
  return new Date(now.getTime() + days * 24 * 60 * 60 * 1000).toISOString()
}
