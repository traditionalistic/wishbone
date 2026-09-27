import { useRef, useState, type DragEvent, type KeyboardEvent, type ReactNode } from 'react'
import { createNode, isContainer } from '../model/catalog'
import { collectIds, duplicateNode, getNode, insertNode, moveNode, removeNode, type Path } from '../model/tree'
import type { Node } from '../model/types'
import { useCtx } from './ctx'

type Drag = { kind: 'move'; path: Path } | { kind: 'new'; type: string }
type Zone = 'before' | 'inside' | 'after'

interface Props {
  root: Node
  selected: Path
  diagPaths: Set<string>
  onSelect: (p: Path) => void
  onChange: (root: Node, select?: Path) => void
}

export function TreePanel({ root, selected, diagPaths, onSelect, onChange }: Props) {
  const { catalog } = useCtx()
  const drag = useRef<Drag | null>(null)
  const [hint, setHint] = useState<{ key: string; zone: Zone } | null>(null)
  const selKey = selected.join('.')

  const insertAt = (type: string, parent: Path, index: number) => {
    const node = createNode(type, catalog, collectIds(root))
    const count = getNode(root, parent)?.children?.length ?? 0
    onChange(insertNode(root, parent, index, node), [...parent, Math.min(index, count)])
  }

  /** Palette click: into the selected container, else right after the selected node. */
  const insertNear = (type: string) => {
    const sel = getNode(root, selected)
    if (sel && isContainer(sel.type, catalog)) insertAt(type, selected, sel.children?.length ?? 0)
    else if (selected.length) insertAt(type, selected.slice(0, -1), selected[selected.length - 1] + 1)
  }

  const remove = () => {
    if (!selected.length) return
    const parent = selected.slice(0, -1)
    const next = removeNode(root, selected)
    const siblings = getNode(next, parent)?.children?.length ?? 0
    const i = Math.min(selected[selected.length - 1], siblings - 1)
    onChange(next, i >= 0 ? [...parent, i] : parent)
  }

  const duplicate = () => {
    if (!selected.length) return
    const d = duplicateNode(root, selected)
    onChange(d.root, d.path)
  }

  const zoneOf = (e: DragEvent, node: Node, path: Path): Zone => {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const y = (e.clientY - r.top) / r.height
    const container = isContainer(node.type, catalog)
    if (path.length === 0) return 'inside'
    if (container && y > 0.25 && y < 0.75) return 'inside'
    return y < 0.5 ? 'before' : 'after'
  }

  const drop = (e: DragEvent, node: Node, path: Path) => {
    e.preventDefault()
    setHint(null)
    const d = drag.current
    drag.current = null
    if (!d) return
    const zone = zoneOf(e, node, path)
    const parent = zone === 'inside' ? path : path.slice(0, -1)
    const index = zone === 'inside' ? (node.children?.length ?? 0) : path[path.length - 1] + (zone === 'after' ? 1 : 0)
    if (zone === 'inside' && !isContainer(node.type, catalog)) return
    if (d.kind === 'new') return insertAt(d.type, parent, index)
    const m = moveNode(root, d.path, parent, index)
    if (m) onChange(m.root, m.path)
  }

  const row = (node: Node, path: Path): ReactNode => {
    const key = path.join('.')
    const text = node.props?.text ?? node.props?.title
    return (
      <div key={key}>
        <div
          className={['tree-row', key === selKey && 'sel', hint?.key === key && `drop-${hint.zone}`].filter(Boolean).join(' ')}
          style={{ paddingLeft: 8 + path.length * 14 }}
          draggable={path.length > 0}
          onClick={() => onSelect(path)}
          onDragStart={(e) => {
            drag.current = { kind: 'move', path }
            e.dataTransfer.setData('text/plain', node.id)
            e.dataTransfer.effectAllowed = 'move'
          }}
          onDragOver={(e) => {
            e.preventDefault()
            const zone = zoneOf(e, node, path)
            if (hint?.key !== key || hint.zone !== zone) setHint({ key, zone })
          }}
          onDragLeave={() => setHint(null)}
          onDrop={(e) => drop(e, node, path)}
        >
          {diagPaths.has(key) && <span className="diag-dot" title="Has diagnostics" />}
          <span className={/^[A-Z]/.test(node.type) ? 'type tpl' : 'type'}>{node.type}</span>
          <span className="id">#{node.id}</span>
          {typeof text === 'string' && <span className="preview">{text}</span>}
          {node.when && <span className="badge" title={`when: ${node.when}`}>if</span>}
          {node.repeat && <span className="badge" title={`repeat over ${node.repeat.over}`}>×{node.repeat.over}</span>}
          {node.on && Object.keys(node.on).length > 0 && <span className="badge dim" title={Object.keys(node.on).join(', ')}>on</span>}
        </div>
        {node.children?.map((c, i) => row(c, [...path, i]))}
      </div>
    )
  }

  const keys = (e: KeyboardEvent) => {
    if (e.target !== e.currentTarget) return
    if (e.key === 'Delete' || e.key === 'Backspace') remove()
    else if (e.key === 'd' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      duplicate()
    }
  }

  const paletteItem = (type: string, tpl: boolean) => (
    <button
      key={type}
      className={tpl ? 'chip tpl' : 'chip'}
      draggable
      title={tpl ? 'template' : catalog.primitives.primitives[type]?.doc}
      onClick={() => insertNear(type)}
      onDragStart={(e) => {
        drag.current = { kind: 'new', type }
        e.dataTransfer.setData('text/plain', type)
      }}
    >
      {type}
    </button>
  )

  return (
    <div className="tree-panel">
      <div className="panel-head">
        Tree
        <span className="spacer" />
        <button disabled={!selected.length} onClick={duplicate} title="Duplicate (⌘D)">
          Dup
        </button>
        <button disabled={!selected.length} onClick={remove} title="Delete (⌫)">
          Del
        </button>
      </div>
      <div className="tree" tabIndex={0} onKeyDown={keys}>
        {row(root, [])}
      </div>
      <div className="palette">
        <div className="panel-sub">Primitives</div>
        <div className="chips">{Object.keys(catalog.primitives.primitives).map((t) => paletteItem(t, false))}</div>
        <div className="panel-sub">Templates</div>
        <div className="chips">{catalog.templates.length ? catalog.templates.map((t) => paletteItem(t.name, true)) : <span className="dim">none</span>}</div>
      </div>
    </div>
  )
}
