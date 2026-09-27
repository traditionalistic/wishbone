import { describe, expect, it } from 'vitest'
import { ApiError, type CompileResult } from '../api'
import type { Source } from '../model/types'
import { pushSource } from './usePreview'

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

  it('other failures propagate', async () => {
    const c = client(async () => {
      throw new ApiError(500, 'INTERNAL', 'boom')
    })
    await expect(pushSource('K7Q2', src, c)).rejects.toThrow('boom')
  })
})
