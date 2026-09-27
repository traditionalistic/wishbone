import { useRef, useState } from 'react'
import { api } from '../api'
import { isTemplate, propSpecs } from '../model/catalog'
import { diagsAt } from '../model/path'
import type { Path } from '../model/tree'
import type { Diag, Json, Node, PropSpec } from '../model/types'
import { CodeEditor } from './CodeEditor'
import { useCtx } from './ctx'
import { NodeFields } from './NodeFields'
import { PropField } from './PropField'

export function Inspector({ node, path, diags, onChange }: { node: Node | undefined; path: Path; diags: Diag[]; onChange: (n: Node) => void }) {
  const { catalog } = useCtx()
  if (!node) return <div className="empty">Select a node</div>

  const specs = propSpecs(node.type, catalog)
  const common = new Set(isTemplate(node.type) ? [] : Object.keys(catalog.primitives.common))
  const own = Object.entries(specs).filter(([k]) => !common.has(k))
  const shared = Object.entries(specs).filter(([k]) => common.has(k))
  const extra = Object.keys(node.props ?? {})
    .filter((k) => !(k in specs))
    .map((k): [string, PropSpec] => [k, { type: 'any', doc: 'not in the catalog for this type' }])
  const prim = catalog.primitives.primitives[node.type]
  const known = prim || catalog.templates.some((t) => t.name === node.type)

  const setProp = (k: string, v: Json | undefined) => {
    const props = { ...node.props }
    if (v === undefined) delete props[k]
    else props[k] = v
    const next: Node = { ...node, props }
    if (!Object.keys(props).length) delete next.props
    onChange(next)
  }

  const field = (name: string, spec: PropSpec) => (
    <PropField key={name} name={name} spec={spec} value={node.props?.[name]} onChange={(v) => setProp(name, v)} diags={diagsAt(diags, path, `props/${name}`)} field={`props/${name}`} />
  )
  const sharedOpen = shared.some(([k]) => node.props?.[k] !== undefined || diagsAt(diags, path, `props/${k}`).length > 0)

  return (
    <div className="inspector">
      <div className="insp-head">
        <span className="type">{node.type}</span>
        <span className="dim">#{node.id}</span>
        {isTemplate(node.type) && <TemplateSource name={node.type} />}
      </div>
      {!known && <div className="field-error">Unknown type “{node.type}”</div>}
      {prim?.doc && <div className="doc">{prim.doc}</div>}
      {diagsAt(diags, path, '').map((d, i) => (
        <div key={i} className="field-error">
          {d.message}
        </div>
      ))}
      <details className="section" open>
        <summary>{isTemplate(node.type) ? 'Params' : 'Props'}</summary>
        {own.length ? own.map(([k, s]) => field(k, s)) : <div className="dim">none</div>}
        {extra.map(([k, s]) => field(k, s))}
      </details>
      {shared.length > 0 && (
        <details className="section" open={sharedOpen}>
          <summary>Layout & style</summary>
          {shared.map(([k, s]) => field(k, s))}
        </details>
      )}
      <NodeFields node={node} path={path} diags={diags} onChange={onChange} />
    </div>
  )
}

/** Templates are edited in git (contract/templates); the admin only shows their source. */
function TemplateSource({ name }: { name: string }) {
  const dlg = useRef<HTMLDialogElement>(null)
  const [text, setText] = useState('')
  const open = async () => {
    dlg.current?.showModal()
    setText('loading…')
    try {
      setText(JSON.stringify(await api.template(name), null, 2))
    } catch (e) {
      setText(String((e as Error).message))
    }
  }
  return (
    <>
      <button className="link" onClick={open}>
        source
      </button>
      <dialog ref={dlg} className="dialog" onClick={(e) => e.target === dlg.current && dlg.current?.close()}>
        <div className="dialog-head">
          Template {name} <span className="dim">(read-only, edit in contract/templates)</span>
          <button className="icon" onClick={() => dlg.current?.close()}>
            ×
          </button>
        </div>
        <div className="dialog-body">{text && <CodeEditor value={text} lang="plain" />}</div>
      </dialog>
    </>
  )
}
