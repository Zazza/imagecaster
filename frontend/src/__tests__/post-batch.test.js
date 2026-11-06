import { describe, it, expect, vi } from 'vitest'
import { createBatch, batchExportSize, batchFileName } from '../postBatch.js'

const makeDeps = () => {
  const files = new Map([
    ['/src/a.jpg', 'A0'],
    ['/src/b.jpg', 'B0'],
    ['/src/c.jpg', 'C0'],
  ])
  let seq = 0
  const readImage = vi.fn(async (path) => {
    if (!files.has(path)) throw new Error(`no file ${path}`)
    return files.get(path)
  })
  const saveImage = vi.fn(async (b64) => {
    seq += 1
    const path = `/batch/${seq}.png`
    files.set(path, b64)
    return path
  })
  return { files, readImage, saveImage }
}

const makeItems = () => [
  { name: 'a.jpg', path: '/src/a.jpg' },
  { name: 'b.jpg', path: '/src/b.jpg' },
  { name: 'c.jpg', path: '/src/c.jpg' },
]

const setup = () => {
  const deps = makeDeps()
  const batch = createBatch(makeItems(), deps)
  return { deps, batch }
}

const current = (batch, deps) => batch.items.map((_, i) => deps.files.get(batch.currentPath(i)))
const suffix = (tag) => async (b64) => `${b64}+${tag}`

