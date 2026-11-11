import { describe, it, expect } from 'vitest'
import { frameEdge } from '../windowEdge.js'

describe('frameEdge', () => {
  it.each([
    ['n-resize', undefined],
    ['nw-resize', 'w-resize'],
    ['ne-resize', 'e-resize'],
    ['s-resize', 's-resize'],
    ['sw-resize', 'sw-resize'],
    ['se-resize', 'se-resize'],
    ['w-resize', 'w-resize'],
    ['e-resize', 'e-resize'],
  ])('%s → %s', (edge, want) => {
    expect(frameEdge(edge, false)).toBe(want)
  })

  it('maximised window does not resize from any edge', () => {
    for (const e of ['s-resize', 'w-resize', 'se-resize', 'nw-resize']) expect(frameEdge(e, true)).toBeUndefined()
  })

  it('no edge stays no edge', () => {
    expect(frameEdge(undefined, false)).toBeUndefined()
    expect(frameEdge('', false)).toBeUndefined()
  })
})
