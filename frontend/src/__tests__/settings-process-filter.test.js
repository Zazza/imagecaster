import { describe, it, expect } from 'vitest'
import { visibleProcesses } from '../components/settings/processFilter.js'

const PROCS = {
  llm: { name: 'llama.cpp', category: 'llm' },
  sd: { name: 'SD', category: 'sd' },
  ollama: { name: 'Ollama', category: 'llm' },
  python: { name: 'Python', category: 'runtime' },
  git: { name: 'git-tool', category: 'utility' },
  ollamaUtil: { name: 'Ollama util', category: 'utility' },
  unnamed: { category: 'sd' },
  unnamedUtil: { name: '', category: 'utility' },
}

const names = list => list.map(([n]) => n)

describe('settings process filter', () => {
  it('services: non-utility named processes except ollama; python stays as before', () => {
    expect(names(visibleProcesses(PROCS, 'service'))).toEqual(['llm', 'sd', 'python'])
  })

  it('utilities: named utility processes except ollama and python', () => {
    const procs = { ...PROCS, ollama: { name: 'Ollama', category: 'utility' }, python: { name: 'Python', category: 'utility' } }
    expect(names(visibleProcesses(procs, 'utility'))).toEqual(['git', 'ollamaUtil'])
  })

  it.each([null, undefined, {}])('empty input %s gives no rows', (procs) => {
    expect(visibleProcesses(procs, 'service')).toEqual([])
    expect(visibleProcesses(procs, 'utility')).toEqual([])
  })

  it('keeps the process object and server order', () => {
    const [[name, proc]] = visibleProcesses({ sd: PROCS.sd }, 'service')
    expect(name).toBe('sd')
    expect(proc).toBe(PROCS.sd)
  })
})
