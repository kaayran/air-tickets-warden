import { useState } from 'react'
import { Tabbar } from '@telegram-apps/telegram-ui'
import { type Subscription } from './api'
import { SettingsScreen } from './screens/SettingsScreen'
import { SubscriptionForm } from './screens/SubscriptionForm'
import { SubscriptionsList } from './screens/SubscriptionsList'

type Route =
  | { screen: 'list' }
  | { screen: 'form'; sub?: Subscription }
  | { screen: 'settings' }

// App is the Phase 1 Mini App: subscriptions list + create/edit form +
// settings, navigated with local state (three screens don't need a router).
export default function App() {
  const [route, setRoute] = useState<Route>({ screen: 'list' })

  if (route.screen === 'form') {
    return <SubscriptionForm initial={route.sub} onDone={() => setRoute({ screen: 'list' })} />
  }

  return (
    <>
      <div style={{ paddingBottom: 84 }}>
        {route.screen === 'list' ? (
          <SubscriptionsList
            onCreate={() => setRoute({ screen: 'form' })}
            onEdit={(sub) => setRoute({ screen: 'form', sub })}
          />
        ) : (
          <SettingsScreen />
        )}
      </div>
      <Tabbar>
        <Tabbar.Item text="Subscriptions" selected={route.screen === 'list'} onClick={() => setRoute({ screen: 'list' })}>
          ✈️
        </Tabbar.Item>
        <Tabbar.Item text="Settings" selected={route.screen === 'settings'} onClick={() => setRoute({ screen: 'settings' })}>
          ⚙️
        </Tabbar.Item>
      </Tabbar>
    </>
  )
}
