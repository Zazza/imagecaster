import { describe, it, expect, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { setLocale } from '../i18n/index.js'
import SeriesCard from '../components/stylist/SeriesCard.vue'
import { setShotWear, shotWearText, keepWear, dirtyRefs, applyTranslations } from '../composables/useStylistPlan.js'

enableAutoUnmount(afterEach)

const tid = (id) => `[data-testid="${id}"]`

const SERIES = () => ({
  outfit: 'white blouse', hair: 'bun', makeup: 'none', background: 'park', place: 'park',
  shots: [{ pose: 'Сидит', prompt: 'sitting' }, { pose: 'Стоит', prompt: 'standing' }],
})

describe('shot wear: helpers', () => {
  it('latin text goes straight to wear', () => {
    const s = setShotWear(SERIES(), 0, ' torn sleeve ')
    expect(s.shots[0]).toMatchObject({ wear: 'torn sleeve' })
    expect(s.shots[0].wear_dirty).toBeUndefined()
    expect(s.shots[1].wear).toBeUndefined()
  })

  it('cyrillic text waits for translation', () => {
    const s = setShotWear(SERIES(), 1, 'порван рукав')
    expect(s.shots[1]).toMatchObject({ wear_ru: 'порван рукав', wear_dirty: true })
    expect(s.shots[1].wear).toBeUndefined()
    expect(dirtyRefs([s])).toEqual([{ series: 0, field: 'wear', shot: 1, text: 'порван рукав' }])
  })

  it('empty text clears the wear', () => {
    const s = setShotWear(setShotWear(SERIES(), 0, 'порван рукав'), 0, '  ')
    expect(s.shots[0]).toEqual({ pose: 'Сидит', prompt: 'sitting' })
  })

  it('translation fills wear and keeps the russian text for display', () => {
    const s = setShotWear(SERIES(), 0, 'порван рукав')
    const refs = dirtyRefs([s])
    const [out] = applyTranslations([s], refs, ['torn sleeve'])
    expect(out.shots[0]).toMatchObject({ wear: 'torn sleeve', wear_ru: 'порван рукав' })
    expect(out.shots[0].wear_dirty).toBeUndefined()
    expect(out.shots[0].prompt).toBe('sitting')
    expect(shotWearText(out.shots[0])).toBe('порван рукав')
    expect(shotWearText(out.shots[0], true)).toBe('torn sleeve')
  })

  it('translation is dropped when the text changed meanwhile', () => {
    const s = setShotWear(SERIES(), 0, 'порван рукав')
    const refs = dirtyRefs([s])
    const changed = setShotWear(s, 0, 'полы разошлись')
    const [out] = applyTranslations([changed], refs, ['torn sleeve'])
    expect(out.shots[0]).toMatchObject({ wear_ru: 'полы разошлись', wear_dirty: true })
    expect(out.shots[0].wear).toBeUndefined()
  })

  it('regenerated series keeps wear by shot index', () => {
    const prev = setShotWear(setShotWear(SERIES(), 0, 'torn sleeve'), 1, 'порван рукав')
    const next = { ...SERIES(), outfit: 'red dress', shots: [{ pose: 'Лёжа', prompt: 'lying' }, { pose: 'X', prompt: 'x' }, { pose: 'Y', prompt: 'y' }] }
    const out = keepWear(prev, next)
    expect(out.outfit).toBe('red dress')
    expect(out.shots[0]).toMatchObject({ prompt: 'lying', wear: 'torn sleeve' })
    expect(out.shots[1]).toMatchObject({ prompt: 'x', wear_ru: 'порван рукав', wear_dirty: true })
    expect(out.shots[2].wear).toBeUndefined()
  })
})

describe('shot wear: SeriesCard', () => {
  afterEach(() => setLocale('ru'))

  it('every shot has a wear field and a change emits update', async () => {
    setLocale('ru')
    const w = mount(SeriesCard, { props: { series: SERIES(), index: 0 } })
    const inputs = w.findAll(tid('shot-wear'))
    expect(inputs).toHaveLength(2)
    await inputs[1].setValue('порван рукав')
    const [next] = w.emitted('update').at(-1)
    expect(next.shots[1]).toMatchObject({ wear_ru: 'порван рукав', wear_dirty: true })
  })

  it('shows russian text and the translate hint while dirty', () => {
    setLocale('ru')
    const w = mount(SeriesCard, { props: { series: setShotWear(SERIES(), 0, 'порван рукав'), index: 0 } })
    expect(w.findAll(tid('shot-wear'))[0].element.value).toBe('порван рукав')
    expect(w.find(tid('series-dirty')).exists()).toBe(true)
  })
})
