// Prop values in the source (spec §3.1): "=expr" is a DSL expression, "@group.name" a token,
// anything else a JSON literal; a literal string starting with "=" or "@" is escaped as "\=" / "\@".
import type { Json } from './types'

export type Mode = 'literal' | 'token' | 'expr'

export type Decoded = { mode: 'literal'; value: Json } | { mode: 'token' | 'expr'; text: string }

export function decodeValue(v: Json): Decoded {
  if (typeof v === 'string') {
    if (v.startsWith('=')) return { mode: 'expr', text: v.slice(1) }
    if (v.startsWith('@')) return { mode: 'token', text: v }
    if (v.startsWith('\\=') || v.startsWith('\\@')) return { mode: 'literal', value: v.slice(1) }
  }
  return { mode: 'literal', value: v }
}

export function encodeLiteral(v: Json): Json {
  return typeof v === 'string' && (v.startsWith('=') || v.startsWith('@')) ? '\\' + v : v
}

export const encodeExpr = (text: string): string => '=' + text
export const encodeToken = (group: string, name: string): string => `@${group}.${name}`

const ident = /^[A-Za-z_][A-Za-z0-9_]*$/

/** JSON value → equivalent DSL literal source. */
export function jsonToDsl(v: Json): string {
  if (typeof v === 'string') return JSON.stringify(v)
  if (Array.isArray(v)) return `[${v.map(jsonToDsl).join(', ')}]`
  if (v && typeof v === 'object')
    return `{${Object.entries(v)
      .map(([k, x]) => `${ident.test(k) ? k : JSON.stringify(k)}: ${jsonToDsl(x)}`)
      .join(', ')}}`
  return String(v)
}

/**
 * Converts a stored prop value to another mode, keeping as much meaning as possible.
 * Returns undefined when the value can't be carried over (caller then unsets the prop).
 * `firstToken` is the token to fall back to when switching to token mode.
 */
export function switchMode(v: Json | undefined, to: Mode, firstToken?: string): Json | undefined {
  const d = v === undefined ? undefined : decodeValue(v)
  if (d?.mode === to) return v
  switch (to) {
    case 'expr':
      if (!d) return encodeExpr('')
      return encodeExpr(d.mode === 'literal' ? jsonToDsl(d.value) : d.text)
    case 'token':
      if (d?.mode === 'expr' && /^@\w+\.\w+$/.test(d.text.trim())) return d.text.trim()
      return firstToken
    case 'literal':
      if (d?.mode === 'expr') {
        try {
          return encodeLiteral(JSON.parse(d.text) as Json)
        } catch {
          return undefined
        }
      }
      return undefined
  }
}
