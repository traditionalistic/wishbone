// DSL support for CodeMirror: a stream highlighter (spec §3.2) and completions from the catalog.
import { HighlightStyle, StreamLanguage, syntaxHighlighting } from '@codemirror/language'
import type { Completion, CompletionSource } from '@codemirror/autocomplete'
import { tags } from '@lezer/highlight'
import { declaredNames } from '../model/catalog'
import type { Catalog, Source } from '../model/types'

const KEYWORDS = new Set('state let fn handler data param event if else await animate emit call required'.split(' '))
const ATOMS = new Set(['true', 'false', 'null'])

/** Stack of string / interpolation frames: 'str' = inside "…", number = brace depth inside ${…}. */
interface St {
  stack: ('str' | number)[]
}

const dslParser = StreamLanguage.define<St>({
  name: 'wishbone-dsl',
  startState: () => ({ stack: [] }),
  copyState: (s) => ({ stack: [...s.stack] }),
  token(stream, st) {
    const top = st.stack[st.stack.length - 1]
    if (top === 'str') {
      if (stream.match('${')) {
        st.stack.push(0)
        return 'interp'
      }
      while (!stream.eol()) {
        if (stream.match('${', false)) return 'string'
        const ch = stream.next()
        if (ch === '\\') stream.next()
        else if (ch === '"') {
          st.stack.pop()
          return 'string'
        }
      }
      return 'string'
    }
    if (stream.eatSpace()) return null
    if (stream.match('//')) {
      stream.skipToEnd()
      return 'comment'
    }
    if (stream.eat('"')) {
      st.stack.push('str')
      return 'string'
    }
    if (typeof top === 'number') {
      if (stream.peek() === '{') st.stack[st.stack.length - 1] = top + 1
      else if (stream.peek() === '}') {
        if (top === 0) {
          stream.next()
          st.stack.pop()
          return 'interp'
        }
        st.stack[st.stack.length - 1] = top - 1
      }
    }
    if (stream.match(/^@[A-Za-z_]\w*(\.[A-Za-z_]\w*)?/)) return 'dslToken'
    if (stream.match(/^\d+(\.\d+)?(ms|s)?(?![\w.])/)) return 'number'
    if (stream.match(/^[A-Za-z_]\w*/)) {
      const w = stream.current()
      if (KEYWORDS.has(w)) return 'keyword'
      if (ATOMS.has(w)) return 'bool'
      return stream.match(/^\s*\(/, false) ? 'fnName' : 'variableName'
    }
    stream.next()
    return /[=+\-*/%<>!&|?:]/.test(stream.current()) ? 'operator' : null
  },
  tokenTable: {
    dslToken: tags.special(tags.variableName),
    fnName: tags.function(tags.variableName),
    interp: tags.processingInstruction,
  },
})

const style = HighlightStyle.define([
  { tag: tags.keyword, color: 'var(--syn-keyword)' },
  { tag: tags.string, color: 'var(--syn-string)' },
  { tag: tags.number, color: 'var(--syn-number)' },
  { tag: tags.comment, color: 'var(--syn-comment)', fontStyle: 'italic' },
  { tag: tags.bool, color: 'var(--syn-number)' },
  { tag: tags.special(tags.variableName), color: 'var(--syn-token)' },
  { tag: tags.function(tags.variableName), color: 'var(--syn-fn)' },
  { tag: tags.processingInstruction, color: 'var(--syn-op)' },
  { tag: tags.operator, color: 'var(--syn-op)' },
])

export const dslLanguage = [dslParser, syntaxHighlighting(style)]

/** Everything the DSL can name, for autocomplete. */
export function dslOptions(c: Catalog, src: Source | null): { all: Completion[]; methods: Completion[] } {
  const fns = Object.entries(c.stdlib.functions)
  const all: Completion[] = [
    ...[...KEYWORDS, ...ATOMS].map((label) => ({ label, type: 'keyword' })),
    ...fns.map(([label, f]) => ({ label, type: 'function', detail: f.sig, info: f.doc })),
    ...Object.entries(c.stdlib.effects).map(([label, f]) => ({ label, type: 'function', detail: f.sig, info: f.doc, boost: -1 })),
    ...Object.entries(c.stdlib.env).map(([k, t]) => ({ label: `env.${k}`, type: 'property', detail: t })),
    ...Object.entries(c.tokens).flatMap(([g, ns]) => ns.map((n) => ({ label: `@${g}.${n}`, type: 'constant' }))),
    ...c.ops.map((o) => ({ label: o.id, type: 'method', detail: `${o.kind} ${o.args} → ${o.result}` })),
    ...declaredNames(src?.script ?? '').map((d) => ({ label: d.name, type: 'variable', detail: d.kind, boost: 2 })),
    ...(src?.timelines ?? []).map((t) => ({ label: t.name, type: 'variable', detail: 'timeline' })),
    ...['event', 'self.pressed', 'item', 'index'].map((label) => ({ label, type: 'variable' })),
  ]
  const methods = fns.filter(([, f]) => f.method).map(([label, f]) => ({ label, type: 'method', detail: f.sig, info: f.doc }))
  return { all, methods }
}

export function dslCompletion(get: () => { all: Completion[]; methods: Completion[] }): CompletionSource {
  return (ctx) => {
    const w = ctx.matchBefore(/@?[\w.]*/)
    if (!w || (w.from === w.to && !ctx.explicit)) return null
    const { all, methods } = get()
    const dot = w.text.lastIndexOf('.')
    if (dot >= 0 && !w.text.startsWith('@')) {
      const head = w.text.slice(0, dot + 1)
      const qualified = all.filter((o) => o.label.startsWith(head))
      // `env.` / `demo.` complete as whole names; anything else after a dot is a method call
      if (qualified.length) return { from: w.from, options: qualified }
      return { from: w.from + dot + 1, options: methods, validFor: /^\w*$/ }
    }
    return { from: w.from, options: all, validFor: /^@?[\w.]*$/ }
  }
}
