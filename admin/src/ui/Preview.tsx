// Preview column: the open screen as the session's iOS and Android devices render it. `Shot` (one
// platform's picture of a screen in every state) is shared with the storyboard cards.
import type { CSSProperties } from 'react'
import type { Device, Frame } from '../api'

/** platform → the device id picked in its slot */
export type Picks = Record<string, string>

export const PLATFORMS = [
  { id: 'ios', name: 'iOS', where: 'на iPhone или симуляторе', size: [402, 874] },
  { id: 'android', name: 'Android', where: 'на Android-телефоне или эмуляторе', size: [412, 915] },
] as const

export type Platform = (typeof PLATFORMS)[number]

/** The picked device of the platform, else its first that renders, else its first. */
export function slotDevice(devices: Device[], platform: string, picks: Picks): Device | undefined {
  const own = devices.filter((d) => d.platform === platform)
  return own.find((d) => d.id === picks[platform]) ?? own.find((d) => d.render) ?? own[0]
}

export function Shot(p: {
  screen: string
  platform: Platform
  device: Device | undefined
  frame: Frame | undefined
  code: string | null
  /** the source does not compile: the picture is of the last good one */
  stale?: boolean
  onRetry?: () => void
  /** storyboard card: the "not connected" note without instructions */
  compact?: boolean
}) {
  const { device, frame, platform } = p
  const [w, h] = frame?.width && frame.height ? [frame.width, frame.height] : platform.size
  const ar = { '--ar': `${w} / ${h}` } as CSSProperties
  if (!device || !device.render)
    return (
      <div className="phone note" style={ar}>
        <p className="shot-note">
          {device ? (
            `${device.device} — старый Playground: обновите, чтобы видеть снимки здесь`
          ) : (
            <>
              {platform.name} не подключён
              {!p.compact && (
                <>
                  . Откройте Playground {platform.where} → сессия <b className="code">{p.code ?? '····'}</b>
                </>
              )}
            </>
          )}
        </p>
      </div>
    )
  return (
    <div className="phone" style={ar}>
      {frame?.src ? (
        <img src={frame.src} alt={`${p.screen} на ${device.device}`} />
      ) : frame?.status === 'failed' ? (
        <div className="shot-fail">
          <span>
            Не отрендерилось на {device.device}: {frame.error || 'unknown error'}
          </span>
          <button
            onClick={(e) => {
              e.stopPropagation()
              p.onRetry?.()
            }}
            disabled={!p.onRetry}
          >
            Retry
          </button>
        </div>
      ) : (
        <div className="skeleton">Рендер на {device.device}…</div>
      )}
      {frame?.status === 'rendering' && frame.src && <span className="spinner" role="status" aria-label={`Рендер на ${device.device}`} />}
      {frame?.src && (p.stale || !frame.settled) && (
        <span className="tags">
          {!frame.settled && <span className="tag">снято в движении</span>}
          {p.stale && <span className="tag">последняя удачная компиляция</span>}
        </span>
      )}
    </div>
  )
}

export function Preview(p: {
  screen: string
  devices: Device[]
  picks: Picks
  onPick: (platform: string, device: string) => void
  frame: (screen: string, device: string) => Frame | undefined
  code: string | null
  stale: boolean
  onRetry?: () => void
}) {
  return (
    <aside className="preview-col" aria-label="Preview">
      {PLATFORMS.map((platform) => {
        const own = p.devices.filter((d) => d.platform === platform.id)
        const device = slotDevice(p.devices, platform.id, p.picks)
        return (
          <section key={platform.id} className="slot" aria-label={platform.name}>
            <div className="slot-head">
              <b>{platform.name}</b>
              {device && own.length > 1 ? (
                <select aria-label={`${platform.name} device`} value={device.id} onChange={(e) => p.onPick(platform.id, e.target.value)}>
                  {own.map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.device}
                    </option>
                  ))}
                </select>
              ) : (
                <span className="dim">{device?.device}</span>
              )}
            </div>
            <div className="shot-box">
              <Shot screen={p.screen} platform={platform} device={device} frame={device && p.frame(p.screen, device.id)} code={p.code} stale={p.stale} onRetry={p.onRetry} />
            </div>
          </section>
        )
      })}
    </aside>
  )
}
