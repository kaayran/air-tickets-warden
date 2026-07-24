import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { init } from '@telegram-apps/sdk-react'
import { AppRoot } from '@telegram-apps/telegram-ui'
import '@telegram-apps/telegram-ui/dist/styles.css'
import { fetchSubscriptions } from './api'
import App from './App'

// Initialise the Telegram Mini App SDK. Safe to swallow when running outside
// Telegram (dev in a plain browser) — the app then shows a sign-in hint.
try {
  init()
} catch {
  // not running inside Telegram
}

const queryClient = new QueryClient()
// The subscriptions query is read by more than one screen; registering the
// fetcher once here keeps the screens free of wiring.
queryClient.setQueryDefaults(['subscriptions'], { queryFn: fetchSubscriptions, retry: false })

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <AppRoot>
        <App />
      </AppRoot>
    </QueryClientProvider>
  </StrictMode>,
)
