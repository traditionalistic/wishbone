import { useEffect, useRef, useState } from 'react'
import { sourceError, type Source } from '../model/types'
import { CodeEditor } from './CodeEditor'

const fmt = (s: Source) => JSON.stringify(s, null, 2)

/** Raw source editor: applies the text whenever it parses and passes the shape check. */
export function JsonPanel({ source, onChange }: { source: Source; onChange: (s: Source) => void }) {
  const [text, setText] = useState(() => fmt(source))
  const [error, setError] = useState<string | null>(null)
  const mine = useRef(source)

  // Changes made elsewhere (tree, inspector) replace the text; our own don't, so typing isn't reformatted.
  useEffect(() => {
    if (source !== mine.current) {
      mine.current = source
      setText(fmt(source))
      setError(null)
    }
  }, [source])

  const edit = (t: string) => {
    setText(t)
    let v: unknown
    try {
      v = JSON.parse(t)
    } catch (e) {
      return setError((e as Error).message)
    }
    const err = sourceError(v)
    setError(err)
    if (err) return
    mine.current = v as Source
    onChange(v as Source)
  }

  return (
    <div className="json-panel">
      <div className={'json-status' + (error ? ' error' : '')}>{error ?? 'Valid — changes apply as you type'}</div>
      <CodeEditor value={text} onChange={edit} lang="plain" />
    </div>
  )
}
