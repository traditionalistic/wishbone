import { createRoot } from 'react-dom/client'
import './styles.css'
import { App } from './ui/App'

const start = () => createRoot(document.getElementById('root')!).render(<App />)

// Dev-only fake backend (npm run dev:mock). The condition is a build-time constant, so production
// builds drop the import entirely.
if (import.meta.env.DEV && import.meta.env.VITE_MOCK === '1') void import('./mock').then(start)
else start()
