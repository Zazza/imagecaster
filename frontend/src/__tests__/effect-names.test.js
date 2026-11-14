import { describe, it, expect, beforeEach } from 'vitest'
import { effectNameKey, effectLabel } from '../effectNames.js'
import { t, setLocale } from '../i18n/index.js'

describe('effectNameKey', () => {
  it.each([
    ['Photo Restore', 'post.effect_photo_restore'],
    ['  Photo -- Restore!  ', 'post.effect_photo_restore'],
    ['Photo Restore (Qwen)', 'post.effect_photo_restore'],
    ['Photo Restore (v2) (Qwen)', 'post.effect_photo_restore_v2'],
  ])('%j -> %s', (name, key) => {
    expect(effectNameKey(name)).toBe(key)
  })
})

describe('effectLabel', () => {
  beforeEach(() => setLocale('ru'))

  it('translates a known effect name', () => {
    expect(effectLabel('Photo Restore')).toBe(t('post.effect_photo_restore'))
  })

  it('keeps the "(...)" variant after the translated base name', () => {
    expect(effectLabel('Photo Restore (Qwen)')).toBe(t('post.effect_photo_restore') + ' (Qwen)')
  })

  it('returns the original name when there is no translation', () => {
    expect(effectLabel('Unknown Effect Xyz (Qwen)')).toBe('Unknown Effect Xyz (Qwen)')
  })

  it('uses the passed translator', () => {
    expect(effectLabel('Photo Restore (Qwen)', k => k.toUpperCase())).toBe('POST.EFFECT_PHOTO_RESTORE (Qwen)')
  })
})
