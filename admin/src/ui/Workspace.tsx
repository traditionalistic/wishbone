import { useEffect, useMemo, useRef, useState } from 'react'
import { ApiError, api, type Revision, type ScreenDetail, type ScreenSummary } from '../api'
import { parseDiagPath, pathsWithDiags } from '../model/path'
import { getNode, updateNode, walk, type Path } from '../model/tree'
import type { Catalog, Diag, Node, Source } from '../model/types'
import { CodeEditor } from './CodeEditor'
import { Console } from './Console'
import { Ctx, type EditorCtx } from './ctx'
import { dslOptions } from './dsl'
import { Inspector } from './Inspector'
import { JsonPanel } from './JsonPanel'
import { TimelinePanel, type TlSel } from './TimelinePanel'
import { TopBar } from './TopBar'
import { TreePanel } from './TreePanel'
import { usePreview } from './usePreview'

type Tab = 'script' | 'timeline' | 'json'

function normalize(s: ScreenDetail): Source {
  const d: Partial<Source> = s.draft ?? {}
  return {
    ...d,
    id: s.id,
    title: s.title,
    script: d.script ?? '',
    timelines: d.timelines ?? [],
    root: d.root ?? { type: 'stack', id: 'root', children: [] },
  }
}

const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

export function Workspace({ onLogout }: { onLogout: () => void }) {
  const [catalog, setCatalog] = useState<Catalog | null>(null)
  const [loadError, setLoadError] = useState('')
  const [screens, setScreens] = useState<ScreenSummary[]>([])
  const [screenId, setScreenId] = useState<string | null>(null)
  const [source, setSource] = useState<Source | null>(null)
  const [saved, setSaved] = useState('')
  const [publishedRev, setPublishedRev] = useState<number | null>(null)
  const [revisions, setRevisions] = useState<Revision[]>([])
  const [selected, setSelected] = useState<Path>([])
  const [tab, setTab] = useState<Tab>('script')
  const [focus, setFocus] = useState<{ line: number; col: number; n: number }>()
  const [tlSel, setTlSel] = useState<TlSel>({ tl: 0 })
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const preview = usePreview(screenId, source)

  const dirty = useMemo(() => source !== null && JSON.stringify(source) !== saved, [source, saved])
  const srcRef = useRef(source)
  srcRef.current = source
  const ctx = useMemo<EditorCtx | null>(() => catalog && { catalog, dsl: () => dslOptions(catalog, srcRef.current) }, [catalog])

  const flash = (msg: string) => {
    setNotice(msg)
    setTimeout(() => setNotice((n) => (n === msg ? '' : n)), 4000)
  }
  const refreshScreens = () => api.screens().then(setScreens, () => undefined)

  const load = async (id: string) => {
    setSource(null)
    setScreenId(id)
    history.replaceState(null, '', `#${encodeURIComponent(id)}`)
    try {
      const s = await api.screen(id)
      const src = normalize(s)
      setSource(src)
      setSaved(JSON.stringify(src))
      setPublishedRev(s.publishedRev)
      setSelected([])
      setTlSel({ tl: 0 })
      api.revisions(id).then(setRevisions, () => setRevisions([]))
    } catch (e) {
      flash(`Cannot open ${id}: ${message(e)}`)
      setScreenId(null)
    }
  }
  const open = (id: string) => {
    if (dirty && !confirm('Discard unsaved changes?')) return
    void load(id)
  }

  useEffect(() => {
    api.catalog().then(setCatalog, (e) => setLoadError(message(e)))
    void refreshScreens()
    const id = decodeURIComponent(location.hash.slice(1))
    if (id) void load(id)
  }, [])

  const save = async (): Promise<boolean> => {
    if (!source || !screenId) return false
    setBusy(true)
    try {
      await api.saveDraft(screenId, source)
      setSaved(JSON.stringify(source))
      void refreshScreens()
      flash('Draft saved')
      return true
    } catch (e) {
      flash(`Save failed: ${message(e)}`)
      return false
    } finally {
      setBusy(false)
    }
  }
  const saveRef = useRef(save)
  saveRef.current = save

  const publish = async () => {
    if (!screenId || (dirty && !(await save()))) return
    setBusy(true)
    try {
      const r = await api.publish(screenId)
      setPublishedRev(r.revision)
      flash(`Published r${r.revision}`)
      api.revisions(screenId).then(setRevisions, () => undefined)
      void refreshScreens()
    } catch (e) {
      if (e instanceof ApiError && e.status === 422) {
        preview.setDiags(e.diags)
        flash(`Not published: ${e.diags.length} problem(s)`)
      } else flash(`Publish failed: ${message(e)}`)
    } finally {
      setBusy(false)
    }
  }

  const rollback = async (rev: number) => {
    if (!screenId || (dirty && !confirm('Discard unsaved changes?'))) return
    setBusy(true)
    try {
      const r = await api.rollback(screenId, rev)
      flash(`Rolled back to r${rev} → live r${r.revision}`)
      await load(screenId)
      void refreshScreens()
    } catch (e) {
      flash(`Rollback failed: ${message(e)}`)
    } finally {
      setBusy(false)
    }
  }

  const create = async () => {
    const id = prompt('New screen id (e.g. showcase.promo)')?.trim()
    if (!id) return
    const title = prompt('Title', id)?.trim() || id
    try {
      await api.createScreen(id, title)
      await refreshScreens()
      open(id)
    } catch (e) {
      flash(`Cannot create: ${message(e)}`)
    }
  }

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 's') {
        e.preventDefault()
        void saveRef.current()
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [])

  useEffect(() => {
    if (!dirty) return
    const h = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', h)
    return () => window.removeEventListener('beforeunload', h)
  }, [dirty])

  const update = (fn: (s: Source) => Source) => setSource((s) => s && fn(s))
  const setRoot = (root: Node, select?: Path) => {
    update((s) => ({ ...s, root }))
    if (select) setSelected(select)
  }

  const gotoDiag = (d: Diag) => {
    const t = parseDiagPath(d.path)
    if (t.kind === 'script') {
      setTab('script')
      setFocus({ line: d.line, col: d.col, n: Date.now() })
    } else if (t.kind === 'timeline') {
      setTab('timeline')
      setTlSel({ tl: t.timeline, track: t.track })
    } else if (t.kind === 'node' && source && getNode(source.root, t.node)) {
      setSelected(t.node)
      setTimeout(() => {
        const parts = t.field.split('/')
        for (let i = parts.length; i > 0; i--) {
          const el = document.querySelector(`.inspector [data-field="${CSS.escape(parts.slice(0, i).join('/'))}"]`)
          if (!el) continue
          const details = el.closest('details')
          if (details) details.open = true
          el.scrollIntoView({ block: 'center' })
          break
        }
      }, 50)
    }
  }

  if (loadError) return <div className="fatal">Cannot load catalog: {loadError}</div>
  if (!catalog || !ctx) return <div className="fatal dim">Loading catalog…</div>

  const diags = preview.diags
  const selPath = source && getNode(source.root, selected) ? selected : []
  const nodeIds: string[] = []
  if (source) walk(source.root, (n) => n.id && nodeIds.push(n.id))

  return (
    <Ctx.Provider value={ctx}>
      <div className="app">
        <TopBar
          screens={screens}
          screenId={screenId}
          title={source?.title ?? ''}
          onTitle={(title) => update((s) => ({ ...s, title }))}
          onOpen={open}
          onCreate={create}
          status={preview.status}
          problems={diags.length}
          code={preview.code}
          devices={preview.devices}
          dirty={dirty}
          busy={busy}
          notice={notice}
          onSave={() => void save()}
          onPublish={() => void publish()}
          publishedRev={publishedRev}
          revisions={revisions}
          onRollback={(rev) => void rollback(rev)}
          onLogout={onLogout}
        />
        {!source ? (
          <div className="welcome">
            {screenId ? (
              <p className="dim">Loading {screenId}…</p>
            ) : (
              <>
                <h2>Screens</h2>
                {screens.map((s) => (
                  <button key={s.id} className="screen-link" onClick={() => open(s.id)}>
                    <b>{s.id}</b> <span className="dim">{s.title}</span>
                  </button>
                ))}
                <button className="primary" onClick={create}>
                  + New screen
                </button>
              </>
            )}
          </div>
        ) : (
          <>
            <aside className="left">
              <TreePanel root={source.root} selected={selPath} diagPaths={pathsWithDiags(diags)} onSelect={setSelected} onChange={setRoot} />
            </aside>
            <main className="center">
              <div className="tabs">
                {(['script', 'timeline', 'json'] as Tab[]).map((t) => {
                  const n = diags.filter((d) => (t === 'script' ? d.path === 'script' : t === 'timeline' ? d.path.startsWith('timelines/') : false)).length
                  return (
                    <button key={t} className={tab === t ? 'on' : ''} onClick={() => setTab(t)}>
                      {t === 'json' ? 'JSON' : t[0].toUpperCase() + t.slice(1)}
                      {n > 0 && <span className="count error">{n}</span>}
                    </button>
                  )
                })}
              </div>
              <div className="tab-body" hidden={tab !== 'script'}>
                <CodeEditor
                  key={screenId}
                  value={source.script}
                  onChange={(script) => update((s) => ({ ...s, script }))}
                  diags={diags.filter((d) => d.path === 'script')}
                  focus={focus}
                />
              </div>
              {tab === 'timeline' && (
                <div className="tab-body">
                  <TimelinePanel
                    timelines={source.timelines}
                    onChange={(timelines) => update((s) => ({ ...s, timelines }))}
                    nodeIds={nodeIds}
                    diags={diags}
                    sel={tlSel}
                    setSel={setTlSel}
                    onPlay={preview.play}
                  />
                </div>
              )}
              {tab === 'json' && (
                <div className="tab-body">
                  <JsonPanel source={source} onChange={setSource} />
                </div>
              )}
            </main>
            <aside className="right">
              <Inspector
                key={`${screenId}:${selPath.join('.')}`}
                node={getNode(source.root, selPath)}
                path={selPath}
                diags={diags}
                onChange={(n) => update((s) => ({ ...s, root: updateNode(s.root, selPath, () => n) }))}
              />
            </aside>
          </>
        )}
        <Console diags={diags} logs={preview.logs} devices={preview.devices} onDiag={gotoDiag} onClear={preview.clearLogs} />
      </div>
    </Ctx.Provider>
  )
}
