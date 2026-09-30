import { describe, expect, it } from 'vitest'
import type { Board, BoardScreen, Frame, Link } from '../api'
import { applyFrame, frameKey, layoutBoard, type Frames } from './board'

const nav = (to: string, via = 'node:x'): Link => ({ to, kind: 'navigate', via })
const pres = (to: string, via = 'node:x'): Link => ({ to, kind: 'present', via })
const scr = (id: string, links: Link[] = [], error?: string): BoardScreen => ({ id, title: id, links, ...(error ? { error } : {}) })
const frames = (...list: Frame[]): Frames => new Map(list.map((f) => [frameKey(f.screen, f.device), f]))
const ready = (screen: string, device: string, src = `data:image/jpeg;base64,${screen}`): Frame => ({ screen, device, status: 'ready', src, width: 402, height: 874, settled: true })

describe('applyFrame', () => {
  it('rendering keeps the previous picture and size, failed drops it', () => {
    const f0 = frames(ready('a', 'd1'))
    const f1 = applyFrame(f0, { type: 'frame', frame: { screen: 'a', device: 'd1', status: 'rendering' } })
    expect(f1.get(frameKey('a', 'd1'))).toEqual({ screen: 'a', device: 'd1', status: 'rendering', src: 'data:image/jpeg;base64,a', width: 402, height: 874, settled: true })
    const f2 = applyFrame(f1, { type: 'frame', frame: { screen: 'a', device: 'd1', status: 'failed', error: 'timeout' } })
    expect(f2.get(frameKey('a', 'd1'))).toEqual({ screen: 'a', device: 'd1', status: 'failed', error: 'timeout' })
    expect(f0.get(frameKey('a', 'd1'))?.status).toBe('ready')
  })

  it('a first rendering has no picture', () => {
    expect(applyFrame(new Map(), { type: 'frame', frame: { screen: 'a', device: 'd1', status: 'rendering' } }).get(frameKey('a', 'd1'))).toEqual({ screen: 'a', device: 'd1', status: 'rendering' })
  })

  it('frames replaces the set, keeping pictures under rendering', () => {
    const next = applyFrame(frames(ready('a', 'd1'), ready('b', 'd1')), {
      type: 'frames',
      list: [{ screen: 'a', device: 'd1', status: 'rendering' }, ready('c', 'd2')],
    })
    expect([...next.keys()]).toEqual([frameKey('a', 'd1'), frameKey('c', 'd2')])
    expect(next.get(frameKey('a', 'd1'))?.src).toBe('data:image/jpeg;base64,a')
  })

  it('a board drops frames of screens that left it', () => {
    const next = applyFrame(frames(ready('a', 'd1'), ready('b', 'd1'), ready('b', 'd2')), { type: 'board', board: { focus: 'a', screens: [scr('a')] } })
    expect([...next.keys()]).toEqual([frameKey('a', 'd1')])
  })

  it('devices drops frames of devices that left', () => {
    const next = applyFrame(frames(ready('a', 'd1'), ready('a', 'd2')), { type: 'devices', list: [{ id: 'd2', platform: 'android', device: 'Pixel 9', render: true }] })
    expect([...next.keys()]).toEqual([frameKey('a', 'd2')])
  })

  it('returns the same map when nothing changes', () => {
    const f = frames(ready('a', 'd1'))
    expect(applyFrame(f, { type: 'board', board: { focus: 'a', screens: [scr('a')] } })).toBe(f)
    expect(applyFrame(f, { type: 'log', device: 'd1', level: 'info', message: 'hi' })).toBe(f)
  })
})

const pos = (b: Board) => Object.fromEntries(layoutBoard(b).cards.map((c) => [c.id, [c.col, c.row]]))

