import type { Subscription } from '../../api'
import { isMuted } from '../../lib/format'

// StatusTags renders a subscription's lifecycle and mute state as chart marks
// (framed uppercase labels), not colored pills. Meaning is carried by the label
// text as well as the ink, so it never relies on color alone.
export function StatusTags({ sub }: { sub: Subscription }) {
  const muted = isMuted(sub)
  return (
    <span className="tags">
      {muted && <span className="tag tag--muted">MUTED</span>}
      <span className={`tag tag--${sub.status}`}>{sub.status.toUpperCase()}</span>
    </span>
  )
}
