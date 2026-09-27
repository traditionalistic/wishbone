import { useEffect, useState } from 'react'
import { ApiError, UNAUTHORIZED_EVENT, api, auth } from '../api'
import { Workspace } from './Workspace'

export function App() {
  const [authed, setAuthed] = useState(() => auth.get() !== '')
  useEffect(() => {
    const drop = () => {
      auth.clear()
      setAuthed(false)
    }
    window.addEventListener(UNAUTHORIZED_EVENT, drop)
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, drop)
  }, [])
  const logout = () => {
    auth.clear()
    setAuthed(false)
  }
  return authed ? <Workspace onLogout={logout} /> : <Login onDone={() => setAuthed(true)} />
}

function Login({ onDone }: { onDone: () => void }) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  return (
    <form
      className="login"
      onSubmit={async (e) => {
        e.preventDefault()
        setBusy(true)
        setError('')
        auth.set(password)
        try {
          await api.screens()
          onDone()
        } catch (err) {
          auth.clear()
          setError(err instanceof ApiError && err.status === 401 ? 'Wrong password' : `Server unavailable: ${(err as Error).message}`)
        } finally {
          setBusy(false)
        }
      }}
    >
      <h1>Wishbone</h1>
      <p className="dim">Screen editor</p>
      <input type="password" autoFocus placeholder="Admin password" value={password} onChange={(e) => setPassword(e.target.value)} />
      <button className="primary" disabled={!password || busy}>
        Sign in
      </button>
      {error && <div className="field-error">{error}</div>}
    </form>
  )
}