describe('layoutBoard', () => {
  // the real raffle edges; the server orders screens by BFS from the focus
  const raffle = (focus: string, order: string[]): Board => {
    const all: Record<string, BoardScreen> = {
      'slot.feed.top': scr('slot.feed.top', [nav('raffle.main', 'node:root')]),
      'raffle.main': scr('raffle.main', [nav('raffle.invites', 'node:invites'), pres('raffle.rules', 'handler:rules'), nav('raffle.tickets', 'node:progress')]),
      'raffle.invites': scr('raffle.invites'),
      'raffle.rules': scr('raffle.rules'),
      'raffle.tickets': scr('raffle.tickets'),
    }
    return { focus, screens: order.map((id) => all[id]) }
  }
  const raffleLayout = {
    'slot.feed.top': [0, 0],
    'raffle.main': [1, 0],
    'raffle.invites': [2, 0],
    'raffle.rules': [2, 1],
    'raffle.tickets': [2, 2],
  }

  it('lays the raffle flow out by longest path, rows in discovery order', () => {
    const b = raffle('raffle.main', ['raffle.main', 'raffle.invites', 'raffle.rules', 'raffle.tickets', 'slot.feed.top'])
    expect(pos(b)).toEqual(raffleLayout)
    const l = layoutBoard(b)
    expect([l.cols, l.rows]).toEqual([3, 3])
    expect(l.arrows).toEqual([
      { from: 'slot.feed.top', to: 'raffle.main', kind: 'navigate', back: false },
      { from: 'raffle.main', to: 'raffle.invites', kind: 'navigate', back: false },
      { from: 'raffle.main', to: 'raffle.rules', kind: 'present', back: false },
      { from: 'raffle.main', to: 'raffle.tickets', kind: 'navigate', back: false },
    ])
  })

  it('does not move when another screen becomes the focus', () => {
    expect(pos(raffle('raffle.tickets', ['raffle.tickets', 'raffle.main', 'raffle.invites', 'raffle.rules', 'slot.feed.top']))).toEqual(raffleLayout)
    expect(pos(raffle('slot.feed.top', ['slot.feed.top', 'raffle.main', 'raffle.invites', 'raffle.rules', 'raffle.tickets']))).toEqual(raffleLayout)
  })

  it('orders several sources by id, not by board order', () => {
    const b = (order: string[]): Board => {
      const all: Record<string, BoardScreen> = { x: scr('x'), 'm.src': scr('m.src', [nav('x')]), 'k.src': scr('k.src', [nav('x')]) }
      return { focus: 'x', screens: order.map((id) => all[id]) }
    }
    const want = { 'k.src': [0, 0], 'm.src': [0, 1], x: [1, 0] }
    expect(pos(b(['x', 'm.src', 'k.src']))).toEqual(want)
    expect(pos(b(['x', 'k.src', 'm.src']))).toEqual(want)
  })

  it('lays out the showcase chain', () => {
    const b: Board = {
      focus: 'showcase.promo',
      screens: [scr('showcase.promo'), scr('showcase.onboarding', [nav('showcase.promo')]), scr('host.hello', [nav('showcase.onboarding')])],
    }
    expect(pos(b)).toEqual({ 'host.hello': [0, 0], 'showcase.onboarding': [1, 0], 'showcase.promo': [2, 0] })
  })

  it('flags an arrow into an earlier column as back, and a cycle without sources starts at the focus', () => {
    const b: Board = { focus: 'b', screens: [scr('b', [nav('c')]), scr('a', [nav('b')]), scr('c', [pres('a')])] }
    expect(pos(b)).toEqual({ b: [0, 0], c: [1, 0], a: [2, 0] })
    expect(layoutBoard(b).arrows.filter((a) => a.back)).toEqual([{ from: 'a', to: 'b', kind: 'navigate', back: true }])
  })

  it('a back link from a source-reached flow keeps the longest-path columns', () => {
    const b: Board = {
      focus: 'showcase.promo',
      screens: [scr('showcase.promo', [nav('showcase.onboarding')]), scr('showcase.onboarding', [nav('showcase.promo')]), scr('host.hello', [nav('showcase.onboarding')])],
    }
    expect(pos(b)).toEqual({ 'host.hello': [0, 0], 'showcase.onboarding': [1, 0], 'showcase.promo': [2, 0] })
    expect(layoutBoard(b).arrows.find((a) => a.from === 'showcase.promo')).toEqual({ from: 'showcase.promo', to: 'showcase.onboarding', kind: 'navigate', back: true })
  })

  it('places ghosts, merges duplicate links, skips self links and links off the board', () => {
    const b: Board = {
      focus: 'a',
      screens: [scr('a', [nav('a'), nav('gone', 'node:x'), nav('gone', 'node:y'), nav('beyond')]), scr('gone', [], 'screen not found')],
    }
    const l = layoutBoard(b)
    expect(pos(b)).toEqual({ a: [0, 0], gone: [1, 0] })
    expect(l.arrows).toEqual([{ from: 'a', to: 'gone', kind: 'navigate', back: false }])
  })

  it('a lone focus is one card', () => {
    expect(layoutBoard({ focus: 'a', screens: [scr('a')] })).toEqual({ cards: [{ id: 'a', col: 0, row: 0 }], arrows: [], cols: 1, rows: 1 })
  })
})
