import { useState } from 'react'
import type { Device } from '../api'
import type { Diag } from '../model/types'

export interface LogEntry {
  at: number
  device: string
  level: 'info' | 'warn' | 'error'
  message: string
}

export function Console({ diags, logs, devices, onDiag, onClear }: { diags: Diag[]; logs: LogEntry[]; devices: Device[]; onDiag: (d: Diag) => void; onClear: () => void }) {
  const [tab, setTab] = useState<'problems' | 'logs'>('problems')
  const deviceName = (id: string) => devices.find((d) => d.id === id)?.device ?? id
  return (
    <div className="console">
      <div className="tabs">
        <button className={tab === 'problems' ? 'on' : ''} onClick={() => setTab('problems')}>
          Problems <span className={diags.length ? 'count error' : 'count'}>{diags.length}</span>
        </button>
        <button className={tab === 'logs' ? 'on' : ''} onClick={() => setTab('logs')}>
          Device logs <span className="count">{logs.length}</span>
        </button>
        <span className="spacer" />
        {tab === 'logs' && logs.length > 0 && <button onClick={onClear}>Clear</button>}
      </div>
      <div className="console-body">
        {tab === 'problems' &&
          (diags.length ? (
            diags.map((d, i) => (
              <div key={i} className="console-row clickable" onClick={() => onDiag(d)}>
                <span className="lvl error">error</span>
                <span className="path">{d.path || '—'}</span>
                {d.line > 0 && (
                  <span className="dim">
                    {d.line}:{d.col}
                  </span>
                )}
                <span>{d.message}</span>
              </div>
            ))
          ) : (
            <div className="dim pad">No problems</div>
          ))}
        {tab === 'logs' &&
          (logs.length ? (
            logs.map((l, i) => (
              <div key={i} className="console-row">
                <span className="dim">{new Date(l.at).toLocaleTimeString()}</span>
                <span className={`lvl ${l.level}`}>{l.level}</span>
                <span className="path">{deviceName(l.device)}</span>
                <span>{l.message}</span>
              </div>
            ))
          ) : (
            <div className="dim pad">No device logs yet</div>
          ))}
      </div>
    </div>
  )
}
