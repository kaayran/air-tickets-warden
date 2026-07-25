import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { deleteSubscription, patchSubscription, type Subscription } from '../api'
import { Button, Masthead, Screen, SectionLabel } from '../components/chart/ui'
import { RouteLine } from '../components/chart/RouteLine'
import { PriceBand } from '../components/chart/PriceBand'
import { StatusTags } from '../components/chart/StatusTags'
import { TensionDelete } from '../components/chart/TensionDelete'
import { formatDateRange, isMuted, muteUntilISO } from '../lib/format'

const MUTE_DAYS = 3

export interface SubscriptionsListProps {
  onCreate: () => void
  onEdit: (sub: Subscription) => void
  onDuplicate: (sub: Subscription) => void
}

// SubscriptionsList is the chart index: every watched route as a callout with
// its status, dates, and price band, plus one-tap pause/resume, mute, duplicate,
// and a tension-lever delete. Mutations update the cache optimistically so the
// phone feels instant.
export function SubscriptionsList({ onCreate, onEdit, onDuplicate }: SubscriptionsListProps) {
  const queryClient = useQueryClient()
  const { data: subs, error, isLoading } = useQuery<Subscription[]>({ queryKey: ['subscriptions'] })
  const [arming, setArming] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const patchMutation = useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: object }) => patchSubscription(id, patch),
    onMutate: async ({ id, patch }) => {
      await queryClient.cancelQueries({ queryKey: ['subscriptions'] })
      const previous = queryClient.getQueryData<Subscription[]>(['subscriptions'])
      queryClient.setQueryData<Subscription[]>(['subscriptions'], (rows) =>
        rows?.map((s) => (s.id === id ? { ...s, ...patch } : s)),
      )
      return { previous }
    },
    onError: (_err, _vars, ctx) => {
      queryClient.setQueryData(['subscriptions'], ctx?.previous)
      setActionError('Could not update the watch. Try again.')
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['subscriptions'] }),
  })

  // Delete optimistically removes the row and restores it on failure, with a
  // visible error — no silent frozen row.
  const deleteMutation = useMutation({
    mutationFn: deleteSubscription,
    onMutate: async (id: string) => {
      await queryClient.cancelQueries({ queryKey: ['subscriptions'] })
      const previous = queryClient.getQueryData<Subscription[]>(['subscriptions'])
      queryClient.setQueryData<Subscription[]>(['subscriptions'], (rows) => rows?.filter((s) => s.id !== id))
      return { previous }
    },
    onError: (_err, _id, ctx) => {
      queryClient.setQueryData(['subscriptions'], ctx?.previous)
      setActionError('Could not delete the watch. Try again.')
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['subscriptions'] }),
  })

  if (isLoading) return <div className="state-msg">Loading watches…</div>
  if (error || !subs) {
    return (
      <div className="state-msg">{error instanceof Error ? error.message : 'Could not load watches.'}</div>
    )
  }

  if (subs.length === 0) {
    return (
      <Screen>
        <Masthead title="Air Tickets Warden" />
        <div className="empty">
          <p className="empty__title">No watches filed</p>
          <p className="empty__body">
            File a watch on a route and the warden tracks its price from now on, alerting you in Telegram when a fare
            is worth acting on.
          </p>
          <div className="empty__demo">
            <SectionLabel>A watch alert looks like</SectionLabel>
            <p className="empty__demo-msg">
              <span className="data">BEG → BCN</span> · fare below <span className="data">€150</span>.
              <br />
              Sources checked live. Wizz Air isn’t covered.
            </p>
          </div>
          <Button variant="primary" onClick={onCreate}>
            File a watch
          </Button>
        </div>
      </Screen>
    )
  }

  return (
    <Screen>
      <Masthead title="Watches" meta={`${subs.length} filed`} />
      <div style={{ marginBottom: 16 }}>
        <Button variant="primary" block onClick={onCreate}>
          File a watch
        </Button>
      </div>
      {actionError && (
        <div className="notice" style={{ marginBottom: 12 }} role="alert">
          {actionError}
        </div>
      )}

      {subs.map((s) => {
        const muted = isMuted(s)
        const paused = s.status === 'paused'
        return (
          <div className="callout" key={s.id}>
            <button
              type="button"
              onClick={() => onEdit(s)}
              aria-label={`Edit watch ${s.origin} to ${s.destinations.join(', ')}`}
              style={{ display: 'block', width: '100%', textAlign: 'left', background: 'none', border: 'none', padding: 0, cursor: 'pointer' }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
                <RouteLine origin={s.origin} originAlternatives={s.origin_alternatives} destinations={s.destinations} />
                <StatusTags sub={s} />
              </div>
              <div className="data t-sub" style={{ marginTop: 8, color: 'var(--ink-muted)' }}>
                {formatDateRange(s.date_from, s.date_to)}
                {s.return_date_from && s.return_date_to
                  ? ` · rtn ${formatDateRange(s.return_date_from, s.return_date_to)}`
                  : ''}
              </div>
              <PriceBand maxPriceMinor={s.max_price_minor} strategy={s.alert_strategy} />
            </button>

            {arming === s.id ? (
              <div style={{ marginTop: 12 }}>
                <TensionDelete
                  onConfirm={() => {
                    setArming(null)
                    setActionError(null)
                    deleteMutation.mutate(s.id)
                  }}
                  onCancel={() => setArming(null)}
                />
              </div>
            ) : (
              <div className="row-actions">
                <Button
                  sm
                  onClick={() => patchMutation.mutate({ id: s.id, patch: { status: paused ? 'active' : 'paused' } })}
                >
                  {paused ? 'Resume' : 'Pause'}
                </Button>
                <Button
                  sm
                  onClick={() =>
                    patchMutation.mutate({ id: s.id, patch: { muted_until: muted ? null : muteUntilISO(MUTE_DAYS) } })
                  }
                >
                  {muted ? 'Unmute' : `Mute ${MUTE_DAYS}d`}
                </Button>
                <Button sm onClick={() => onDuplicate(s)}>
                  Duplicate
                </Button>
                <Button sm variant="secondary" onClick={() => { setActionError(null); setArming(s.id) }}>
                  Delete
                </Button>
              </div>
            )}
          </div>
        )
      })}
    </Screen>
  )
}
