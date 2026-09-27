import { describe, expect, it } from 'vitest'
import primitives from '../../../contract/primitives.json'
import type { Catalog } from './types'
import { analyzeType, createNode, declaredNames, isContainer, nodeEvents, propSpecs, tokenOptions } from './catalog'

const catalog = {
  primitives,
  stdlib: { functions: {}, env: {}, effects: {}, easings: {} },
  templates: [
    { name: 'Button', params: [{ name: 'title', type: 'string' }, { name: 'style', type: '"primary" | "ghost"', default: 'primary' }], events: ['tap'], slot: false },
    { name: 'Card', params: [{ name: 'padding', type: 'number', default: 16 }], events: [], slot: true },
  ],
  tokens: { color: ['el16', 'bg1'], type: ['body'], space: ['sm', 'lg'], radius: ['md'], motion: ['snappy'], icon: ['close', 'star'] },
  icons: ['close', 'star'],
  ops: [],
  brands: ['korpu'],
} as unknown as Catalog

describe('analyzeType', () => {
  it('literal unions → enum', () => {
    expect(analyzeType('"v" | "h" | "z"')).toEqual({ base: 'enum', options: ['v', 'h', 'z'], groups: [] })
  })
  it('number with string options → number + options + geometry tokens', () => {
    expect(analyzeType('number | "fill" | "wrap"')).toEqual({ base: 'number', options: ['fill', 'wrap'], groups: ['space', 'radius'] })
    expect(analyzeType('number')).toEqual({ base: 'number', options: [], groups: ['space', 'radius'] })
  })
  it('token-only types', () => {
    expect(analyzeType('color')).toEqual({ base: 'token', options: [], groups: ['color'] })
    expect(analyzeType('paint')).toEqual({ base: 'token', options: [], groups: ['color'] })
    expect(analyzeType('textStyle')).toEqual({ base: 'token', options: [], groups: ['type'] })
    expect(analyzeType('icon')).toEqual({ base: 'token', options: [], groups: ['icon'] })
    expect(analyzeType('motion?')).toEqual({ base: 'token', options: [], groups: ['motion'] })
  })
  it('scalars and structured types', () => {
    expect(analyzeType('bool').base).toBe('bool')
    expect(analyzeType('string').base).toBe('string')
    expect(analyzeType('map<bool | number>').base).toBe('json')
    expect(analyzeType('{color: color, width: number}').base).toBe('json')
    expect(analyzeType('number | {top?: number, h?: number}')).toEqual({ base: 'json', options: [], groups: ['space', 'radius'] })
  })
})

describe('propSpecs', () => {
  it('primitive props + sugar + common', () => {
    const p = propSpecs('textField', catalog)
    expect(Object.keys(p).slice(0, 7)).toEqual(['value', 'placeholder', 'keyboard', 'secure', 'style', 'color', 'bind'])
    expect(p.padding.type).toContain('number')
  })
  it('template params followed by common props', () => {
    const p = propSpecs('Button', catalog)
    expect(Object.keys(p)).toEqual(['title', 'style', ...Object.keys(primitives.common)])
    expect(p.title).toEqual({ type: 'string', required: true })
    expect(p.style).toEqual({ type: '"primary" | "ghost"', default: 'primary', required: false })
    expect(p.opacity).toEqual(primitives.common.opacity)
  })
  it('a template param shadows the common prop of the same name', () => {
    const p = propSpecs('Card', catalog)
    expect(p.padding).toEqual({ type: 'number', default: 16, required: false })
    expect(Object.keys(p).filter((k) => k === 'padding')).toHaveLength(1)
    expect(Object.keys(p)[0]).toBe('padding')
  })
  it('optional-typed params without default are not required', () => {
    const c = { ...catalog, templates: [{ name: 'X', params: [{ name: 'icon', type: 'icon?' }], events: [], slot: false }] } as Catalog
    expect(propSpecs('X', c).icon.required).toBe(false)
    expect(createNode('X', c, new Set())).toEqual({ type: 'X', id: 'x' })
  })
  it('unknown type → common only', () => {
    expect(Object.keys(propSpecs('nope', catalog))).toEqual(Object.keys(primitives.common))
  })
})

describe('containers and events', () => {
  it('isContainer', () => {
    expect(isContainer('stack', catalog)).toBe(true)
    expect(isContainer('text', catalog)).toBe(false)
    expect(isContainer('Card', catalog)).toBe(true)
    expect(isContainer('Button', catalog)).toBe(false)
  })
  it('nodeEvents: common + primitive + dragEnd when draggable; template: declared + common, de-duplicated', () => {
    expect(nodeEvents({ type: 'textField', id: 'f' }, catalog)).toEqual(['tap', 'longPress', 'appear', 'disappear', 'change', 'submit'])
    expect(nodeEvents({ type: 'shape', id: 's', drag: { axis: 'x' } }, catalog)).toEqual(['tap', 'longPress', 'appear', 'disappear', 'dragEnd'])
    expect(nodeEvents({ type: 'Button', id: 'b' }, catalog)).toEqual(['tap', 'longPress', 'appear', 'disappear'])
    expect(nodeEvents({ type: 'Card', id: 'c', drag: { axis: 'y' } }, catalog)).toEqual(['tap', 'longPress', 'appear', 'disappear', 'dragEnd'])
  })
})

describe('createNode', () => {
  it('fills required props and a fresh id', () => {
    expect(createNode('text', catalog, new Set(['text']))).toEqual({ type: 'text', id: 'text1', props: { text: 'Text' } })
    expect(createNode('icon', catalog, new Set())).toEqual({ type: 'icon', id: 'icon', props: { name: '@icon.close' } })
    expect(createNode('grid', catalog, new Set())).toEqual({ type: 'grid', id: 'grid', props: { columns: 1 }, children: [] })
    expect(createNode('stack', catalog, new Set())).toEqual({ type: 'stack', id: 'stack', children: [] })
  })
  it('templates: required params without default, slot → children', () => {
    expect(createNode('Button', catalog, new Set())).toEqual({ type: 'Button', id: 'button', props: { title: 'Title' } })
    expect(createNode('Card', catalog, new Set())).toEqual({ type: 'Card', id: 'card', children: [] })
  })
})

describe('tokenOptions', () => {
  it('lists prefixed tokens per group', () => {
    expect(tokenOptions(['space', 'radius'], catalog)).toEqual([
      { group: 'space', values: ['@space.sm', '@space.lg'] },
      { group: 'radius', values: ['@radius.md'] },
    ])
  })
})

describe('declaredNames', () => {
  it('extracts top-level declarations', () => {
    const s = 'state page = 0\nlet last = page == 2\n  fn tax(x: number): number = x\nhandler next() {\n  let local = 1\n}\ndata me = demo.profile()\nparam id: string = ""\nevent tap\n// state commented = 1'
    expect(declaredNames(s)).toEqual([
      { name: 'page', kind: 'state' },
      { name: 'last', kind: 'let' },
      { name: 'tax', kind: 'fn' },
      { name: 'next', kind: 'handler' },
      { name: 'me', kind: 'data' },
      { name: 'id', kind: 'param' },
    ])
  })
})
