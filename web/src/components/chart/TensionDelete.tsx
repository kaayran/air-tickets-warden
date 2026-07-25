import { useRef, useState } from 'react'

// TensionDelete is the destructive-commit lever: drag it through its arc against
// visible resistance; releasing past the threshold fires, short of it snaps
// back with nothing done. Replaces a fragile confirm-tap. Keyboard users get a
// direct commit via Enter/Space (the drag is a pointer affordance, not the only
// path). The parent shows this only while the row is armed for deletion.
const LEVER_WIDTH = 60
const THRESHOLD_FRAC = 0.8

export function TensionDelete({ onConfirm, onCancel }: { onConfirm: () => void; onCancel: () => void }) {
  const trackRef = useRef<HTMLDivElement>(null)
  const [offset, setOffset] = useState(0)
  const [maxWidth, setMaxWidth] = useState(1)
  const [dragging, setDragging] = useState(false)
  const startX = useRef(0)

  const begin = (clientX: number) => {
    const track = trackRef.current
    if (!track) return
    setMaxWidth(Math.max(1, track.clientWidth - LEVER_WIDTH - 6))
    startX.current = clientX
    setDragging(true)
  }

  const move = (clientX: number) => {
    if (!dragging) return
    setOffset(Math.min(maxWidth, Math.max(0, clientX - startX.current)))
  }

  const end = () => {
    if (!dragging) return
    setDragging(false)
    if (offset >= maxWidth * THRESHOLD_FRAC) onConfirm()
    else setOffset(0)
  }

  const past = offset >= maxWidth * THRESHOLD_FRAC
  const fillScale = (offset + LEVER_WIDTH) / (maxWidth + LEVER_WIDTH)

  return (
    <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
      <div ref={trackRef} className="tension">
        <div className="tension__fill" style={{ transform: `scaleX(${fillScale})` }} />
        <span className="tension__threshold" />
        <span className="tension__label">{past ? 'RELEASE TO DELETE' : 'DRAG TO DELETE'}</span>
        {/* Move/up handlers live on the lever, which holds the pointer capture,
            so the drag keeps tracking even when the pointer leaves the track. */}
        <div
          className="tension__lever"
          role="button"
          tabIndex={0}
          aria-label="Drag to delete, or press Enter to delete"
          style={{ transform: `translateX(${offset}px)`, transition: dragging ? 'none' : 'transform 0.2s cubic-bezier(0.16,1,0.3,1)' }}
          onPointerDown={(e) => {
            e.currentTarget.setPointerCapture(e.pointerId)
            begin(e.clientX)
          }}
          onPointerMove={(e) => move(e.clientX)}
          onPointerUp={end}
          onPointerCancel={end}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              onConfirm()
            }
          }}
        >
          ›
        </div>
      </div>
      <button type="button" className="btn btn--ghost btn--sm" onClick={onCancel}>
        Cancel
      </button>
    </div>
  )
}
