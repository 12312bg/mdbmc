import type { Api } from './types'
import { wailsApi } from './wails'
import { mockApi } from './mock'

export * from './types'

// Inside the Wails window (served from wails.localhost) the Go bindings are
// injected as window.go; in a plain browser (npm run dev) the UI runs against
// an in-memory mock.
const inWails =
  !!(window as any).go || location.hostname === 'wails.localhost' || location.protocol === 'wails:'

export const api: Api = inWails ? wailsApi : mockApi

/** Normalizes errors thrown by bindings (strings) and mocks (Error). */
export function errMsg(e: unknown): string {
  if (typeof e === 'string') return e
  if (e instanceof Error) return e.message
  return String(e)
}
