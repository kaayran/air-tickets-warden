import { useState } from 'react'
import { type Subscription } from './api'
import { SettingsScreen } from './screens/SettingsScreen'
import { SubscriptionForm } from './screens/SubscriptionForm'
import { SubscriptionsList } from './screens/SubscriptionsList'

type Route =
  | { screen: 'list' }
  | { screen: 'form'; edit?: Subscription; seed?: Subscription }
  | { screen: 'settings' }

// App is the Mini App shell: a chart index of subscriptions, the flight-plan
// wizard, and settings, navigated with local state (three screens need no
// router). The form doubles as create, edit (edit), and duplicate (seed).
export default function App() {
  const [route, setRoute] = useState<Route>({ screen: 'list' })

  if (route.screen === 'form') {
    return (
      <SubscriptionForm edit={route.edit} seed={route.seed} onDone={() => setRoute({ screen: 'list' })} />
    )
  }

  return (
    <>
      {route.screen === 'list' ? (
        <SubscriptionsList
          onCreate={() => setRoute({ screen: 'form' })}
          onEdit={(sub) => setRoute({ screen: 'form', edit: sub })}
          onDuplicate={(sub) => setRoute({ screen: 'form', seed: sub })}
        />
      ) : (
        <SettingsScreen />
      )}
      <nav className="tabbar">
        <button
          type="button"
          className={`tabbar__item${route.screen === 'list' ? ' tabbar__item--active' : ''}`}
          onClick={() => setRoute({ screen: 'list' })}
        >
          Watches
        </button>
        <button
          type="button"
          className={`tabbar__item${route.screen === 'settings' ? ' tabbar__item--active' : ''}`}
          onClick={() => setRoute({ screen: 'settings' })}
        >
          Settings
        </button>
      </nav>
      <div style={{ height: 'calc(52px + env(safe-area-inset-bottom))' }} />
    </>
  )
}
