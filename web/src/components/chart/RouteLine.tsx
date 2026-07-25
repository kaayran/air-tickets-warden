// RouteLine draws a route as a chart course line: origin waypoint, a leader to
// the destination(s), and a faint note of alternative departure airports.
export function RouteLine({
  origin,
  originAlternatives,
  destinations,
}: {
  origin: string
  originAlternatives: string[]
  destinations: string[]
}) {
  return (
    <div className="routeline">
      <span className="routeline__code">{origin || '—'}</span>
      {originAlternatives.length > 0 && (
        <span className="routeline__alt">+{originAlternatives.length}</span>
      )}
      <span className="routeline__arrow" aria-hidden="true" />
      <span className="routeline__dest">{destinations.length > 0 ? destinations.join(' · ') : '—'}</span>
    </div>
  )
}
