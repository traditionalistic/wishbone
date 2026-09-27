import { describe, expect, it } from 'vitest'
import { decodeValue, encodeExpr, encodeLiteral, encodeToken, jsonToDsl, switchMode } from './value'

describe('decodeValue', () => {
  it('expression: "=" prefix', () => {
    expect(decodeValue('="Hi, ${me.name}"')).toEqual({ mode: 'expr', text: '"Hi, ${me.name}"' })
  })
  it('token: "@" prefix', () => {
    expect(decodeValue('@color.el16')).toEqual({ mode: 'token', text: '@color.el16' })
  })
  it('escaped "\\=" and "\\@" are literal strings without the backslash', () => {
    expect(decodeValue('\\=5')).toEqual({ mode: 'literal', value: '=5' })
    expect(decodeValue('\\@home')).toEqual({ mode: 'literal', value: '@home' })
  })
  it('everything else is a literal', () => {
    expect(decodeValue('hello')).toEqual({ mode: 'literal', value: 'hello' })
    expect(decodeValue(12)).toEqual({ mode: 'literal', value: 12 })
    expect(decodeValue(true)).toEqual({ mode: 'literal', value: true })
    expect(decodeValue({ top: 4 })).toEqual({ mode: 'literal', value: { top: 4 } })
    expect(decodeValue('\\n')).toEqual({ mode: 'literal', value: '\\n' })
  })
})

describe('encode', () => {
  it('escapes literal strings that would read as expr/token', () => {
    expect(encodeLiteral('=5')).toBe('\\=5')
    expect(encodeLiteral('@home')).toBe('\\@home')
    expect(encodeLiteral('plain')).toBe('plain')
    expect(encodeLiteral(3)).toBe(3)
  })
  it('round-trips literals through decode', () => {
    for (const v of ['=x', '@y', 'z', '', 0, false, null, [1, 2], { a: '=b' }]) {
      expect(decodeValue(encodeLiteral(v))).toEqual({ mode: 'literal', value: v })
    }
  })
  it('expr and token', () => {
    expect(encodeExpr('a + 1')).toBe('=a + 1')
    expect(encodeToken('space', 'lg')).toBe('@space.lg')
  })
})

describe('jsonToDsl', () => {
  it('renders JSON literals as DSL source', () => {
    expect(jsonToDsl(12)).toBe('12')
    expect(jsonToDsl(true)).toBe('true')
    expect(jsonToDsl(null)).toBe('null')
    expect(jsonToDsl('say "hi"\n')).toBe('"say \\"hi\\"\\n"')
    expect(jsonToDsl([1, 'a'])).toBe('[1, "a"]')
    expect(jsonToDsl({ top: 4, 'x-y': 1 })).toBe('{top: 4, "x-y": 1}')
  })
})

describe('switchMode', () => {
  it('literal → expr keeps the value as DSL', () => {
    expect(switchMode('hi', 'expr')).toBe('="hi"')
    expect(switchMode(16, 'expr')).toBe('=16')
    expect(switchMode(undefined, 'expr')).toBe('=')
  })
  it('token → expr keeps the token (tokens are valid DSL)', () => {
    expect(switchMode('@space.lg', 'expr')).toBe('=@space.lg')
  })
  it('expr → literal parses a JSON-literal expression, else undefined', () => {
    expect(switchMode('=16', 'literal')).toBe(16)
    expect(switchMode('="=hi"', 'literal')).toBe('\\=hi')
    expect(switchMode('=a + 1', 'literal')).toBeUndefined()
  })
  it('→ token uses the first token offered, or keeps an existing token', () => {
    expect(switchMode(16, 'token', '@space.none')).toBe('@space.none')
    expect(switchMode('=@color.bg1', 'token', '@color.el1')).toBe('@color.bg1')
    expect(switchMode('@color.bg2', 'token', '@color.el1')).toBe('@color.bg2')
  })
  it('same mode is identity', () => {
    expect(switchMode('=x', 'expr')).toBe('=x')
    expect(switchMode(3, 'literal')).toBe(3)
  })
})
