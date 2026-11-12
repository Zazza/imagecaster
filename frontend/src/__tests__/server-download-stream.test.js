import { describe, it, expect, vi, beforeEach } from 'vitest'
import { EventsEmit, clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return {
    ...base,
    DownloadServerModelStream: vi.fn(),
    PullServerLLMModelStream: vi.fn(),
  }
})

import { DownloadServerModelStream, PullServerLLMModelStream } from '../wailsjs/go/main/App.js'
import { api } from '../api.js'

describe('server download goes through the app, not the webview', () => {
  beforeEach(() => {
    clearEventMocks()
    vi.clearAllMocks()
    globalThis.EventSource = vi.fn(() => { throw new Error('webview must not open EventSource') })
  })

  it('passes model params to the backend and forwards progress', async () => {
    DownloadServerModelStream.mockImplementation(async (id) => {
      EventsEmit(`server:download:progress:${id}`, '{"downloaded":1048576,"total":2097152}')
      EventsEmit(`server:download:progress:${id}`, 'already downloaded')
    })
    const seen = []
    await api.downloadServerModelStream('http://srv:8080', 'sd', 'https://h/x.safetensors', 'x.safetensors', (text, pct) => seen.push([text, pct]))
    expect(DownloadServerModelStream).toHaveBeenCalledWith(expect.any(String), 'sd', 'https://h/x.safetensors', 'x.safetensors')
    expect(seen[0]).toEqual(['1.0 MB / 2.0 MB', 50])
    expect(seen[1][0]).toBe('already downloaded')
    expect(globalThis.EventSource).not.toHaveBeenCalled()
  })

  it('rejects with the backend error and stops listening', async () => {
    let jobID
    DownloadServerModelStream.mockImplementation(async (id) => { jobID = id; throw 'download: 404' })
    const onProgress = vi.fn()
    await expect(api.downloadServerModelStream('http://srv:8080', 'vae', 'u', 'v.safetensors', onProgress)).rejects.toBe('download: 404')
    EventsEmit(`server:download:progress:${jobID}`, 'late line')
    expect(onProgress).not.toHaveBeenCalled()
  })

  it('pulls LLM models through the backend', async () => {
    PullServerLLMModelStream.mockImplementation(async (id) => {
      EventsEmit(`server:download:progress:${id}`, 'pulling manifest')
    })
    const seen = []
    await api.pullServerLLMModelStream('http://srv:8080', 'hf.co/a/b:Q4_K_M', (text) => seen.push(text))
    expect(PullServerLLMModelStream).toHaveBeenCalledWith(expect.any(String), 'hf.co/a/b:Q4_K_M')
    expect(seen).toEqual(['pulling manifest'])
  })

  it('keeps progress of parallel downloads apart', async () => {
    const release = {}
    DownloadServerModelStream.mockImplementation((id, type) => new Promise((resolve) => { release[type] = () => { EventsEmit(`server:download:progress:${id}`, type + ' line'); resolve() } }))
    PullServerLLMModelStream.mockImplementation((id) => new Promise((resolve) => { release.llm = () => { EventsEmit(`server:download:progress:${id}`, 'llm line'); resolve() } }))
    const sd = [], llm = []
    const a = api.downloadServerModelStream('http://srv:8080', 'sd', 'u', 'a.safetensors', (t) => sd.push(t))
    const b = api.pullServerLLMModelStream('http://srv:8080', 'm', (t) => llm.push(t))
    await Promise.resolve()
    release.llm(); release.sd()
    await Promise.all([a, b])
    expect(sd).toEqual(['sd line'])
    expect(llm).toEqual(['llm line'])
  })
})
