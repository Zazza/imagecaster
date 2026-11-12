import { describe, it, expect } from 'vitest'
import { formatBytes, parseProgress } from '../downloadProgress.js'

describe('formatBytes', () => {
  it.each([
    [1073741824, '1.0 GB'],
    [1048576 * 5, '5.0 MB'],
    [2048, '2.0 KB'],
    [500, '500 B'],
  ])('%s -> %s', (n, want) => {
    expect(formatBytes(n)).toBe(want)
  })
})

describe('parseProgress', () => {
  it('JSON with total gives percent and sizes', () => {
    expect(parseProgress('{"downloaded":1073741824,"total":4294967296,"percent":25}'))
      .toEqual({ percent: 25, text: '1.0 GB / 4.0 GB' })
  })

  it('JSON with total 0 gives unknown percent', () => {
    expect(parseProgress('{"downloaded":1048576,"total":0,"percent":0}'))
      .toEqual({ percent: null, text: '1.0 MB / ???' })
  })

  it('plain text with percent keeps text unchanged', () => {
    const s = 'pulling manifest: 42% 1.2 GB/3.0 GB'
    expect(parseProgress(s)).toEqual({ percent: 42, text: s })
  })

  it('decimal percent in text', () => {
    expect(parseProgress('12.5%').percent).toBe(12.5)
  })

  it('text without percent', () => {
    const s = 'verifying sha256 digest'
    expect(parseProgress(s)).toEqual({ percent: null, text: s })
  })

  it('several percents take the last', () => {
    expect(parseProgress('a 10% b 55%').percent).toBe(55)
  })

  it('JSON percent above 100 is clamped', () => {
    expect(parseProgress('{"downloaded":10,"total":10,"percent":130}').percent).toBe(100)
  })

  it('negative text percent is never negative', () => {
    const p = parseProgress('-5%').percent
    expect(p === null || (p >= 0 && p <= 100)).toBe(true)
  })

  it.each([[''], [undefined]])('empty input %s', (v) => {
    expect(parseProgress(v)).toEqual({ percent: null, text: '' })
  })
})
