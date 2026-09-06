import { createContext, useContext } from 'react'
import type { ChangeEvent } from '../api/generated/types.gen'

export type ResourceKey = ChangeEvent['resources'][number]
export type RevisionMap = Record<ResourceKey, number>

type ChangeStreamValue = {
  revisions: RevisionMap
  invalidate: (...keys: ResourceKey[]) => void
}

export const ChangeStreamContext = createContext<ChangeStreamValue | null>(null)

function useChangeStreamContext() {
  const value = useContext(ChangeStreamContext)
  if (!value) throw new Error('ChangeStreamProvider is required')
  return value
}

export function useResourceRevision(resource: ResourceKey) {
  return useChangeStreamContext().revisions[resource]
}

export function useInvalidateResources() {
  return useChangeStreamContext().invalidate
}
