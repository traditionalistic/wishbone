import { createContext, useContext } from 'react'
import type { Completion } from '@codemirror/autocomplete'
import type { Catalog } from '../model/types'

export interface EditorCtx {
  catalog: Catalog
  /** DSL completions for the current source, computed on demand */
  dsl: () => { all: Completion[]; methods: Completion[] }
}

export const Ctx = createContext<EditorCtx | null>(null)

export function useCtx(): EditorCtx {
  const c = useContext(Ctx)
  if (!c) throw new Error('EditorCtx missing')
  return c
}