describe('createBatch', () => {
  it('starts with sources as the only version', () => {
    const { batch } = setup()
    expect(batch.items.map((it) => it.versions)).toEqual([['/src/a.jpg'], ['/src/b.jpg'], ['/src/c.jpg']])
    expect(batch.items.map((it) => it.failed)).toEqual([false, false, false])
    expect(batch.items.map((it) => it.name)).toEqual(['a.jpg', 'b.jpg', 'c.jpg'])
    expect(batch.steps).toBe(0)
    expect(batch.canUndo).toBe(false)
    expect([0, 1, 2].map((i) => batch.currentPath(i))).toEqual(['/src/a.jpg', '/src/b.jpg', '/src/c.jpg'])
  })

  it('P1 successful step adds a version to every photo', async () => {
    const { deps, batch } = setup()
    const op = vi.fn(suffix('s1'))
    const onProgress = vi.fn()

    const res = await batch.applyStep(op, { isCancelled: () => false, onProgress })

    expect(res).toEqual({ changed: 3, failed: 0, cancelled: false })
    expect(batch.steps).toBe(1)
    expect(batch.canUndo).toBe(true)
    expect(batch.items.map((it) => it.versions.length)).toEqual([2, 2, 2])
    batch.items.forEach((it, i) => {
      expect(batch.currentPath(i)).toBe(it.versions[1])
      expect(batch.currentPath(i)).not.toBe(it.versions[0])
    })
    expect(new Set([0, 1, 2].map((i) => batch.currentPath(i))).size).toBe(3)
    expect(op.mock.calls).toEqual([['A0', 0], ['B0', 1], ['C0', 2]])
    expect(current(batch, deps)).toEqual(['A0+s1', 'B0+s1', 'C0+s1'])
    expect(onProgress.mock.calls).toEqual([[1, 3], [2, 3], [3, 3]])
  })

  it.each([
    ['empty result', async (b64, i) => (i === 1 ? '' : `${b64}+s1`)],
    ['null result', async (b64, i) => (i === 1 ? null : `${b64}+s1`)],
    ['op throws', async (b64, i) => {
      if (i === 1) throw new Error('boom')
      return `${b64}+s1`
    }],
  ])('P2 %s on photo #2 marks it failed and keeps its version', async (_label, op) => {
    const { deps, batch } = setup()

    const res = await batch.applyStep(op, { isCancelled: () => false, onProgress: () => {} })

    expect(res).toEqual({ changed: 2, failed: 1, cancelled: false })
    expect(batch.steps).toBe(1)
    expect(batch.items.map((it) => it.failed)).toEqual([false, true, false])
    expect(batch.currentPath(1)).toBe('/src/b.jpg')
    expect(current(batch, deps)).toEqual(['A0+s1', 'B0', 'C0+s1'])
  })

  it('P2 saveImage failure marks the photo failed', async () => {
    const { deps, batch } = setup()
    const realSave = deps.saveImage.getMockImplementation()
    deps.saveImage.mockImplementation(async (b64) => {
      if (b64.startsWith('B0')) throw new Error('disk full')
      return realSave(b64)
    })

    const res = await batch.applyStep(suffix('s1'), { isCancelled: () => false, onProgress: () => {} })

    expect(res).toEqual({ changed: 2, failed: 1, cancelled: false })
    expect(batch.items[1].failed).toBe(true)
    expect(batch.currentPath(1)).toBe('/src/b.jpg')
  })

  it('P2 readImage failure marks the photo failed and op is not called for it', async () => {
    const { deps, batch } = setup()
    deps.files.delete('/src/b.jpg')
    const op = vi.fn(suffix('s1'))

    const res = await batch.applyStep(op, { isCancelled: () => false, onProgress: () => {} })

    expect(res).toEqual({ changed: 2, failed: 1, cancelled: false })
    expect(batch.items.map((it) => it.failed)).toEqual([false, true, false])
    expect(op.mock.calls.map((c) => c[1])).toEqual([0, 2])
  })

  it('P2 failed flag clears when the next step succeeds on that photo', async () => {
    const { batch } = setup()
    await batch.applyStep(async (b64, i) => (i === 1 ? '' : `${b64}+s1`), { isCancelled: () => false, onProgress: () => {} })
    expect(batch.items[1].failed).toBe(true)

    await batch.applyStep(suffix('s2'), { isCancelled: () => false, onProgress: () => {} })

    expect(batch.items.map((it) => it.failed)).toEqual([false, false, false])
  })

  it('P3 second step builds on the result of the first', async () => {
    const { deps, batch } = setup()
    await batch.applyStep(suffix('s1'), { isCancelled: () => false, onProgress: () => {} })
    const op2 = vi.fn(suffix('s2'))

    await batch.applyStep(op2, { isCancelled: () => false, onProgress: () => {} })

    expect(op2.mock.calls).toEqual([['A0+s1', 0], ['B0+s1', 1], ['C0+s1', 2]])
    expect(current(batch, deps)).toEqual(['A0+s1+s2', 'B0+s1+s2', 'C0+s1+s2'])
    expect(batch.steps).toBe(2)
    expect(batch.items.map((it) => it.versions.length)).toEqual([3, 3, 3])
  })

  it('P4 cancel after the first photo keeps its new version and leaves the rest', async () => {
    const { deps, batch } = setup()
    let calls = 0
    const op = vi.fn(async (b64) => {
      calls += 1
      return `${b64}+s1`
    })

    const res = await batch.applyStep(op, { isCancelled: () => calls >= 1, onProgress: () => {} })

    expect(res.changed).toBe(1)
    expect(res.cancelled).toBe(true)
    expect(op).toHaveBeenCalledTimes(1)
    expect(batch.steps).toBe(1)
    expect(current(batch, deps)).toEqual(['A0+s1', 'B0', 'C0'])
    expect(batch.items.map((it) => it.failed)).toEqual([false, false, false])

    expect(batch.undo()).toBe(true)
    expect(batch.steps).toBe(0)
    expect([0, 1, 2].map((i) => batch.currentPath(i))).toEqual(['/src/a.jpg', '/src/b.jpg', '/src/c.jpg'])
    expect(batch.items.map((it) => it.versions.length)).toEqual([1, 1, 1])
  })

  it('P4 cancelled before start changes nothing and records no step', async () => {
    const { batch } = setup()
    const op = vi.fn(suffix('s1'))

    const res = await batch.applyStep(op, { isCancelled: () => true, onProgress: () => {} })

    expect(res.changed).toBe(0)
    expect(res.cancelled).toBe(true)
    expect(op).not.toHaveBeenCalled()
    expect(batch.steps).toBe(0)
  })

  it('P5 all results empty records no step', async () => {
    const { deps, batch } = setup()

    const res = await batch.applyStep(async () => '', { isCancelled: () => false, onProgress: () => {} })

    expect(res).toEqual({ changed: 0, failed: 3, cancelled: false })
    expect(batch.steps).toBe(0)
    expect(batch.canUndo).toBe(false)
    expect(batch.items.map((it) => it.versions.length)).toEqual([1, 1, 1])
    expect(deps.saveImage).not.toHaveBeenCalled()
    expect(batch.undo()).toBe(false)
  })

  it('P6 undo walks back step by step to the sources', async () => {
    const { deps, batch } = setup()
    await batch.applyStep(suffix('s1'), { isCancelled: () => false, onProgress: () => {} })
    const afterFirst = [0, 1, 2].map((i) => batch.currentPath(i))
    await batch.applyStep(suffix('s2'), { isCancelled: () => false, onProgress: () => {} })

    expect(batch.undo()).toBe(true)
    expect(batch.steps).toBe(1)
    expect(batch.canUndo).toBe(true)
    expect([0, 1, 2].map((i) => batch.currentPath(i))).toEqual(afterFirst)
    expect(current(batch, deps)).toEqual(['A0+s1', 'B0+s1', 'C0+s1'])

    expect(batch.undo()).toBe(true)
    expect(batch.steps).toBe(0)
    expect(batch.canUndo).toBe(false)
    expect([0, 1, 2].map((i) => batch.currentPath(i))).toEqual(['/src/a.jpg', '/src/b.jpg', '/src/c.jpg'])

    expect(batch.undo()).toBe(false)
    expect(batch.steps).toBe(0)
    expect(batch.items.map((it) => it.versions.length)).toEqual([1, 1, 1])
  })

  it('P6 undo keeps photos consistent when a step skipped some of them', async () => {
    const { deps, batch } = setup()
    await batch.applyStep(suffix('s1'), { isCancelled: () => false, onProgress: () => {} })
    await batch.applyStep(async (b64, i) => (i === 1 ? '' : `${b64}+s2`), { isCancelled: () => false, onProgress: () => {} })
    expect(current(batch, deps)).toEqual(['A0+s1+s2', 'B0+s1', 'C0+s1+s2'])

    expect(batch.undo()).toBe(true)

    expect(current(batch, deps)).toEqual(['A0+s1', 'B0+s1', 'C0+s1'])
    expect(batch.items.map((it) => it.failed)).toEqual([false, false, false])

    expect(batch.undo()).toBe(true)
    expect(current(batch, deps)).toEqual(['A0', 'B0', 'C0'])
  })

  it('P6 step after undo continues from the restored version', async () => {
    const { deps, batch } = setup()
    await batch.applyStep(suffix('s1'), { isCancelled: () => false, onProgress: () => {} })
    batch.undo()

    await batch.applyStep(suffix('s2'), { isCancelled: () => false, onProgress: () => {} })

    expect(current(batch, deps)).toEqual(['A0+s2', 'B0+s2', 'C0+s2'])
    expect(batch.steps).toBe(1)
  })

  it('P7 exportAll passes current version, item and index', async () => {
    const { batch } = setup()
    await batch.applyStep(suffix('s1'), { isCancelled: () => false, onProgress: () => {} })
    const fn = vi.fn(async () => true)

    const res = await batch.exportAll(fn, { isCancelled: () => false, onProgress: () => {} })

    expect(res).toEqual({ saved: 3, skipped: 0, cancelled: false })
    expect(fn.mock.calls.map(([b64, item, i]) => [b64, item.name, i])).toEqual([
      ['A0+s1', 'a.jpg', 0],
      ['B0+s1', 'b.jpg', 1],
      ['C0+s1', 'c.jpg', 2],
    ])
    expect(fn.mock.calls[1][1]).toBe(batch.items[1])
  })

  it.each([
    ['fn throws', async (_b64, _item, i) => {
      if (i === 1) throw new Error('write failed')
      return true
    }],
    ['fn returns falsy', async (_b64, _item, i) => i !== 1],
  ])('P7 %s on #2 skips it and continues', async (_label, impl) => {
    const { batch } = setup()
    const fn = vi.fn(impl)

    const res = await batch.exportAll(fn, { isCancelled: () => false, onProgress: () => {} })

    expect(res).toEqual({ saved: 2, skipped: 1, cancelled: false })
    expect(fn).toHaveBeenCalledTimes(3)
  })

  it('P7 read error on #2 skips it', async () => {
    const { deps, batch } = setup()
    deps.files.delete('/src/b.jpg')
    const fn = vi.fn(async () => true)

    const res = await batch.exportAll(fn, { isCancelled: () => false, onProgress: () => {} })

    expect(res).toEqual({ saved: 2, skipped: 1, cancelled: false })
    expect(fn.mock.calls.map((c) => c[2])).toEqual([0, 2])
  })

  it('P7 cancel stops export and leaves the rest uncalled', async () => {
    const { batch } = setup()
    let calls = 0
    const fn = vi.fn(async () => {
      calls += 1
      return true
    })

    const res = await batch.exportAll(fn, { isCancelled: () => calls >= 1, onProgress: () => {} })

    expect(res.cancelled).toBe(true)
    expect(res.saved).toBe(1)
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('P7 export does not change versions or steps', async () => {
    const { batch } = setup()
    await batch.applyStep(suffix('s1'), { isCancelled: () => false, onProgress: () => {} })

    await batch.exportAll(async () => true, { isCancelled: () => false, onProgress: () => {} })

    expect(batch.steps).toBe(1)
    expect(batch.items.map((it) => it.versions.length)).toEqual([2, 2, 2])
  })
})

describe('batchExportSize', () => {
  it.each([
    ['P8 unchanged size keeps each photo own size', { width: 2048, height: 1536, lockRatio: true, previewWidth: 2048, previewHeight: 1536 }, { width: 0, height: 0 }],
    ['P8 unchanged size without lock', { width: 2048, height: 1536, lockRatio: false, previewWidth: 2048, previewHeight: 1536 }, { width: 0, height: 0 }],
    ['P8 lockRatio uses the larger value as long side', { width: 1024, height: 768, lockRatio: true, previewWidth: 2048, previewHeight: 1536 }, { width: 1024, height: 0 }],
    ['P8 lockRatio with portrait values', { width: 600, height: 900, lockRatio: true, previewWidth: 1200, previewHeight: 1800 }, { width: 900, height: 0 }],
    ['P8 lockRatio with only height changed', { width: 2048, height: 1000, lockRatio: true, previewWidth: 2048, previewHeight: 1536 }, { width: 2048, height: 0 }],
    ['P8 no lockRatio gives exact size', { width: 800, height: 600, lockRatio: false, previewWidth: 2048, previewHeight: 1536 }, { width: 800, height: 600 }],
  ])('%s', (_label, input, expected) => {
    expect(batchExportSize(input)).toEqual(expected)
  })
})

describe('batchFileName', () => {
  it.each([
    ['P9 replaces extension', 'IMG_0412.jpg', 'png', 'IMG_0412.png'],
    ['P9 empty name', '', 'jpeg', 'image.jpeg'],
    ['P9 keeps inner dots', 'a.b.c.png', 'png', 'a.b.c.png'],
    ['P9 name without extension', 'photo', 'webp', 'photo.webp'],
  ])('%s', (_label, name, format, expected) => {
    expect(batchFileName(name, format)).toBe(expected)
  })
})

const deferred = () => {
  let resolve
  const promise = new Promise((r) => { resolve = r })
  return { promise, resolve }
}

const tick = () => new Promise((r) => setTimeout(r, 0))

describe('createBatch with waiter ops', () => {
  it('P10 all ops are called before the first result and results land on their own photos', async () => {
    const { deps, batch } = setup()
    const waits = [deferred(), deferred(), deferred()]
    const op = vi.fn((b64, i) => () => waits[i].promise)
    const onProgress = vi.fn()

    const run = batch.applyStep(op, { isCancelled: () => false, onProgress })
    for (let k = 0; k < 5 && op.mock.calls.length < 3; k++) await tick()

    expect(op.mock.calls).toEqual([['A0', 0], ['B0', 1], ['C0', 2]])
    expect(onProgress).not.toHaveBeenCalled()

    waits[2].resolve('C0+s1')
    await tick()
    waits[1].resolve('B0+s1')
    await tick()
    waits[0].resolve('A0+s1')
    const res = await run

    expect(res).toEqual({ changed: 3, failed: 0, cancelled: false })
    expect(batch.steps).toBe(1)
    expect(current(batch, deps)).toEqual(['A0+s1', 'B0+s1', 'C0+s1'])
    expect(batch.items.map((it) => it.versions.length)).toEqual([2, 2, 2])
    expect(onProgress).toHaveBeenCalledTimes(3)
    expect(onProgress.mock.calls.at(-1)).toEqual([3, 3])
  })

  it('P11 cancel after enqueue leaves unfinished photos untouched and not failed', async () => {
    const { deps, batch } = setup()
    const waits = [deferred(), deferred(), deferred()]
    const op = vi.fn((b64, i) => () => waits[i].promise)
    let cancelled = false

    const run = batch.applyStep(op, { isCancelled: () => cancelled, onProgress: () => {} })
    for (let k = 0; k < 5 && op.mock.calls.length < 3; k++) await tick()
    expect(op).toHaveBeenCalledTimes(3)

    waits[0].resolve('A0+s1')
    await tick()
    cancelled = true
    waits[1].resolve('')
    waits[2].resolve('')
    const res = await run

    expect(res.cancelled).toBe(true)
    expect(res.changed).toBe(1)
    expect(batch.steps).toBe(1)
    expect(batch.items.map((it) => it.failed)).toEqual([false, false, false])
    expect(current(batch, deps)).toEqual(['A0+s1', 'B0', 'C0'])
    expect([1, 2].map((i) => batch.currentPath(i))).toEqual(['/src/b.jpg', '/src/c.jpg'])
  })

  it('P11 empty waiter result without cancel still marks the photo failed', async () => {
    const { batch } = setup()
    const op = (b64, i) => () => Promise.resolve(i === 1 ? '' : `${b64}+s1`)

    const res = await batch.applyStep(op, { isCancelled: () => false, onProgress: () => {} })

    expect(res).toEqual({ changed: 2, failed: 1, cancelled: false })
    expect(batch.items.map((it) => it.failed)).toEqual([false, true, false])
  })
})

describe('createBatch removeItem', () => {
  it('P12 removing a photo keeps the others with their versions and steps', async () => {
    const { deps, batch } = setup()
    await batch.applyStep(suffix('s1'), { isCancelled: () => false, onProgress: () => {} })
    const keptA = [...batch.items[0].versions]
    const keptC = [...batch.items[2].versions]

    expect(batch.removeItem(1)).toBe(true)

    expect(batch.items.map((it) => it.name)).toEqual(['a.jpg', 'c.jpg'])
    expect(batch.items.map((it) => it.versions)).toEqual([keptA, keptC])
    expect(batch.steps).toBe(1)
    expect(batch.canUndo).toBe(true)
    expect(current(batch, deps)).toEqual(['A0+s1', 'C0+s1'])

    expect(batch.undo()).toBe(true)
    expect(batch.steps).toBe(0)
    expect([0, 1].map((i) => batch.currentPath(i))).toEqual(['/src/a.jpg', '/src/c.jpg'])
    expect(batch.items.map((it) => it.versions.length)).toEqual([1, 1])
  })

  it.each([[5], [3], [-1]])('P12 invalid index %s returns false and changes nothing', async (idx) => {
    const { batch } = setup()
    await batch.applyStep(suffix('s1'), { isCancelled: () => false, onProgress: () => {} })

    expect(batch.removeItem(idx)).toBe(false)

    expect(batch.items).toHaveLength(3)
    expect(batch.steps).toBe(1)
  })

  it('P12 removing before any step keeps steps at zero', () => {
    const { batch } = setup()

    expect(batch.removeItem(0)).toBe(true)

    expect(batch.items.map((it) => it.name)).toEqual(['b.jpg', 'c.jpg'])
    expect(batch.steps).toBe(0)
    expect(batch.canUndo).toBe(false)
  })

  it('P12 next step after removal touches only the remaining photos', async () => {
    const { deps, batch } = setup()
    batch.removeItem(1)
    const op = vi.fn(suffix('s1'))

    const res = await batch.applyStep(op, { isCancelled: () => false, onProgress: () => {} })

    expect(res).toEqual({ changed: 2, failed: 0, cancelled: false })
    expect(op.mock.calls).toEqual([['A0', 0], ['C0', 1]])
    expect(current(batch, deps)).toEqual(['A0+s1', 'C0+s1'])
  })
})
