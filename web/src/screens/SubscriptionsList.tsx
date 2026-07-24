import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Badge,
  Button,
  ButtonCell,
  Cell,
  InlineButtons,
  List,
  Placeholder,
  Section,
  Spinner,
} from '@telegram-apps/telegram-ui'
import { deleteSubscription, patchSubscription, type Subscription } from '../api'
import { formatDateRange, formatPrice, formatRoute, isMuted, muteUntilISO } from '../lib/format'

const MUTE_DAYS = 3

export interface SubscriptionsListProps {
  onCreate: () => void
  onEdit: (sub: Subscription) => void
}

// SubscriptionsList is the home screen: every monitoring rule with its status
// badges and one-tap pause/resume, mute, and delete actions.
export function SubscriptionsList({ onCreate, onEdit }: SubscriptionsListProps) {
  const queryClient = useQueryClient()
  const { data: subs, error, isLoading } = useQuery<Subscription[]>({ queryKey: ['subscriptions'] })
  const [confirmingDelete, setConfirmingDelete] = useState<string | null>(null)

  // patchMutation optimistically rewrites the cached row — pause/resume/mute
  // must feel instant on the phone.
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
    onError: (_err, _vars, ctx) => queryClient.setQueryData(['subscriptions'], ctx?.previous),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['subscriptions'] }),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteSubscription,
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['subscriptions'] }),
  })

  if (isLoading) {
    return (
      <Placeholder>
        <Spinner size="l" />
      </Placeholder>
    )
  }
  if (error || !subs) {
    return (
      <Placeholder
        header="Could not load subscriptions"
        description={error instanceof Error ? error.message : 'Unknown error'}
      />
    )
  }
  if (subs.length === 0) {
    return (
      <Placeholder
        header="No subscriptions yet"
        description="Create one and the warden starts watching prices for you."
      >
        <Button size="l" onClick={onCreate}>
          New subscription
        </Button>
      </Placeholder>
    )
  }

  return (
    <List>
      <Section header="Subscriptions">
        <ButtonCell onClick={onCreate}>＋ New subscription</ButtonCell>
        {subs.map((s) => {
          const muted = isMuted(s)
          const paused = s.status === 'paused'
          return (
            <div key={s.id}>
              <Cell
                multiline
                onClick={() => onEdit(s)}
                subtitle={`${formatDateRange(s.date_from, s.date_to)}${
                  s.max_price_minor != null ? ` · below ${formatPrice(s.max_price_minor)}` : ''
                }`}
                after={
                  <span style={{ display: 'flex', gap: 4 }}>
                    {muted && (
                      <Badge type="number" mode="gray">
                        muted
                      </Badge>
                    )}
                    <Badge type="number" mode={s.status === 'active' ? 'primary' : s.status === 'paused' ? 'critical' : 'gray'}>
                      {s.status}
                    </Badge>
                  </span>
                }
              >
                {formatRoute(s)}
              </Cell>
              <InlineButtons mode="bezeled">
                <InlineButtons.Item
                  text={paused ? 'Resume' : 'Pause'}
                  onClick={() =>
                    patchMutation.mutate({ id: s.id, patch: { status: paused ? 'active' : 'paused' } })
                  }
                >
                  {paused ? '▶' : '⏸'}
                </InlineButtons.Item>
                <InlineButtons.Item
                  text={muted ? 'Unmute' : `Mute ${MUTE_DAYS}d`}
                  onClick={() =>
                    patchMutation.mutate({
                      id: s.id,
                      patch: { muted_until: muted ? null : muteUntilISO(MUTE_DAYS) },
                    })
                  }
                >
                  {muted ? '🔔' : '🔕'}
                </InlineButtons.Item>
                <InlineButtons.Item
                  mode="plain"
                  text={confirmingDelete === s.id ? 'Sure?' : 'Delete'}
                  onClick={() => {
                    if (confirmingDelete === s.id) {
                      setConfirmingDelete(null)
                      deleteMutation.mutate(s.id)
                    } else {
                      setConfirmingDelete(s.id)
                    }
                  }}
                >
                  🗑
                </InlineButtons.Item>
              </InlineButtons>
            </div>
          )
        })}
      </Section>
    </List>
  )
}
