---
name: Air Tickets Warden
description: A personal flight-price instrument read like an aeronautical chart
colors:
  chart-magenta: "#c42a6b"
  chart-blue: "#35618c"
  chart-amber: "#b3781a"
  chart-green: "#2f7d57"
  chart-paper: "#e9e4d4"
  chart-paper-raised: "#f4f1e7"
  chart-night: "#0f161b"
  chart-night-raised: "#16212a"
  ink: "#1b2530"
  ink-muted: "#55606d"
  ink-faint: "#877f6a"
  rule: "#b9b199"
  rule-strong: "#6f6a58"
typography:
  fare:
    fontFamily: "B612 Mono, ui-monospace, monospace"
    fontSize: "26px"
    fontWeight: 700
    lineHeight: 1.1
    letterSpacing: "0.01em"
  title:
    fontFamily: "B612, system-ui, sans-serif"
    fontSize: "22px"
    fontWeight: 700
    lineHeight: 1.05
    letterSpacing: "-0.01em"
  lead:
    fontFamily: "B612 Mono, ui-monospace, monospace"
    fontSize: "18px"
    fontWeight: 700
    lineHeight: 1.15
    letterSpacing: "0.01em"
  body:
    fontFamily: "B612, system-ui, sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: 1.4
    letterSpacing: "normal"
  sub:
    fontFamily: "B612, system-ui, sans-serif"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: 1.35
    letterSpacing: "normal"
  label:
    fontFamily: "B612 Mono, ui-monospace, monospace"
    fontSize: "11px"
    fontWeight: 400
    lineHeight: 1.2
    letterSpacing: "0.14em"
rounded:
  edge: "2px"
spacing:
  sp-2: "8px"
  sp-3: "12px"
  sp-4: "16px"
  sp-5: "24px"
components:
  button-primary:
    backgroundColor: "{colors.chart-magenta}"
    textColor: "{colors.chart-paper-raised}"
    rounded: "{rounded.edge}"
    padding: "14px 20px"
  callout-card:
    backgroundColor: "{colors.chart-paper-raised}"
    textColor: "{colors.ink}"
    rounded: "{rounded.edge}"
    padding: "16px"
---

# Design System: Air Tickets Warden

<!-- Established during a redesign (new-work seed key 330a04a6). Tokens landed with the first build; re-run /impeccable document if the system drifts. -->

## Overview

**Creative North Star: "The Sectional Chart"**

The app is a personal price instrument, and it should read like the thing pilots actually trust: a VFR sectional chart. Routes are course lines between waypoints. Fares are boxed data callouts with corner ticks. Every number — price, date, IATA code, the low–high band a fare sits inside — is set in B612, the typeface Airbus and ENAC designed for cockpit displays, in tabular figures so a column of fares scans as a column. The interface is drawn, not decorated: hairline rules, rectilinear frames, and reserved chart inks doing the work that a travel app would hand to photography and gradients.

The surface is quiet on purpose. Most of a chart is line-work on ground; saturated ink is rationed to what matters. That rationing is the product's anti-spam principle made visual: a watch that just crossed its threshold lights magenta; everything else stays ink-on-paper. The tone is dry and exact — an instrument, not a companion — matching the product voice.

This world explicitly rejects the travel-app category default (a beach or plane-wing hero, sky-blue gradients, pill-rounded cards) and the undesigned telegram-ui gallery look it replaces. It also rejects monospace-as-costume: B612 Mono is here for data and measurement, never as a "technical" veneer over prose.

**Key Characteristics:**
- Aeronautical-chart composition: course lines, boxed callouts, corner ticks, hairline rules.
- Cockpit typography (B612 / B612 Mono), tabular figures for all measured values.
- Reserved chart inks; saturated color only on what matters now.
- Rectilinear, near-sharp corners (2px); no pill shapes, no drop shadows at rest.
- Theme follows the use scene: chart paper in daylight, backlit night chart in the dark.

## Colors

A rationed chart palette: two grounds (paper or night), a three-step ink ramp, hairline rules, and four reserved inks that each carry exactly one meaning.

