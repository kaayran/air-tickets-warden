import { formatPrice } from '../../lib/format'

// PriceBand is the mechanism made visible: where a fare sits between a route's
// historical low and high. Live prices don't exist yet (Phase 2+), so it never
// fabricates a fare — it states the target (or the drop rule) and renders an
// honest "awaiting price history" note in place of the marker.
export function PriceBand({
  maxPriceMinor,
  strategy,
}: {
  maxPriceMinor: number | null
  strategy: string
}) {
  return (
    <div className="band">
      <div className="fare">
        {maxPriceMinor != null ? (
          <>
            <span className="fare__value data">{formatPrice(maxPriceMinor)}</span>
            <span className="fare__caption">alert threshold</span>
          </>
        ) : (
          <>
            <span className="fare__value data">DROP</span>
            <span className="fare__caption">
              {strategy === 'historical_minimum' ? 'below the route low' : 'on a significant drop'}
            </span>
          </>
        )}
      </div>
      <div className="band__await data">AWAITING PRICE HISTORY — watching from now</div>
    </div>
  )
}
