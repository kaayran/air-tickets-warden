import { useQuery } from '@tanstack/react-query'
import { Cell, List, Placeholder, Section, Spinner } from '@telegram-apps/telegram-ui'
import { fetchMe } from './api'

// App is the Phase 0 walking-skeleton shell: it authenticates via Telegram
// initData and renders the caller's settings from GET /api/v1/me. Real
// subscription screens arrive in Phase 1.
export default function App() {
  const { data, error, isLoading } = useQuery({
    queryKey: ['me'],
    queryFn: fetchMe,
    retry: false,
  })

  if (isLoading) {
    return (
      <Placeholder>
        <Spinner size="l" />
      </Placeholder>
    )
  }

  if (error || !data) {
    return (
      <Placeholder
        header="Could not load your data"
        description={error instanceof Error ? error.message : 'Unknown error'}
      />
    )
  }

  return (
    <List>
      <Section
        header="Air Tickets Warden"
        footer="Phase 0 shell — data from GET /api/v1/me"
      >
        <Cell subtitle="Chat ID">{data.chat_id}</Cell>
        <Cell subtitle="Cooldown hours">{data.cooldown_hours ?? '—'}</Cell>
        <Cell subtitle="Drop %">{data.drop_pct ?? '—'}</Cell>
        <Cell subtitle="Stable price band %">{data.stable_price_band_pct ?? '—'}</Cell>
      </Section>
    </List>
  )
}
