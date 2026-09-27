// Inspector sections for the node-level fields (spec §2.1 "общие поля узла").
import type { ReactNode } from 'react'
import { nodeEvents } from '../model/catalog'
import { diagsAt } from '../model/path'
import type { Path } from '../model/tree'
import type { A11y, Diag, Drag, Json, Node, Repeat } from '../model/types'
import { CodeEditor } from './CodeEditor'
import { useCtx } from './ctx'
import { DraftInput, Field } from './PropField'

type Obj = Record<string, unknown>

/** Sets/removes an optional key, dropping empty strings and empty objects. */
function put<T extends Obj>(o: T, k: string, v: unknown): T {
  const c: Obj = { ...o }
  const empty = v === undefined || v === '' || (typeof v === 'object' && v !== null && !Array.isArray(v) && Object.keys(v).length === 0)
  if (empty) delete c[k]
  else c[k] = v
  return c as T
}

const Dsl = (p: { value: string | undefined; onChange: (v: string) => void; diags: Diag[]; placeholder?: string }) => (
  <CodeEditor inline value={p.value ?? ''} onChange={p.onChange} diags={p.diags} placeholder={p.placeholder} />
)

const Section = (p: { title: string; children: ReactNode; open?: boolean }) => (
  <details className="section" open={p.open ?? true}>
    <summary>{p.title}</summary>
    {p.children}
  </details>
)

const numList = (s: string) => {
  const parts = s.split(',').map((x) => x.trim()).filter(Boolean)
  const ns = parts.map(Number)
  return ns.every(Number.isFinite) ? { ok: true as const, v: ns.length ? (ns as Json) : undefined } : { ok: false as const }
}
const fmtList = (v: Json | undefined) => (Array.isArray(v) ? v.join(', ') : '')

