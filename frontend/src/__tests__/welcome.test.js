import { describe, it, expect } from 'vitest'
import { WELCOME_KEY, welcomeTab, welcomeClosed, isFreshInstall, startupTab } from '../welcome.js'
import { RELEASE_NOTES } from '../releaseNotes.js'

describe('WELCOME_KEY', () => {
  it('is a non-empty string', () => {
    expect(typeof WELCOME_KEY).toBe('string')
    expect(WELCOME_KEY.length).toBeGreaterThan(0)
  })
})

describe('welcomeTab', () => {
  it.each([
    ['null', null],
    ['undefined', undefined],
    ['empty object', {}],
  ])('first launch (%s) gives about, not news, even with version', (_, state) => {
    expect(welcomeTab(state, '1.2.0')).toBe('about')
    expect(welcomeTab(state, '')).toBe('about')
  })

  it('new version gives news when not dismissed', () => {
    expect(welcomeTab({ dismissed: false, seenVersion: '1.1.0' }, '1.2.0')).toBe('news')
  })

  it('new version gives news even when dismissed', () => {
    expect(welcomeTab({ dismissed: true, seenVersion: '1.1.0' }, '1.2.0')).toBe('news')
  })

  it('same version, not dismissed gives about', () => {
    expect(welcomeTab({ dismissed: false, seenVersion: '1.2.0' }, '1.2.0')).toBe('about')
  })

  it('same version, dismissed gives null', () => {
    expect(welcomeTab({ dismissed: true, seenVersion: '1.2.0' }, '1.2.0')).toBeNull()
  })

  it.each([
    ['empty string', ''],
    ['undefined', undefined],
    ['null', null],
  ])('missing current version (%s) never gives news', (_, ver) => {
    expect(welcomeTab({ dismissed: false, seenVersion: '1.1.0' }, ver)).not.toBe('news')
    expect(welcomeTab({ dismissed: true, seenVersion: '1.1.0' }, ver)).not.toBe('news')
    expect(welcomeTab(null, ver)).not.toBe('news')
  })
})

describe('welcomeClosed', () => {
  it('sets seenVersion to current version', () => {
    expect(welcomeClosed({ dismissed: false, seenVersion: '1.0.0' }, '1.2.0', false)).toEqual({ dismissed: false, seenVersion: '1.2.0' })
  })

  it('dontShow true sets dismissed', () => {
    expect(welcomeClosed({ dismissed: false, seenVersion: '1.0.0' }, '1.2.0', true)).toEqual({ dismissed: true, seenVersion: '1.2.0' })
  })

  it('once dismissed stays dismissed when dontShow is false', () => {
    expect(welcomeClosed({ dismissed: true, seenVersion: '1.0.0' }, '1.2.0', false)).toEqual({ dismissed: true, seenVersion: '1.2.0' })
  })

  it('works with null state', () => {
    expect(welcomeClosed(null, '1.2.0', false)).toEqual({ dismissed: false, seenVersion: '1.2.0' })
    expect(welcomeClosed(null, '1.2.0', true)).toEqual({ dismissed: true, seenVersion: '1.2.0' })
  })

  it('works with undefined state', () => {
    expect(welcomeClosed(undefined, '1.2.0', false)).toEqual({ dismissed: false, seenVersion: '1.2.0' })
  })

  it('empty current version keeps old seenVersion', () => {
    expect(welcomeClosed({ dismissed: false, seenVersion: '1.0.0' }, '', true)).toEqual({ dismissed: true, seenVersion: '1.0.0' })
  })

  it('empty current version with no old seenVersion gives empty string', () => {
    expect(welcomeClosed(null, '', false)).toEqual({ dismissed: false, seenVersion: '' })
    expect(welcomeClosed({}, '', true)).toEqual({ dismissed: true, seenVersion: '' })
  })

  it('round trip: dismissed result with same version gives null', () => {
    const s = welcomeClosed(null, '1.2.0', true)
    expect(welcomeTab(s, '1.2.0')).toBeNull()
  })

  it('round trip: not dismissed result with same version gives about', () => {
    const s = welcomeClosed(null, '1.2.0', false)
    expect(welcomeTab(s, '1.2.0')).toBe('about')
  })

  it.each([true, false])('round trip: newer version gives news (dontShow=%s)', (dontShow) => {
    const s = welcomeClosed(null, '1.2.0', dontShow)
    expect(welcomeTab(s, '1.3.0')).toBe('news')
  })

  it('does not mutate input state', () => {
    const state = { dismissed: false, seenVersion: '1.0.0' }
    welcomeClosed(state, '1.2.0', true)
    expect(state).toEqual({ dismissed: false, seenVersion: '1.0.0' })
  })
})

