import { useContext, useEffect, useRef } from 'react'
import { Annotation, EditorState } from '@codemirror/state'
import { EditorView, drawSelection, highlightActiveLine, keymap, lineNumbers, placeholder as cmPlaceholder } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { autocompletion, completionKeymap } from '@codemirror/autocomplete'
import { lintGutter, setDiagnostics, type Diagnostic } from '@codemirror/lint'
import { dslCompletion, dslLanguage } from './dsl'
import { Ctx } from './ctx'

export interface Mark {
  line: number
  col: number
  message: string
}

interface Props {
  value: string
  /** absent → read-only */
  onChange?: (v: string) => void
  lang?: 'dsl' | 'plain'
  /** compact field-like editor (inspector): no gutters, wrapping */
  inline?: boolean
  diags?: Mark[]
  /** move the caret here whenever `n` changes */
  focus?: { line: number; col: number; n: number }
  placeholder?: string
}

/** Marks our own value→doc syncs so they don't echo back through onChange. */
const External = Annotation.define<boolean>()

const theme = EditorView.theme(
  {
    '&': { color: 'var(--fg)', backgroundColor: 'var(--bg-input)', fontSize: '12px', height: '100%' },
    '.cm-content': { fontFamily: 'var(--mono)', caretColor: 'var(--fg)' },
    '.cm-gutters': { backgroundColor: 'var(--bg-panel)', color: 'var(--fg-dim)', border: 'none' },
    '.cm-activeLine': { backgroundColor: 'rgba(255,255,255,0.03)' },
    '&.cm-focused': { outline: 'none' },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground': { backgroundColor: 'rgba(110,150,255,0.25) !important' },
    '.cm-cursor': { borderLeftColor: 'var(--fg)' },
    '.cm-tooltip': { backgroundColor: 'var(--bg-raised)', border: '1px solid var(--border)', color: 'var(--fg)' },
    '.cm-tooltip-autocomplete ul li[aria-selected]': { backgroundColor: 'var(--accent)', color: '#fff' },
    '.cm-completionDetail': { color: 'var(--fg-dim)', marginLeft: '8px' },
    '.cm-tooltip-autocomplete ul li[aria-selected] .cm-completionDetail': { color: 'rgba(255,255,255,0.75)' },
  },
  { dark: true },
)

function pos(state: EditorState, line: number, col: number): number {
  const l = state.doc.line(Math.min(Math.max(line || 1, 1), state.doc.lines))
  return Math.min(l.from + Math.max(0, (col || 1) - 1), l.to)
}

export function CodeEditor(p: Props) {
  const host = useRef<HTMLDivElement>(null)
  const view = useRef<EditorView | null>(null)
  const onChange = useRef(p.onChange)
  onChange.current = p.onChange
  const ctx = useContext(Ctx)
  const dsl = useRef(ctx?.dsl)
  dsl.current = ctx?.dsl

  // Extensions are fixed at mount; value/diags/focus are synced by the effects below.
  useEffect(() => {
    const ext = [
      history(),
      drawSelection(),
      keymap.of([...completionKeymap, ...defaultKeymap, ...historyKeymap, indentWithTab]),
      theme,
      EditorState.readOnly.of(!onChange.current),
      EditorView.updateListener.of((u) => {
        if (u.docChanged && !u.transactions.some((t) => t.annotation(External))) onChange.current?.(u.state.doc.toString())
      }),
    ]
    if (p.inline) ext.push(EditorView.lineWrapping)
    else ext.push(lineNumbers(), highlightActiveLine(), lintGutter())
    if (p.lang !== 'plain')
      ext.push(dslLanguage, autocompletion({ override: [dslCompletion(() => dsl.current?.() ?? { all: [], methods: [] })] }))
    if (p.placeholder) ext.push(cmPlaceholder(p.placeholder))
    const v = new EditorView({ parent: host.current!, state: EditorState.create({ doc: p.value, extensions: ext }) })
    view.current = v
    return () => v.destroy()
  }, [])

  useEffect(() => {
    const v = view.current
    if (v && v.state.doc.toString() !== p.value)
      v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: p.value }, annotations: External.of(true) })
  }, [p.value])

  const diagKey = JSON.stringify(p.diags ?? [])
  useEffect(() => {
    const v = view.current
    if (!v) return
    const list: Diagnostic[] = (p.diags ?? []).map((d) => {
      const from = pos(v.state, d.line, d.col)
      const word = /^\w+/.exec(v.state.doc.sliceString(from, from + 64))
      return { from, to: from + (word ? word[0].length : 0), severity: 'error', message: d.message }
    })
    v.dispatch(setDiagnostics(v.state, list))
  }, [diagKey])

  useEffect(() => {
    const v = view.current
    if (!v || !p.focus) return
    v.dispatch({ selection: { anchor: pos(v.state, p.focus.line, p.focus.col) }, scrollIntoView: true })
    v.focus()
  }, [p.focus?.n])

  return <div ref={host} className={p.inline ? 'cm-inline' : 'cm-full'} />
}