export function NodeFields({ node, path, diags, onChange }: { node: Node; path: Path; diags: Diag[]; onChange: (n: Node) => void }) {
  const { catalog } = useCtx()
  const at = (field: string) => diagsAt(diags, path, field)
  const set = (k: keyof Node, v: unknown) => onChange(put(node as unknown as Obj, k, v) as unknown as Node)
  const setIn = (k: 'on' | 'animate', key: string, v: string | undefined) => set(k, put(node[k] ?? {}, key, v))
  const events = nodeEvents(node, catalog)
  const animatable = ['*', ...catalog.primitives.animatable]
  const rep = node.repeat
  const setRep = (k: keyof Repeat, v: string) => set('repeat', put(rep as unknown as Obj, k, v))
  const drag = node.drag
  const setDrag = (k: keyof Drag, v: unknown) => set('drag', put((drag ?? {}) as Obj, k, v))
  const a11y = node.a11y
  const setA11y = (k: keyof A11y, v: unknown) => set('a11y', put((a11y ?? {}) as Obj, k, v))

  return (
    <>
      <Section title="Node">
        <Field label="id" field="id" diags={at('id')}>
          <input value={node.id} onChange={(e) => onChange({ ...node, id: e.target.value })} />
        </Field>
        <Field label="when" title="bool expression; false ⇒ node absent" field="when" diags={at('when')}>
          <Dsl value={node.when} onChange={(v) => set('when', v)} diags={at('when')} placeholder="always" />
        </Field>
        <Field label="matchId" title="shared element id between `when` branches" field="matchId" diags={at('matchId')}>
          <Dsl value={node.matchId} onChange={(v) => set('matchId', v)} diags={at('matchId')} />
        </Field>
      </Section>

      <Section title="Repeat" open={!!rep}>
        {rep ? (
          <>
            <Field label="over" field="repeat/over" diags={at('repeat/over')} tools={<button className="icon" title="Remove repeat" onClick={() => set('repeat', undefined)}>×</button>}>
              <Dsl value={rep.over} onChange={(v) => set('repeat', { ...rep, over: v })} diags={at('repeat/over')} />
            </Field>
            <div className="row2">
              <Field label="as" field="repeat/as" diags={at('repeat/as')}>
                <input value={rep.as ?? ''} placeholder="item" onChange={(e) => setRep('as', e.target.value)} />
              </Field>
              <Field label="index" field="repeat/index" diags={at('repeat/index')}>
                <input value={rep.index ?? ''} placeholder="index" onChange={(e) => setRep('index', e.target.value)} />
              </Field>
            </div>
            <Field label="key" field="repeat/key" diags={at('repeat/key')}>
              <Dsl value={rep.key} onChange={(v) => setRep('key', v)} diags={at('repeat/key')} placeholder="index" />
            </Field>
          </>
        ) : (
          <button onClick={() => set('repeat', { over: '' })}>+ repeat</button>
        )}
      </Section>

      <Section title="Events">
        {Object.entries(node.on ?? {}).map(([ev, code]) => (
          <Field key={ev} label={`on.${ev}`} field={`on/${ev}`} diags={at(`on/${ev}`)} tools={<button className="icon" onClick={() => setIn('on', ev, undefined)}>×</button>}>
            <Dsl value={code} onChange={(v) => set('on', { ...node.on, [ev]: v })} diags={at(`on/${ev}`)} />
          </Field>
        ))}
        <AddSelect label="+ event" options={events.filter((e) => !(e in (node.on ?? {})))} onPick={(ev) => set('on', { ...node.on, [ev]: '' })} />
      </Section>

      <Section title="Motion" open={!!(node.animate || node.transition)}>
        {Object.entries(node.animate ?? {}).map(([prop, m]) => (
          <Field key={prop} label={`animate.${prop}`} title="motion expression" field={`animate/${prop}`} diags={at(`animate/${prop}`)} tools={<button className="icon" onClick={() => setIn('animate', prop, undefined)}>×</button>}>
            <Dsl value={m} onChange={(v) => set('animate', { ...node.animate, [prop]: v })} diags={at(`animate/${prop}`)} />
          </Field>
        ))}
        <AddSelect
          label="+ animate"
          options={animatable.filter((a) => !(a in (node.animate ?? {})))}
          onPick={(prop) => set('animate', { ...node.animate, [prop]: `@motion.${catalog.tokens.motion[0] ?? 'standard'}` })}
        />
        <div className="row2">
          <Field label="transition.in" title="fade + slide(bottom) + scale(0.9) + offset(x, y)" field="transition/in" diags={at('transition/in')}>
            <input value={node.transition?.in ?? ''} placeholder="fade" onChange={(e) => set('transition', put(node.transition ?? {}, 'in', e.target.value))} />
          </Field>
          <Field label="transition.out" field="transition/out" diags={at('transition/out')}>
            <input value={node.transition?.out ?? ''} placeholder="fade" onChange={(e) => set('transition', put(node.transition ?? {}, 'out', e.target.value))} />
          </Field>
        </div>
      </Section>

      <Section title="Drag" open={!!drag}>
        <div className="row3">
          <Field label="axis" field="drag" diags={at('drag')}>
            <select value={drag?.axis ?? ''} onChange={(e) => (e.target.value ? setDrag('axis', e.target.value) : set('drag', undefined))}>
              <option value="">none</option>
              <option>x</option>
              <option>y</option>
              <option>both</option>
            </select>
          </Field>
          <Field label="snap" title="comma-separated offsets" field="drag/snap" diags={[]}>
            <DraftInput value={fmtList(drag?.snap)} parse={numList} format={fmtList} onCommit={(v) => setDrag('snap', v)} placeholder="0, 300" />
          </Field>
          <Field label="bounds" title="min, max" field="drag/bounds" diags={[]}>
            <DraftInput value={fmtList(drag?.bounds)} parse={numList} format={fmtList} onCommit={(v) => setDrag('bounds', v)} placeholder="-400, 400" />
          </Field>
        </div>
      </Section>

      <Section title="Accessibility" open={!!a11y}>
        <Field label="label" field="a11y/label" diags={at('a11y/label')}>
          <Dsl value={a11y?.label} onChange={(v) => setA11y('label', v)} diags={at('a11y/label')} />
        </Field>
        <div className="row2">
          <Field label="role" field="a11y/role" diags={at('a11y/role')}>
            <select value={a11y?.role ?? ''} onChange={(e) => setA11y('role', e.target.value || undefined)}>
              <option value="">—</option>
              {['button', 'header', 'image', 'none'].map((r) => (
                <option key={r}>{r}</option>
              ))}
            </select>
          </Field>
          <Field label="hidden" field="a11y/hidden" diags={[]}>
            <input type="checkbox" checked={!!a11y?.hidden} onChange={(e) => setA11y('hidden', e.target.checked || undefined)} />
          </Field>
        </div>
      </Section>
    </>
  )
}

function AddSelect({ label, options, onPick }: { label: string; options: string[]; onPick: (v: string) => void }) {
  if (!options.length) return null
  return (
    <select className="add" value="" onChange={(e) => e.target.value && onPick(e.target.value)}>
      <option value="">{label}</option>
      {options.map((o) => (
        <option key={o}>{o}</option>
      ))}
    </select>
  )
}
