# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

A small circle of friends — each an individual traveller with their own routes,
home airports, and date flexibility, not a shared team on one itinerary. Every
user manages a private set of subscriptions; there is no collaboration,
shared inventory, or org structure. Access is whitelist-only (Telegram user
IDs); this is not a public service and never authenticates strangers.

The usage scene: someone loosely planning a trip who knows roughly where and
when they want to go but wants to buy when the price is right. They set up a
watch once, then live in Telegram; the product reaches back out to them.

## Product Purpose

Watch air-ticket prices on personal routes and notify the user, in Telegram,
when a fare is worth acting on. The user creates monitoring rules (route +
date window + alert conditions); the service polls several fare sources on a
schedule, normalizes everything to EUR, keeps a per-route price history, and
sends a bot message when a rule's condition is met. Success is a timely,
trustworthy alert that leads to a booking the user would otherwise have missed
— with no spam in between.

Explicitly out of scope (MVP): no booking or payments (alerts carry links
only), no multi-city/open-jaw split-ticket itineraries, no baggage/seat-fee
modelling.

## Positioning

"Cheap" is defined relative to a route's own history, not an absolute number
the user has to guess. The product accumulates price observations per route and
can alert on a drop against the observed minimum — so a user who has no idea
what a fare "should" cost still gets a meaningful signal. Two supporting
mechanisms reinforce this: multi-source coverage (no single API sees the whole
market, especially low-cost carriers, so several are polled in parallel) and
airport flexibility (alternative departure airports compared on effective
price including ground-transfer cost).

## Operating Context

- The only management UI is a **Telegram Mini App** (React SPA embedded in the
  Go binary, opened inside Telegram over HTTPS). All setup, editing, and
  browsing happen there.
- The **bot** is the entry point (`/start` → "Open App") and the notification
  channel. A Mini App cannot push while closed, so every alert is a bot
  message — the Mini App is for management, the bot for delivery.
- Sessions are short and phone-first: the app is opened from a Telegram chat,
  used briefly, and closed. Telegram `initData` authenticates each open and
  expires after ~1 hour; an expired session must be reopened from the bot.
- The app renders inside Telegram's WebView and inherits Telegram's theme
  (light/dark, user-controlled) via the telegram-ui component library.

## Capabilities and Constraints

- **Subscription CRUD** (implemented): origin + alternative departures,
  one or more destinations, departure date window, optional return window for
  round trips, max stops, and an alert rule. Lifecycle: active / paused /
  archived, plus a "muted until" state that suppresses notifications while
  monitoring continues.
- **Alert strategies:** `absolute_threshold` (fare below a user-set EUR price)
  and `historical_minimum` (a significant drop against accumulated history).
  Alert parameters resolve through a cascade: per-subscription → per-user
  settings → service defaults.
- **Airport picker** over an embedded offline dataset (~8800 airports):
  browse-on-focus list of major hubs plus first-character search.
- **Money is always integer EUR minor units** end to end; everything is
  normalized to EUR (no currency column, no floats for money).
- **Not yet built (later phases):** live price fetching and "check now",
  price-history charts and stats, scheduler-driven autonomous monitoring, and
  the alert delivery itself. Wizz Air is an accepted coverage gap for v1.0
  (no API / no GDS presence) and the UI must state such gaps honestly rather
  than imply full coverage.
- Single instance, no horizontal scaling; personal-scale data volumes.

## Brand Commitments

- Name: **Air Tickets Warden**.
- UI language: **English** (interface copy and bot notifications).
- **No emoji in the interface** (user directive) — labels and states are text.
- Tone: **dry and precise** — an instrument, not a companion. Numbers, route,
  price, date; no conversational filler, no hype, no exclamation. State facts
  ("Price dropped 30% below the 90-day low"), don't sell them.

## Evidence on Hand

- Design document (`air-tickets-warden.md`) and phased plan (`PLAN.md`) — the
  authoritative product and roadmap record.
- Working Mini App (`web/src`): subscriptions list, a five-step create/edit
  wizard, and a settings screen, built on `@telegram-apps/telegram-ui` +
  TanStack Query.
- No live fare data yet: there are no real prices, price histories, or sent
  alerts to show. Any screen depicting them must use clearly non-fabricated
  placeholder or empty states until the fare adapters (Phase 2+) land.

## Product Principles

1. **History over spot price.** Cheapness is relative to a route's own past;
   never force the user to name a number they cannot know.
2. **Reach out, don't demand attention.** The user lives in Telegram; the
   product does the watching and messages only when it earns the interruption.
3. **Anti-spam is a feature.** Cooldowns, mute, and stable-price bands exist so
   an alert always means something — silence is correct most of the time.
4. **Honest coverage.** Say what is and isn't watched (e.g. Wizz Air); never
   imply a completeness the sources don't provide.
5. **Instrument, not companion.** Dry, precise, text-only; the craft shows in
   exactness and legibility, not personality.

## Accessibility & Inclusion

Must inherit and respect Telegram's user-chosen light/dark theme; never fight
the host theme. Phone-first touch targets and legibility inside the Telegram
WebView are the baseline.
