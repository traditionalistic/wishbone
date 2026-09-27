import { useEffect, useRef, useState, type ReactNode } from 'react'
import { analyzeType, tokenOptions } from '../model/catalog'
import { decodeValue, encodeExpr, encodeLiteral, switchMode, type Mode } from '../model/value'
import type { Diag, Json, PropSpec } from '../model/types'
import { CodeEditor } from './CodeEditor'
import { useCtx } from './ctx'

/** Label row + body + inline diagnostics; `field` is the diag path tail used for scroll-to. */
export function Field(p: { label: string; title?: string; field: string; diags: Diag[]; tools?: ReactNode; children: ReactNode }) {
  return (
    <div className={'field' + (p.diags.length ? ' has-error' : '')} data-field={p.field}>
      <div className="field-head">
        <label title={p.title}>{p.label}</label>
        {p.tools}
      </div>
      {p.children}
      {p.diags.map((d, i) => (
        <div key={i} className="field-error">
          {d.line ? `${d.line}:${d.col} ` : ''}
          {d.message}
        </div>
      ))}
    </div>
  )
}

/** Text input that keeps what the user typed ("1.") while committing parsed values. */
export function DraftInput(p: {
  value: string
  parse: (s: string) => { ok: true; v: Json | undefined } | { ok: false }
  format: (v: Json | undefined) => string
  onCommit: (v: Json | undefined) => void
  placeholder?: string
  list?: string
}) {
  const [text, setText] = useState(p.value)
  const [bad, setBad] = useState(false)
  const last = useRef(p.value)
  useEffect(() => {
    if (p.value !== last.current) {
      last.current = p.value
      setText(p.value)
      setBad(false)
    }
  }, [p.value])
  return (
    <input
      className={bad ? 'bad' : ''}
      value={text}
      placeholder={p.placeholder}
      list={p.list}
      onChange={(e) => {
        setText(e.target.value)
        const r = p.parse(e.target.value)
        setBad(!r.ok)
        if (r.ok) {
          last.current = p.format(r.v)
          p.onCommit(r.v)
        }
      }}
    />
  )
}

const fmt = (v: Json | undefined): string => (v === undefined ? '' : typeof v === 'string' ? v : JSON.stringify(v))

function parseNumberish(options: string[]) {
  return (s: string) => {
    const t = s.trim()
    if (t === '') return { ok: true as const, v: undefined }
    if (options.includes(t)) return { ok: true as const, v: t }
    const n = Number(t)
    return Number.isFinite(n) ? { ok: true as const, v: n } : { ok: false as const }
  }
}

function parseJson(s: string) {
  if (s.trim() === '') return { ok: true as const, v: undefined }
  try {
    return { ok: true as const, v: encodeLiteral(JSON.parse(s) as Json) }
  } catch {
    return { ok: false as const }
  }
}

const MODE_LABEL: Record<Mode, [string, string]> = {
  literal: ['L', 'Literal value'],
  token: ['@', 'Theme token'],
  expr: ['=', 'DSL expression'],
}

export function PropField(p: { name: string; spec: PropSpec; value: Json | undefined; onChange: (v: Json | undefined) => void; diags: Diag[]; field: string }) {
  const { catalog } = useCtx()
  const kind = analyzeType(p.spec.type)
  const tokens = tokenOptions(kind.groups, catalog)
  const d = p.value === undefined ? undefined : decodeValue(p.value)
  const mode: Mode = d?.mode ?? (kind.base === 'token' ? 'token' : 'literal')
  const modes: Mode[] = [...(kind.base !== 'token' ? (['literal'] as const) : []), ...(tokens.length ? (['token'] as const) : []), 'expr']
  const def = p.spec.default !== undefined ? fmt(p.spec.default) : ''
  const listId = kind.options.length && kind.base === 'number' ? `opts-${p.name}` : undefined

  let body: ReactNode
  if (d?.mode === 'expr') {
    body = <CodeEditor inline value={d.text} onChange={(t) => p.onChange(encodeExpr(t))} diags={p.diags} />
  } else if (mode === 'token') {
    const cur = d?.mode === 'token' ? d.text : ''
    const known = tokens.some((g) => g.values.includes(cur))
    body = (
      <select value={cur} onChange={(e) => p.onChange(e.target.value || undefined)}>
        <option value="">{def ? `default (${def})` : '—'}</option>
        {cur && !known && <option value={cur}>{cur} (unknown)</option>}
        {tokens.map((g) => (
          <optgroup key={g.group} label={g.group}>
            {g.values.map((v) => (
              <option key={v} value={v}>
                {v}
              </option>
            ))}
          </optgroup>
        ))}
      </select>
    )
  } else {
    const v = d?.mode === 'literal' ? d.value : undefined
    switch (kind.base) {
      case 'bool':
        body = <input type="checkbox" checked={Boolean(v ?? p.spec.default)} onChange={(e) => p.onChange(e.target.checked)} />
        break
      case 'enum':
        body = (
          <select value={typeof v === 'string' ? v : ''} onChange={(e) => p.onChange(e.target.value || undefined)}>
            <option value="">{def ? `default (${def})` : '—'}</option>
            {kind.options.map((o) => (
              <option key={o}>{o}</option>
            ))}
          </select>
        )
        break
      case 'number':
        body = (
          <>
            <DraftInput value={fmt(v)} parse={parseNumberish(kind.options)} format={fmt} onCommit={p.onChange} placeholder={def} list={listId} />
            {listId && (
              <datalist id={listId}>
                {kind.options.map((o) => (
                  <option key={o} value={o} />
                ))}
              </datalist>
            )}
          </>
        )
        break
      case 'string':
        body = <input value={typeof v === 'string' ? v : fmt(v)} placeholder={def} onChange={(e) => p.onChange(encodeLiteral(e.target.value))} />
        break
      default:
        body = <DraftInput value={v === undefined ? '' : JSON.stringify(v)} parse={parseJson} format={(x) => (x === undefined ? '' : JSON.stringify(x))} onCommit={p.onChange} placeholder={def || 'JSON'} />
    }
  }

  return (
    <Field
      label={p.name + (p.spec.required ? '*' : '')}
      title={p.spec.type + (p.spec.doc ? `\n${p.spec.doc}` : '')}
      field={p.field}
      diags={p.diags}
      tools={
        <span className="field-tools">
          <span className="modes">
            {modes.map((m) => (
              <button key={m} className={m === mode ? 'on' : ''} title={MODE_LABEL[m][1]} onClick={() => {
                  const next = switchMode(p.value, m, tokens[0]?.values[0])
                  if (next === undefined && d?.mode === 'expr' && d.text.trim() && !confirm('Discard this expression?')) return
                  p.onChange(next)
                }}>
                {MODE_LABEL[m][0]}
              </button>
            ))}
          </span>
          <button className="icon" title="Unset (use default)" disabled={p.value === undefined} onClick={() => p.onChange(undefined)}>
            ×
          </button>
        </span>
      }
    >
      {body}
    </Field>
  )
}
