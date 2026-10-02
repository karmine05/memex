import { Dashboard } from './components/Dashboard'
import { LandingPage } from './components/LandingPage'

// Two entry points, one bundle — no router dependency:
//   /      → public landing page
//   /dash  → admin dashboard (asks for the mxa_ admin key)
function App() {
  const path = window.location.pathname.replace(/\/+$/, '') || '/'
  return path === '/dash' ? <Dashboard /> : <LandingPage />
}

export default App