### Primary
- **Chart Magenta** (#c42a6b light / #e0508a night): the sectional's signature ink. The single active accent — a triggered watch, the primary "File a watch" action, a fare below its target. Never decorative.

### Secondary
- **Airspace Blue** (#35618c / #6ea3d6): structural and informational — links, the focus ring, selected states, neutral emphasis.

### Tertiary
- **Caution Amber** (#b3781a / #d9a13f): paused and muted states, warnings.
- **VFR Green** (#2f7d57 / #56b083): confirmations, "below target", healthy states.

### Neutral
- **Chart Paper / Night** (#e9e4d4 / #0f161b): the page ground. Paper in daylight, backlit night chart in the dark.
- **Callout Fill** (#f4f1e7 / #16212a): raised callout and field surfaces.
- **Ink / Ink Muted / Ink Faint** (#1b2530·#55606d·#877f6a light): the text ramp — primary data, secondary labels, faint captions.
- **Rule / Rule Strong** (#b9b199·#6f6a58): hairline chart lines and heavier section framing.

### Named Rules
**The One Ink Rule.** Magenta covers ≤10% of any screen and only ever marks what is active, triggered, or the primary action. If two things are magenta, one of them is wrong. Its rarity is how the eye finds the alert.

**The Tint-From-Hue Rule.** Secondary text on a colored callout tints from that callout's ink, never flat gray.

## Typography

**Display / Body Font:** B612 (with system-ui fallback)
**Data / Label Font:** B612 Mono (with ui-monospace fallback)

**Character:** B612 was engineered for legibility on aircraft instrument panels under vibration and glance-reading — precisely the register this product wants. The sans carries UI text and headings; the mono carries every measured value, so numbers align in true columns.

### Hierarchy
An authored six-step scale is the only set of sizes the UI uses (tokens `--fs-*` in chart.css):
- **Fare** (B612 Mono 700, 26px, tabular): the price figure — the biggest number on a callout.
- **Title** (B612 700, 22px): screen mastheads.
- **Lead** (B612 Mono 700, 18px): route codes and the wizard step name.
- **Body** (B612 400, 15px, 1.4): field labels, descriptions, inputs, buttons.
- **Sub** (B612 400, 13px): secondary data, field labels, captions, help text.
- **Label** (B612 Mono 400, 11px, 0.14em, uppercase): the one system-wide eyebrow register — section labels, IATA ticks, band endpoints.

### Named Rules
**The Tabular Number Rule.** Every price, date, count, and IATA code renders in B612 Mono with `font-variant-numeric: tabular-nums`. Numbers never sit in the proportional UI face.

## Layout

Single-column, phone-first, inside the Telegram WebView. Content is organized as stacked chart callouts separated by generous ground, with a fixed bottom action bar on task screens (the wizard). Spacing rhythm uses an 8px base (8/12/16/24/32/48); more space sits above a section label than below it. The primary action lives at the bottom of the thumb zone. Touch targets ≥44px.

## Elevation & Depth

Flat by default — a chart has no drop shadows. Depth is drawn, not lit: hairline rules, a heavier frame rule for section boundaries, and inset (sunk) wells for scales. The only "lift" is a state response — a triggered callout gains a magenta wash and frame, and the tension-commit delete lever previews its consequence by dimming the target. No ambient shadows, no glass.

### Named Rules
**The Drawn-Depth Rule.** Separation comes from rules, frames, and ground — never from a resting shadow. A shadow may appear only as a transient response to a drag/press, never at rest.

## Shapes

Rectilinear. Corners are near-sharp (2px), matching chart callout boxes; pill shapes and large radii are out of world. Callouts carry an optional corner tick (a short mismatched rule at one corner) as the recurring chart-callout silhouette. Borders are the primary container device: 1px hairline rules for internal division, 1.5px frame rules for callout and section edges.

## Components

### Buttons
- **Shape:** rectilinear, 2px corners.
- **Primary ("File a watch", "Confirm"):** solid Chart Magenta, paper-colored text, 14×20px padding, B612 700. The only magenta button on a screen.
- **Secondary:** framed (1.5px rule) transparent fill, ink text — "Back", "Cancel", stepper controls.
- **Hover/Focus:** primary deepens to magenta-deep; all controls take the blue focus ring on `:focus-visible`.

### Chips (airport selections)
- **Style:** framed rectilinear tag, callout-fill background, B612 Mono IATA code, ink text, a `×` remove affordance. No pill radius.

### Cards / Containers (the callout card)
- **Corner Style:** 2px, with an optional corner tick.
- **Background:** callout fill; a triggered watch takes the magenta wash.
- **Border:** 1.5px frame rule; the triggered state frames in magenta.
- **Shadow:** none at rest (see Drawn-Depth Rule).
- **Internal Padding:** 16px.

### Inputs / Fields
- **Style:** callout-fill, 1px rule frame, 2px corners, B612 UI text (B612 Mono for numeric inputs).
- **Focus:** blue focus ring.
- **Error:** frame shifts to magenta with an inline message beneath the field, in words, naming the fix.

### Navigation
- **Tabs:** a bottom chart-rule bar; the active tab is marked by a magenta tick and ink label, inactive tabs are muted ink. Text labels, no icons.
- **Wizard progress:** a labeled step scale — "Step N of 5 · <name>" over a segmented chart rule — never unlabeled dots.

### Price Band (signature component)
The mechanism made visible: a horizontal inset scale from a route's historical low to high with a marker at the current fare, magenta when below target. Endpoints are B612 Mono labels. Until live prices exist (Phase 2+), it renders an honest "awaiting price history" state — never a fabricated number.

### Tension-Commit Delete (signature interaction)
Destructive delete is a lever dragged through an arc against rising resistance, with the target row dimming in preview; releasing past the marked threshold fires, short of it snaps back. Replaces a fragile confirm tap and gives the delete a real, reversible-until-committed feel.

## Do's and Don'ts

### Do:
- **Do** set every price, date, count, and IATA code in B612 Mono tabular figures.
- **Do** ration magenta to a single active/alert/primary element per screen (The One Ink Rule).
- **Do** build separation from hairline rules, frame rules, and ground; keep surfaces flat at rest.
- **Do** label wizard progress with step number and name.
- **Do** state coverage honestly (e.g. "Wizz Air isn't covered") and render "awaiting price history" rather than invent a fare.

### Don't:
- **Don't** use pill shapes, large radii, or resting drop shadows — the world is drawn and rectilinear.
- **Don't** introduce a second saturated accent alongside magenta on one screen.
- **Don't** put numbers in the proportional UI face.
- **Don't** reach for gradients, glass, sky-blue travel-app tropes, or a photographic hero.
- **Don't** add a colored `border-left` bar as a status device; frame the whole callout instead.
