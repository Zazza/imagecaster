import { vi } from 'vitest'

const listeners = {}

export function EventsOn(event, handler) {
  if (!listeners[event]) listeners[event] = []
  listeners[event].push(handler)
  return () => {
    if (listeners[event]) listeners[event] = listeners[event].filter(h => h !== handler)
  }
}

export function EventsOff(event) {
  delete listeners[event]
}

export function EventsEmit(event, ...args) {
  if (listeners[event]) {
    listeners[event].forEach(h => h(...args))
  }
}

export function clearEventMocks() {
  for (const key in listeners) delete listeners[key]
}

export const BrowserOpenURL = vi.fn()
export const WindowSetSystemDefaultTheme = vi.fn()
export const WindowSetDarkTheme = vi.fn()
export const WindowSetLightTheme = vi.fn()
export const Environment = vi.fn(() => Promise.reject(new Error('no wails')))
export const WindowIsMaximised = vi.fn(() => Promise.resolve(false))
export const WindowIsFullscreen = vi.fn(() => Promise.resolve(false))
