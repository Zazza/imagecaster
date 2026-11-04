import { describe, it, expect, vi } from 'vitest'
import { runIdeasBatch } from '../ideasBatch.js'

function idea(id) {
  return { id, name: 'Idea ' + id, text: 'text ' + id, negative_prompt: 'neg ' + id }
}

function okPrompt(i) {
  return { prompt: 'prompt for ' + i.id, negative_prompt: 'negp for ' + i.id }
}

function makeDeps(overrides = {}) {
  let job = 0
  return {
    generatePrompt: vi.fn(async (i) => okPrompt(i)),
    enqueue: vi.fn(async () => 'job-' + (++job)),
    isStopped: vi.fn(() => false),
    onProgress: vi.fn(),
    ...overrides,
  }
}

describe('runIdeasBatch (У6)', () => {
  it('processes ideas strictly in order, one at a time', async () => {
    const log = []
    let inFlight = 0
    let maxInFlight = 0
    const deps = makeDeps({
      generatePrompt: vi.fn(async (i) => {
        inFlight++
        maxInFlight = Math.max(maxInFlight, inFlight)
        log.push('gen:' + i.id)
        await new Promise(r => setTimeout(r, 1))
        inFlight--
        return okPrompt(i)
      }),
      enqueue: vi.fn(async (p) => {
        log.push('enq:' + p.idea.id + ':' + p.index)
        return 'j-' + p.idea.id + '-' + p.index
      }),
    })
    const res = await runIdeasBatch({ ideas: [idea(3), idea(1), idea(2)], perIdea: 2, ...deps })
    expect(log).toEqual([
      'gen:3', 'enq:3:0', 'enq:3:1',
      'gen:1', 'enq:1:0', 'enq:1:1',
      'gen:2', 'enq:2:0', 'enq:2:1',
    ])
    expect(maxInFlight).toBe(1)
    expect(res.jobIds).toEqual(['j-3-0', 'j-3-1', 'j-1-0', 'j-1-1', 'j-2-0', 'j-2-1'])
    expect(res.errors).toEqual([])
    expect(res.stopped).toBe(false)
  })

  it('passes the idea to generatePrompt and {prompt, negative_prompt, idea, index} to enqueue', async () => {
    const a = idea(7)
    const deps = makeDeps()
    await runIdeasBatch({ ideas: [a], perIdea: 3, ...deps })
    expect(deps.generatePrompt).toHaveBeenCalledTimes(1)
    expect(deps.generatePrompt.mock.calls[0][0]).toEqual(a)
    expect(deps.enqueue).toHaveBeenCalledTimes(3)
    deps.enqueue.mock.calls.forEach(([arg], idx) => {
      expect(arg).toEqual(expect.objectContaining({
        prompt: 'prompt for 7',
        negative_prompt: 'negp for 7',
        index: idx,
      }))
      expect(arg.idea).toEqual(a)
    })
  })

  it('empty ideas list: nothing called, empty result', async () => {
    const deps = makeDeps()
    const res = await runIdeasBatch({ ideas: [], perIdea: 2, ...deps })
    expect(deps.generatePrompt).not.toHaveBeenCalled()
    expect(deps.enqueue).not.toHaveBeenCalled()
    expect(res).toEqual({ jobIds: [], errors: [], stopped: false })
  })

  describe('perIdea clamping to integer 1..20', () => {
    const cases = [
      { perIdea: 0, expected: 1 },
      { perIdea: -3, expected: 1 },
      { perIdea: 'abc', expected: 1 },
      { perIdea: undefined, expected: 1 },
      { perIdea: 2.7, expected: 2 },
      { perIdea: 20, expected: 20 },
      { perIdea: 25, expected: 20 },
      { perIdea: '4', expected: 4 },
    ]
    for (const c of cases) {
      it(`perIdea=${JSON.stringify(c.perIdea)} → ${c.expected} enqueues`, async () => {
        const deps = makeDeps()
        const res = await runIdeasBatch({ ideas: [idea(1)], perIdea: c.perIdea, ...deps })
        expect(deps.enqueue).toHaveBeenCalledTimes(c.expected)
        expect(deps.enqueue.mock.calls.map(([a]) => a.index)).toEqual([...Array(c.expected).keys()])
        expect(res.jobIds).toHaveLength(c.expected)
      })
    }
  })

  it('empty prompt: idea error, no enqueue for it, next idea still processed', async () => {
    const a = idea(1)
    const b = idea(2)
    const deps = makeDeps({
      generatePrompt: vi.fn(async (i) => (i.id === 1 ? { prompt: '', negative_prompt: 'x' } : okPrompt(i))),
    })
    const res = await runIdeasBatch({ ideas: [a, b], perIdea: 2, ...deps })
    expect(deps.enqueue.mock.calls.map(([x]) => x.idea.id)).toEqual([2, 2])
    expect(res.errors).toHaveLength(1)
    expect(res.errors[0].idea).toEqual(a)
    expect(typeof res.errors[0].error).toBe('string')
    expect(res.errors[0].error.length).toBeGreaterThan(0)
    expect(res.jobIds).toHaveLength(2)
    expect(res.stopped).toBe(false)
  })

  it('whitespace-only prompt is treated as empty', async () => {
    const a = idea(1)
    const deps = makeDeps({ generatePrompt: vi.fn(async () => ({ prompt: '   \n\t ', negative_prompt: '' })) })
    const res = await runIdeasBatch({ ideas: [a], perIdea: 1, ...deps })
    expect(deps.enqueue).not.toHaveBeenCalled()
    expect(res.errors).toHaveLength(1)
    expect(res.errors[0].idea).toEqual(a)
    expect(res.jobIds).toEqual([])
  })

  it('generatePrompt throws: idea error with message string, no enqueue, next idea processed', async () => {
    const a = idea(1)
    const b = idea(2)
    const deps = makeDeps({
      generatePrompt: vi.fn(async (i) => {
        if (i.id === 1) throw new Error('llm down')
        return okPrompt(i)
      }),
    })
    const res = await runIdeasBatch({ ideas: [a, b], perIdea: 1, ...deps })
    expect(deps.enqueue).toHaveBeenCalledTimes(1)
    expect(deps.enqueue.mock.calls[0][0].idea).toEqual(b)
    expect(res.errors).toHaveLength(1)
    expect(res.errors[0].idea).toEqual(a)
    expect(typeof res.errors[0].error).toBe('string')
    expect(res.errors[0].error).toContain('llm down')
    expect(res.jobIds).toHaveLength(1)
  })

  it('enqueue throws: remaining enqueues of that idea skipped, next idea continues', async () => {
    const a = idea(1)
    const b = idea(2)
    const deps = makeDeps({
      enqueue: vi.fn(async (p) => {
        if (p.idea.id === 1 && p.index === 1) throw new Error('queue full')
        return 'j-' + p.idea.id + '-' + p.index
      }),
    })
    const res = await runIdeasBatch({ ideas: [a, b], perIdea: 4, ...deps })
    expect(deps.enqueue.mock.calls.map(([p]) => p.idea.id + ':' + p.index)).toEqual([
      '1:0', '1:1', '2:0', '2:1', '2:2', '2:3',
    ])
    expect(res.jobIds).toEqual(['j-1-0', 'j-2-0', 'j-2-1', 'j-2-2', 'j-2-3'])
    expect(res.errors).toHaveLength(1)
    expect(res.errors[0].idea).toEqual(a)
    expect(typeof res.errors[0].error).toBe('string')
    expect(res.errors[0].error).toContain('queue full')
    expect(res.stopped).toBe(false)
  })

  it('falsy jobIds returned by enqueue are not collected', async () => {
    const values = ['j1', '', null, undefined, 0, 'j2']
    let n = 0
    const deps = makeDeps({ enqueue: vi.fn(async () => values[n++]) })
    const res = await runIdeasBatch({ ideas: [idea(1), idea(2)], perIdea: 3, ...deps })
    expect(deps.enqueue).toHaveBeenCalledTimes(6)
    expect(res.jobIds).toEqual(['j1', 'j2'])
    expect(res.errors).toEqual([])
  })

  it('isStopped true before the first idea: nothing processed, stopped=true', async () => {
    const deps = makeDeps({ isStopped: vi.fn(() => true) })
    const res = await runIdeasBatch({ ideas: [idea(1), idea(2)], perIdea: 2, ...deps })
    expect(deps.generatePrompt).not.toHaveBeenCalled()
    expect(deps.enqueue).not.toHaveBeenCalled()
    expect(res.stopped).toBe(true)
    expect(res.jobIds).toEqual([])
  })

  it('isStopped becomes true between ideas: next idea not started, stopped=true', async () => {
    let stop = false
    const deps = makeDeps({ isStopped: vi.fn(() => stop) })
    deps.onProgress = vi.fn(() => { stop = true })
    const res = await runIdeasBatch({ ideas: [idea(1), idea(2), idea(3)], perIdea: 2, ...deps })
    expect(deps.generatePrompt).toHaveBeenCalledTimes(1)
    expect(deps.enqueue).toHaveBeenCalledTimes(2)
    expect(res.jobIds).toEqual(['job-1', 'job-2'])
    expect(res.stopped).toBe(true)
  })

  it('isStopped checked before each enqueue: stop after first enqueue prevents the rest', async () => {
    let stop = false
    let job = 0
    const deps = makeDeps({
      isStopped: vi.fn(() => stop),
      enqueue: vi.fn(async () => { stop = true; return 'job-' + (++job) }),
    })
    const res = await runIdeasBatch({ ideas: [idea(1), idea(2)], perIdea: 5, ...deps })
    expect(deps.enqueue).toHaveBeenCalledTimes(1)
    expect(deps.generatePrompt).toHaveBeenCalledTimes(1)
    expect(res.jobIds).toEqual(['job-1'])
    expect(res.stopped).toBe(true)
  })

  it('isStopped becomes true during generatePrompt: no enqueue for that idea', async () => {
    let stop = false
    const deps = makeDeps({
      isStopped: vi.fn(() => stop),
      generatePrompt: vi.fn(async (i) => { stop = true; return okPrompt(i) }),
    })
    const res = await runIdeasBatch({ ideas: [idea(1), idea(2)], perIdea: 2, ...deps })
    expect(deps.enqueue).not.toHaveBeenCalled()
    expect(deps.generatePrompt).toHaveBeenCalledTimes(1)
    expect(res.stopped).toBe(true)
  })

  it('onProgress after each idea (success or error) with done/total/idea', async () => {
    const a = idea(1)
    const b = idea(2)
    const c = idea(3)
    const calls = []
    const deps = makeDeps({
      generatePrompt: vi.fn(async (i) => {
        if (i.id === 2) throw new Error('bad')
        return okPrompt(i)
      }),
      enqueue: vi.fn(async (p) => {
        calls.push('enq:' + p.idea.id)
        return 'j'
      }),
    })
    deps.onProgress = vi.fn((p) => calls.push('progress:' + p.done + '/' + p.total))
    await runIdeasBatch({ ideas: [a, b, c], perIdea: 1, ...deps })
    expect(calls).toEqual(['enq:1', 'progress:1/3', 'progress:2/3', 'enq:3', 'progress:3/3'])
    expect(deps.onProgress.mock.calls.map(([p]) => p.idea)).toEqual([a, b, c])
  })

  it('onProgress is also called after an idea whose enqueue failed', async () => {
    const deps = makeDeps({ enqueue: vi.fn(async () => { throw new Error('x') }) })
    await runIdeasBatch({ ideas: [idea(1), idea(2)], perIdea: 2, ...deps })
    expect(deps.onProgress.mock.calls.map(([p]) => [p.done, p.total])).toEqual([[1, 2], [2, 2]])
  })
})
