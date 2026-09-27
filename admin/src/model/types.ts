// Screen source format (spec §3.1) and admin API shapes (contract/api.md).

export type Json = null | boolean | number | string | Json[] | { [k: string]: Json }

export interface Repeat {
  over: string
  as?: string
  index?: string
  key?: string
}

export interface Drag {
  axis?: string
  snap?: number[]
  bounds?: number[]
}

export interface A11y {
  label?: string
  role?: 'button' | 'header' | 'image' | 'none'
  hidden?: boolean
}

export interface Node {
  type: string
  id: string
  props?: Record<string, Json>
  children?: Node[]
  when?: string
  repeat?: Repeat
  on?: Record<string, string>
  animate?: Record<string, string>
  transition?: { in?: string; out?: string }
  matchId?: string
  drag?: Drag
  a11y?: A11y
}

export type Ease = string | [number, number, number, number]

export interface Key {
  t: number
  v: number
  ease?: Ease
}

export interface Track {
  target: string
  prop: string
  stagger?: number
  keys: Key[]
}

export interface Timeline {
  name: string
  repeat?: number
  autoreverse?: boolean
  autoplay?: boolean
  tracks: Track[]
}

export interface Source {
  id: string
  title: string
  script: string
  timelines: Timeline[]
  root: Node
}

export interface Diag {
  path: string
  line: number
  col: number
  message: string
}

// ---- catalog ----

export interface PropSpec {
  type: string
  default?: Json
  required?: boolean
  doc?: string
}

export interface PrimitiveSpec {
  children: boolean
  props: Record<string, PropSpec>
  events: Record<string, string>
  doc?: string
  sugar?: Record<string, string>
}

export interface Primitives {
  events: Record<string, string>
  dragEvents: Record<string, string>
  common: Record<string, PropSpec>
  animatable: string[]
  timelineProps: string[]
  primitives: Record<string, PrimitiveSpec>
}

export interface Stdlib {
  functions: Record<string, { sig: string; doc?: string; method?: boolean; property?: boolean }>
  env: Record<string, string>
  effects: Record<string, { sig: string; doc?: string }>
  easings: Record<string, number[]>
}

export interface TemplateSpec {
  name: string
  params: { name: string; type: string; default?: Json }[]
  events: string[]
  slot: boolean
}

export type TokenGroup = 'color' | 'type' | 'space' | 'radius' | 'motion' | 'icon'

export interface Catalog {
  primitives: Primitives
  stdlib: Stdlib
  templates: TemplateSpec[]
  /** token names without the `@group.` prefix; `icon` is filled from `icons` */
  tokens: Record<TokenGroup, string[]>
  icons: string[]
  ops: { id: string; kind: 'read' | 'write'; args: string; result: string }[]
  brands: string[]
}

/** Cheap structural check for a source typed/pasted by hand (JSON tab). Returns an error or null. */
export function sourceError(v: unknown): string | null {
  if (!isObj(v)) return 'source must be an object'
  if (typeof v.script !== 'string') return '"script" must be a string'
  if (!Array.isArray(v.timelines)) return '"timelines" must be an array'
  return nodeError(v.root, 'root')
}

function nodeError(n: unknown, at: string): string | null {
  if (!isObj(n)) return `${at}: node must be an object`
  if (typeof n.type !== 'string' || !n.type) return `${at}: "type" must be a non-empty string`
  if (typeof n.id !== 'string') return `${at}: "id" must be a string`
  if (n.children === undefined) return null
  if (!Array.isArray(n.children)) return `${at}: "children" must be an array`
  for (let i = 0; i < n.children.length; i++) {
    const e = nodeError(n.children[i], `${at}/children/${i}`)
    if (e) return e
  }
  return null
}

function isObj(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}
