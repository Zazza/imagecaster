import { describe, it, expect } from 'vitest'
import { sortEntries } from '../fileSort.js'

const dir = (name, mod_time) => ({ name, path: `/x/${name}`, is_dir: true, size: 0, mod_time })
const file = (name, mod_time) => ({ name, path: `/x/${name}`, is_dir: false, size: 10, mod_time })

const fixture = () => [
  file('img10', '2026-01-05 10:00'),
  dir('beta', '2026-01-02 10:00'),
  file('B', '2026-01-03 10:00'),
  file('a', '2026-01-01 10:00'),
  dir('Alpha', '2026-01-04 10:00'),
  file('img2', '2026-01-03 10:00'),
]

const names = (list) => list.map((e) => e.name)

describe('sortEntries', () => {
  it.each([
    ['S1 name asc', 'name', 'asc', ['Alpha', 'beta', 'a', 'B', 'img2', 'img10']],
    ['S2 name desc', 'name', 'desc', ['beta', 'Alpha', 'img10', 'img2', 'B', 'a']],
    ['S3 date desc, equal date by name asc', 'date', 'desc', ['Alpha', 'beta', 'img10', 'B', 'img2', 'a']],
    ['S4 date asc, folders still first', 'date', 'asc', ['beta', 'Alpha', 'a', 'B', 'img2', 'img10']],
  ])('%s', (_label, key, direction, expected) => {
    expect(names(sortEntries(fixture(), key, direction))).toEqual(expected)
  })

  it('S5 does not mutate input and returns a new array', () => {
    const input = fixture()
    const snapshot = JSON.parse(JSON.stringify(input))
    const out = sortEntries(input, 'name', 'asc')
    expect(out).not.toBe(input)
    expect(input).toEqual(snapshot)
  })

  it.each([
    ['unknown key', 'size', 'desc'],
    ['unknown dir', 'name', 'sideways'],
    ['both unknown', undefined, undefined],
  ])('S5 %s falls back to date desc', (_label, key, direction) => {
    expect(names(sortEntries(fixture(), key, direction))).toEqual(
      names(sortEntries(fixture(), 'date', 'desc')),
    )
    expect(names(sortEntries(fixture(), key, direction))).toEqual(['Alpha', 'beta', 'img10', 'B', 'img2', 'a'])
  })

  it('empty list returns empty array', () => {
    expect(sortEntries([], 'name', 'asc')).toEqual([])
  })
})
