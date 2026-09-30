import { describe, expect, it } from 'vitest'
import { parsePreviewMessage } from './api'

describe('parsePreviewMessage', () => {
  it('reads a ready frame; settled defaults to true', () => {
    expect(parsePreviewMessage({ type: 'frame', screen: 'raffle.main', device: 'd1', status: 'ready', src: 'data:image/jpeg;base64,/9j/', width: 402, height: 874 })).toEqual({
      type: 'frame',
      frame: { screen: 'raffle.main', device: 'd1', status: 'ready', src: 'data:image/jpeg;base64,/9j/', width: 402, height: 874, settled: true },
    })
  })

  it('drops a ready frame whose src is not an inline image or whose size is missing', () => {
    expect(parsePreviewMessage({ type: 'frame', screen: 's', device: 'd1', status: 'ready', src: 'https://evil.example/x.jpg', width: 1, height: 1 })).toBeNull()
    expect(parsePreviewMessage({ type: 'frame', screen: 's', device: 'd1', status: 'ready', src: 'data:image/jpeg;base64,', width: 0, height: 1 })).toBeNull()
  })

  it('keeps the valid entries of a board and a frames list', () => {
    const board = parsePreviewMessage({
      type: 'board',
      focus: 'raffle.main',
      screens: [
        { id: 'raffle.main', title: 'Розыгрыш', links: [{ to: 'raffle.tickets', kind: 'navigate', via: 'node:progress' }, { to: 'x', kind: 'open', via: 'node:y' }] },
        { id: 'raffle.gone', links: [], error: 'screen not found' },
        { title: 'no id' },
      ],
    })
    expect(board).toEqual({
      type: 'board',
      board: {
        focus: 'raffle.main',
        screens: [
          { id: 'raffle.main', title: 'Розыгрыш', links: [{ to: 'raffle.tickets', kind: 'navigate', via: 'node:progress' }] },
          { id: 'raffle.gone', title: '', links: [], error: 'screen not found' },
        ],
      },
    })
    expect(parsePreviewMessage({ type: 'frames', list: [{ screen: 's', device: 'd2', status: 'failed', error: 'timeout' }, { screen: 's', status: 'rendering' }] })).toEqual({
      type: 'frames',
      list: [{ screen: 's', device: 'd2', status: 'failed', error: 'timeout' }],
    })
  })

  it('marks devices without render: true as not rendering', () => {
    expect(parsePreviewMessage({ type: 'devices', list: [{ id: 'd1', platform: 'ios', device: 'iPhone 17 Pro' }, { id: 'd2', platform: 'android', device: 'Pixel 9', render: true }] })).toEqual({
      type: 'devices',
      list: [
        { id: 'd1', platform: 'ios', device: 'iPhone 17 Pro', render: false },
        { id: 'd2', platform: 'android', device: 'Pixel 9', render: true },
      ],
    })
  })

  it('rejects unknown types and non-objects', () => {
    expect(parsePreviewMessage({ type: 'nope' })).toBeNull()
    expect(parsePreviewMessage([])).toBeNull()
    expect(parsePreviewMessage(null)).toBeNull()
  })
})