describe('isFreshInstall', () => {
  const sdPlan = { items: [{ type: 'vae' }, { type: 'sd' }] }

  it('true when no sd models and plan has an sd item', () => {
    expect(isFreshInstall(sdPlan, 0)).toBe(true)
    expect(isFreshInstall({ items: [{ type: 'sd' }] }, 0)).toBe(true)
  })

  it.each([
    ['null plan', null, 0],
    ['undefined plan', undefined, 0],
    ['items missing', {}, 0],
    ['items not an array', { items: { type: 'sd' } }, 0],
    ['items is a string', { items: 'sd' }, 0],
    ['items empty', { items: [] }, 0],
    ['only non-sd items', { items: [{ type: 'lora' }, { type: 'vae' }] }, 0],
    ['sd models already present', sdPlan, 1],
    ['many sd models', sdPlan, 5],
    ['model count undefined', sdPlan, undefined],
    ['model count null', sdPlan, null],
    ['model count string "0"', sdPlan, '0'],
  ])('false: %s', (_, plan, count) => {
    expect(isFreshInstall(plan, count)).toBe(false)
  })
})

describe('startupTab', () => {
  it('returns welcomeTab result when not null (first launch -> about)', () => {
    expect(startupTab(null, '1.2.0', true)).toBe('about')
    expect(startupTab(null, '1.2.0', false)).toBe('about')
  })

  it('new version wins with news even on fresh install', () => {
    expect(startupTab({ dismissed: true, seenVersion: '1.1.0' }, '1.2.0', true)).toBe('news')
  })

  it('same version not dismissed gives about even on fresh install', () => {
    expect(startupTab({ dismissed: false, seenVersion: '1.2.0' }, '1.2.0', true)).toBe('about')
  })

  it('dismissed user on fresh install gets models', () => {
    expect(startupTab({ dismissed: true, seenVersion: '1.2.0' }, '1.2.0', true)).toBe('models')
  })

  it('dismissed user, not fresh gets null', () => {
    expect(startupTab({ dismissed: true, seenVersion: '1.2.0' }, '1.2.0', false)).toBeNull()
  })

  it('works with isFreshInstall result', () => {
    const fresh = isFreshInstall({ items: [{ type: 'sd' }] }, 0)
    expect(startupTab({ dismissed: true, seenVersion: '1.2.0' }, '1.2.0', fresh)).toBe('models')
  })
})

describe('RELEASE_NOTES', () => {
  const semver = /^\d+\.\d+\.\d+$/
  const cmp = (a, b) => {
    const pa = a.split('.').map(Number)
    const pb = b.split('.').map(Number)
    for (let i = 0; i < 3; i++) if (pa[i] !== pb[i]) return pa[i] - pb[i]
    return 0
  }

  it('is a non-empty array', () => {
    expect(Array.isArray(RELEASE_NOTES)).toBe(true)
    expect(RELEASE_NOTES.length).toBeGreaterThan(0)
  })

  it('each entry has semver version and ru/en lists of equal length, at least 3 non-empty strings', () => {
    for (const n of RELEASE_NOTES) {
      expect(n.version).toMatch(semver)
      expect(Array.isArray(n.ru)).toBe(true)
      expect(Array.isArray(n.en)).toBe(true)
      expect(n.ru.length).toBeGreaterThanOrEqual(3)
      expect(n.en.length).toBe(n.ru.length)
      for (const s of [...n.ru, ...n.en]) {
        expect(typeof s).toBe('string')
        expect(s.trim().length).toBeGreaterThan(0)
      }
    }
  })

  it('versions are unique', () => {
    const vs = RELEASE_NOTES.map(n => n.version)
    expect(new Set(vs).size).toBe(vs.length)
  })

  it('versions are sorted newest first', () => {
    for (let i = 1; i < RELEASE_NOTES.length; i++) {
      expect(cmp(RELEASE_NOTES[i - 1].version, RELEASE_NOTES[i].version)).toBeGreaterThan(0)
    }
  })
})
