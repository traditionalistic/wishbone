import { describe, expect, it } from 'vitest'
import { ApiError, type CompileResult } from '../api'
import type { Source } from '../model/types'
import { openSession, pushSource } from './usePreview'

const src: Source = { id: 's', title: 't', script: '', timelines: [], root: { type: 'stack', id: 'root' } }
const ok: CompileResult = { ok: true, diags: [] }
const compiled: CompileResult = { ok: false, diags: [{ path: 'script', line: 1, col: 1, message: 'x' }] }

function client(push: () => Promise<CompileResult>) {
  const calls: string[] = []
  return {
    calls,
    pushDocument: async (code: string) => (calls.push(`push:${code}`), push()),
    compile: async () => (calls.push('compile'), compiled),
  }
}

describe('pushSource', () => {
  it('pushes to the live session', async () => {
    const c = client(async () => ok)
    expect(await pushSource('K7Q2', src, c)).toEqual({ result: ok, lost: false })
    expect(c.calls).toEqual(['push:K7Q2'])
  })

  it('without a session compiles only', async () => {
    const c = client(async () => ok)
    expect(await pushSource(null, src, c)).toEqual({ result: compiled, lost: false })
    expect(c.calls).toEqual(['compile'])
  })

  it('on 404 reports the session lost and still returns diagnostics via compile', async () => {
    const c = client(async () => {
      throw new ApiError(404, 'SESSION_NOT_FOUND', 'gone')
    })
    expect(await pushSource('K7Q2', src, c)).toEqual({ result: compiled, lost: true })
    expect(c.calls).toEqual(['push:K7Q2', 'compile'])
  })

  it('sends one push at a time; while one is in flight only the newest waiting source follows it', async () => {
    const log: string[] = []
    let finishA!: () => void
    const c = {
      pushDocument: async (_code: string, s: Source) => {
        log.push(`start:${s.id}`)
        if (s.id === 'A') await new Promise<void>((r) => (finishA = r))
        log.push(`end:${s.id}`)
        return ok
      },
      compile: async () => compiled,
    }
    const a = pushSource('K7Q2', { ...src, id: 'A' }, c)
    const b = pushSource('K7Q2', { ...src, id: 'B' }, c)
    const d = pushSource('K7Q2', { ...src, id: 'C' }, c)
    await Promise.resolve()
    expect(log).toEqual(['start:A']) // B and C wait for A: the hub applies pushes in arrival order
    finishA()
    await expect(a).resolves.toEqual({ result: ok, lost: false })
    await expect(b).resolves.toBeNull() // superseded by C while waiting
    await expect(d).resolves.toEqual({ result: ok, lost: false })
    expect(log).toEqual(['start:A', 'end:A', 'start:C', 'end:C'])
  })

  it('other failures propagate', async () => {
    const c = client(async () => {
      throw new ApiError(500, 'INTERNAL', 'boom')
    })
    await expect(pushSource('K7Q2', src, c)).rejects.toThrow('boom')
  })
})

describe('openSession', () => {
  function server(listed: () => Promise<{ code: string }[]>) {
    const calls: string[] = []
    return {
      calls,
      previewSessions: async () => (calls.push('list'), listed()),
      createPreview: async () => (calls.push('create'), { code: 'NEW1' }),
    }
  }

  it('reuses the stored code while the server lists it', async () => {
    const s = server(async () => [{ code: 'A1B2' }, { code: 'K7Q2' }])
    expect(await openSession('K7Q2', s)).toBe('K7Q2')
    expect(s.calls).toEqual(['list'])
  })

  it('creates a session when the stored code is gone', async () => {
    const s = server(async () => [{ code: 'A1B2' }])
    expect(await openSession('K7Q2', s)).toBe('NEW1')
    expect(s.calls).toEqual(['list', 'create'])
  })

  it('creates one without a stored code, and when the listing fails', async () => {
    const s = server(async () => [{ code: 'K7Q2' }])
    expect(await openSession(null, s)).toBe('NEW1')
    expect(s.calls).toEqual(['create'])
    const down = server(async () => {
      throw new Error('502')
    })
    expect(await openSession('K7Q2', down)).toBe('NEW1')
  })
})
