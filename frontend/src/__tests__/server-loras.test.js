import { describe, it, expect, vi, beforeEach } from 'vitest'

const getLoRAs = vi.fn()
vi.mock('../api.js', () => ({ api: { getLoRAs: (...a) => getLoRAs(...a) } }))

const { fetchServerLoras, useServerLoras } = await import('../composables/useServerLoras.js')

beforeEach(() => {
  getLoRAs.mockReset()
})

describe('useServerLoras', () => {
  it('joins simultaneous requests into one server call', async () => {
    let resolve
    getLoRAs.mockReturnValue(new Promise(r => { resolve = r }))
    const a = fetchServerLoras()
    const b = fetchServerLoras()
    resolve([{ name: 'x' }])
    expect(await a).toEqual([{ name: 'x' }])
    expect(await b).toEqual([{ name: 'x' }])
    expect(getLoRAs).toHaveBeenCalledTimes(1)
  })

  it('asks the server again for a later request, so a new LoRA shows up', async () => {
    getLoRAs.mockResolvedValueOnce([{ name: 'x' }]).mockResolvedValueOnce([{ name: 'x' }, { name: 'y' }])
    await fetchServerLoras()
    expect(await fetchServerLoras()).toEqual([{ name: 'x' }, { name: 'y' }])
    expect(getLoRAs).toHaveBeenCalledTimes(2)
  })

  it('loads once per component and keeps the list', async () => {
    getLoRAs.mockResolvedValue([{ name: 'x' }])
    const s = useServerLoras()
    await s.load()
    await s.load()
    expect(s.loras.value).toEqual([{ name: 'x' }])
    expect(s.loaded.value).toBe(true)
    expect(getLoRAs).toHaveBeenCalledTimes(1)
  })

  it('marks an error and retries on the next load', async () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    getLoRAs.mockRejectedValueOnce(new Error('down')).mockResolvedValueOnce(null)
    const s = useServerLoras()
    await s.load()
    expect(s.error.value).toBe(true)
    expect(s.loaded.value).toBe(false)
    expect(err).toHaveBeenCalled()
    await s.load()
    expect(s.error.value).toBe(false)
    expect(s.loras.value).toEqual([])
    err.mockRestore()
  })
})
