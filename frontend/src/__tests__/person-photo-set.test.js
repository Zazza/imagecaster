import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'

import FileBrowserPage from '../components/FileBrowserPage.vue'
import ImageViewer from '../components/ImageViewer.vue'
import AddToSetDialog from '../components/people/AddToSetDialog.vue'
import PeoplePage from '../components/PeoplePage.vue'
import {
  GetSettings, BrowseDirectory, ReadFileAsBase64, ReadThumbnail,
  PersonSetAdd, PersonSetSummary, PersonSetList, PersonSetThumb, PersonSetRemove,
  TrainPersonFromSet, AddPersonIdentityFromSet, TrainPerson, PickPeopleFolder,
  ListPeople, TrainingStatus, FaceSwapStatus, TrainingBases, ListTrainingJobs,
} from '../wailsjs/go/main/App.js'
import { t, setLocale } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

const TID = id => `[data-testid="${id}"]`

function findAllDeep(wrapper, id) {
  const inside = wrapper.findAll(TID(id))
  if (inside.length) return inside.map(w => w.element)
  return Array.from(document.body.querySelectorAll(TID(id)))
}

function findDeep(wrapper, id) {
  return findAllDeep(wrapper, id)[0] || null
}

async function click(el) {
  el.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  await settle()
}

async function chooseOption(selectEl, optionText) {
  const opt = Array.from(selectEl.options).find(o => o.textContent.trim().includes(optionText))
  expect(opt, `option «${optionText}»`).toBeTruthy()
  selectEl.value = opt.value
  selectEl.dispatchEvent(new Event('change', { bubbles: true }))
  selectEl.dispatchEvent(new Event('input', { bubbles: true }))
  await settle()
}

async function typeInto(inputEl, value) {
  inputEl.value = value
  inputEl.dispatchEvent(new Event('input', { bubbles: true }))
  inputEl.dispatchEvent(new Event('change', { bubbles: true }))
  await settle()
}

async function activate(el) {
  if (el.tagName === 'INPUT' && (el.type === 'radio' || el.type === 'checkbox')) {
    el.checked = true
    el.dispatchEvent(new Event('change', { bubbles: true }))
    await settle()
    return
  }
  await click(el)
}

const OK = path => Promise.resolve({ photo: { id: Math.floor(Math.random() * 1e6), source_name: path.split('/').pop() } })

beforeEach(() => {
  vi.clearAllMocks()
  vi.useRealTimers()
  setLocale('ru')
  document.body.innerHTML = ''
  TrainingStatus.mockImplementation(() => Promise.resolve('ok'))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
  TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image']))
  ListTrainingJobs.mockImplementation(() => Promise.resolve([]))
  ListPeople.mockImplementation(() => Promise.resolve([]))
  PersonSetSummary.mockImplementation(() => Promise.resolve([]))
  PersonSetList.mockImplementation(() => Promise.resolve([]))
  PersonSetThumb.mockImplementation(() => Promise.resolve('QUJD'))
  PersonSetRemove.mockImplementation(() => Promise.resolve())
  PersonSetAdd.mockImplementation(({ path }) => OK(path))
  AddPersonIdentityFromSet.mockImplementation(() => Promise.resolve({}))
  TrainPersonFromSet.mockImplementation((key, name, gender, base) => Promise.resolve({ id: 'j1', name, base, status: 'queued', progress: { step: 0, steps: 2000 } }))
  PickPeopleFolder.mockImplementation(() => Promise.resolve('/photos/kira'))
})

describe('condition 1: «Мои изображения» → «В набор для обучения»', () => {
  const DIR = '/pics'
  const ENTRIES = [
    { name: 'one.png', path: '/pics/one.png', is_dir: false, size: 100, mod_time: '2026-11-07 10:00' },
    { name: 'two.png', path: '/pics/two.png', is_dir: false, size: 100, mod_time: '2026-11-07 10:00' },
    { name: 'three.png', path: '/pics/three.png', is_dir: false, size: 100, mod_time: '2026-11-07 10:00' },
  ]

  beforeEach(() => {
    GetSettings.mockImplementation(() => Promise.resolve({ file_browser_path: DIR }))
    BrowseDirectory.mockImplementation(() => Promise.resolve(ENTRIES.map(e => ({ ...e }))))
    ReadFileAsBase64.mockImplementation(() => Promise.resolve('QUJD'))
    ReadThumbnail.mockImplementation(() => Promise.resolve('QUJD'))
  })

  function imageCards(w) {
    return w.findAll('.fb-card').filter(c => !c.classes().includes('fb-card-dir'))
  }

  function cardPath(card) {
    const name = card.find('.fb-card-name').text().trim()
    return ENTRIES.find(e => e.name === name).path
  }

  async function mountPage() {
    const w = mount(FileBrowserPage, { attachTo: document.body })
    await settle()
    expect(imageCards(w)).toHaveLength(ENTRIES.length)
    return w
  }

  it('select mode: button «В набор для обучения (N)» opens the dialog with the selected paths', async () => {
    const w = await mountPage()
    const selectBtn = w.findAll('button').find(b => b.text().trim() === t('browser.select_mode'))
    await selectBtn.trigger('click')
    await settle()
    const cards = imageCards(w)
    const picked = [cardPath(cards[0]), cardPath(cards[2])]
    await cards[0].trigger('click')
    await settle()
    await cards[2].trigger('click')
    await settle()

    const btn = w.find(TID('fb-to-set'))
    expect(btn.exists()).toBe(true)
    expect(btn.text()).toContain('В набор для обучения')
    expect(btn.text()).toContain('(2)')
    expect(findDeep(w, 'set-dialog')).toBeNull()

    await btn.trigger('click')
    await settle()
    expect(findDeep(w, 'set-dialog')).not.toBeNull()
    const dlg = w.findComponent(AddToSetDialog)
    expect(dlg.exists()).toBe(true)
    expect([...dlg.props('paths')].sort()).toEqual([...picked].sort())
  })

  it('single-photo viewer: «В набор для обучения» opens the dialog with that one path', async () => {
    const w = await mountPage()
    const card = imageCards(w)[1]
    const path = cardPath(card)
    await card.trigger('click')
    await settle()

    const btn = findDeep(w, 'fb-viewer-to-set')
    expect(btn).not.toBeNull()
    expect(btn.textContent.trim()).toBe('В набор для обучения')
    await click(btn)
    expect(findDeep(w, 'set-dialog')).not.toBeNull()
    const dlg = w.findComponent(AddToSetDialog)
    expect(dlg.exists()).toBe(true)
    expect([...dlg.props('paths')]).toEqual([path])
  })

  it('dialog over the viewer: Esc and arrows do not close or flip the viewer under it', async () => {
    const w = await mountPage()
    await imageCards(w)[1].trigger('click')
    await settle()
    await click(findDeep(w, 'fb-viewer-to-set'))
    const viewer = w.findComponent(ImageViewer)
    for (const key of ['Escape', 'ArrowRight', 'ArrowLeft']) {
      document.dispatchEvent(new KeyboardEvent('keydown', { key }))
    }
    await settle()
    expect(viewer.emitted('close')).toBeUndefined()
    expect(viewer.emitted('next')).toBeUndefined()
    expect(viewer.emitted('prev')).toBeUndefined()
    expect(findDeep(w, 'set-dialog')).not.toBeNull()
  })
})

describe('conditions 2–5: AddToSetDialog', () => {
  const PATHS = ['/p/a.jpg', '/p/b.jpg', '/p/c.jpg']

  async function mountDialog(paths = PATHS) {
    const w = mount(AddToSetDialog, { props: { paths }, attachTo: document.body })
    await settle()
    expect(findDeep(w, 'set-dialog')).not.toBeNull()
    return w
  }

  async function pickExisting(w, key) {
    await chooseOption(findDeep(w, 'set-key'), key)
  }

  function calls() {
    return PersonSetAdd.mock.calls.map(c => c[0])
  }

  it('2: a click outside the dialog (or a drag released outside) does not close it', async () => {
    const w = await mountDialog(['/p/a.jpg'])
    const overlay = document.querySelector('.set-overlay')
    overlay.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    overlay.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await settle()
    expect(w.emitted('close')).toBeUndefined()
  })

  it('5: summary shows how many photos the set has now and how many are still needed', async () => {
    let count = 2
    PersonSetSummary.mockImplementation(() => Promise.resolve([{ key: 'Кира', count, front: count, half: 0, profile: 0, last_added: '' }]))
    PersonSetAdd.mockImplementation(({ path }) => { count++; return OK(path) })
    const w = await mountDialog(['/p/a.jpg'])
    await click(findDeep(w, 'set-add'))
    await settle()
    const total = findDeep(w, 'set-total')
    expect(total).not.toBeNull()
    expect(total.textContent).toContain('В наборе «Кира» теперь 3 фото')
    expect(total.textContent).toContain('нужно ещё 7 фото')
  })

  it('2: person select lists set keys, people display names and «+ Новый человек»', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([{ key: 'Кира', count: 3, front: 3, half: 0, profile: 0, last_added: '' }]))
    ListPeople.mockImplementation(() => Promise.resolve([{ name: 'natasha_v1', lora: 'natasha_v1', base: 'Z-Image', display_name: 'Наташа' }]))
    const w = await mountDialog()
    const sel = findDeep(w, 'set-key')
    expect(sel).not.toBeNull()
    const texts = Array.from(sel.options).map(o => o.textContent.trim())
    expect(texts.some(s => s.includes('Кира'))).toBe(true)
    expect(texts.some(s => s.includes('Наташа'))).toBe(true)
    expect(texts.some(s => s.includes('+ Новый человек'))).toBe(true)
  })

  it('2: new person — input appears, «Добавить» disabled until a name is typed, then sends that key', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([{ key: 'Кира', count: 3, front: 3, half: 0, profile: 0, last_added: '' }]))
    const w = await mountDialog(['/p/a.jpg'])
    await chooseOption(findDeep(w, 'set-key'), '+ Новый человек')
    const input = findDeep(w, 'set-new-key')
    expect(input).not.toBeNull()
    const add = findDeep(w, 'set-add')
    expect(add.disabled).toBe(true)
    await click(add)
    expect(PersonSetAdd).not.toHaveBeenCalled()

    await typeInto(input, 'Оля')
    expect(findDeep(w, 'set-add').disabled).toBe(false)
    await click(findDeep(w, 'set-add'))
    expect(PersonSetAdd).toHaveBeenCalledTimes(1)
    expect(calls()[0]).toMatchObject({ path: '/p/a.jpg', key: 'Оля' })
  })

  it('2: new person typed like an existing Имя in other case/spaces goes to that set', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([{ key: 'Анна Мария', count: 3, front: 3, half: 0, profile: 0, last_added: '' }]))
    const w = await mountDialog(['/p/a.jpg'])
    await chooseOption(findDeep(w, 'set-key'), '+ Новый человек')
    await typeInto(findDeep(w, 'set-new-key'), '  анна   мария ')
    await click(findDeep(w, 'set-add'))
    expect(calls()[0]).toMatchObject({ key: 'Анна Мария' })
  })

  it('2: new person with whitespace-only name keeps «Добавить» disabled', async () => {
    const w = await mountDialog(['/p/a.jpg'])
    await chooseOption(findDeep(w, 'set-key'), '+ Новый человек')
    await typeInto(findDeep(w, 'set-new-key'), '   ')
    expect(findDeep(w, 'set-add').disabled).toBe(true)
  })

  it('2: sends every path with the chosen existing key, in order, without face_index/confirm', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([{ key: 'Кира', count: 3, front: 3, half: 0, profile: 0, last_added: '' }]))
    const w = await mountDialog()
    await pickExisting(w, 'Кира')
    await click(findDeep(w, 'set-add'))
    expect(calls().map(c => c.path)).toEqual(PATHS)
    for (const c of calls()) {
      expect(c.key).toBe('Кира')
      expect(c.face_index).toBeUndefined()
      expect(c.confirm).toBeFalsy()
    }
  })

  it('2: display name of a person (no set yet) can be chosen as key', async () => {
    ListPeople.mockImplementation(() => Promise.resolve([{ name: 'natasha_v1', lora: 'natasha_v1', base: 'Z-Image', display_name: 'Наташа' }]))
    const w = await mountDialog(['/p/a.jpg'])
    await pickExisting(w, 'Наташа')
    await click(findDeep(w, 'set-add'))
    expect(calls()[0]).toMatchObject({ path: '/p/a.jpg', key: 'Наташа' })
  })

  it('3: pick_face — faces are clickable, click resends with face_index, then continues', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([{ key: 'Кира', count: 3, front: 3, half: 0, profile: 0, last_added: '' }]))
    PersonSetAdd.mockImplementation(({ path, face_index }) => {
      if (path === '/p/a.jpg' && face_index === undefined) {
        return Promise.resolve({ pick_face: true, preview: 'QUJD', faces: [{ box: [0.1, 0.1, 0.3, 0.4] }, { box: [0.5, 0.1, 0.7, 0.4] }] })
      }
      return OK(path)
    })
    const w = await mountDialog(['/p/a.jpg', '/p/b.jpg'])
    await pickExisting(w, 'Кира')
    await click(findDeep(w, 'set-add'))

    expect(PersonSetAdd).toHaveBeenCalledTimes(1)
    expect(findDeep(w, 'set-face-0')).not.toBeNull()
    const face1 = findDeep(w, 'set-face-1')
    expect(face1).not.toBeNull()

    await click(face1)
    expect(calls()[1]).toMatchObject({ path: '/p/a.jpg', key: 'Кира', face_index: 1 })
    expect(calls()[2]).toMatchObject({ path: '/p/b.jpg', key: 'Кира' })
    expect(calls()[2].face_index).toBeUndefined()
    expect(PersonSetAdd).toHaveBeenCalledTimes(3)
    const summary = findDeep(w, 'set-summary')
    expect(summary).not.toBeNull()
    expect(summary.textContent).toMatch(/Добавлено 2 из 2/)
  })

  it('3: face_index 0 is sent as 0 (not dropped as falsy)', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([{ key: 'Кира', count: 3, front: 3, half: 0, profile: 0, last_added: '' }]))
    PersonSetAdd.mockImplementation(({ path, face_index }) => face_index === undefined
      ? Promise.resolve({ pick_face: true, preview: 'QUJD', faces: [{ box: [0.1, 0.1, 0.3, 0.4] }, { box: [0.5, 0.1, 0.7, 0.4] }] })
      : OK(path))
    const w = await mountDialog(['/p/a.jpg'])
    await pickExisting(w, 'Кира')
    await click(findDeep(w, 'set-add'))
    await click(findDeep(w, 'set-face-0'))
    expect(calls()[1]).toMatchObject({ path: '/p/a.jpg', face_index: 0 })
  })

  it('4: confirm — «Да, добавить» resends with confirm:true and continues', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([{ key: 'Кира', count: 3, front: 3, half: 0, profile: 0, last_added: '' }]))
    PersonSetAdd.mockImplementation(({ path, confirm }) => {
      if (path === '/p/a.jpg' && !confirm) return Promise.resolve({ confirm: 'похоже на уже добавленное фото' })
      return OK(path)
    })
    const w = await mountDialog(['/p/a.jpg', '/p/b.jpg'])
    await pickExisting(w, 'Кира')
    await click(findDeep(w, 'set-add'))
    expect(PersonSetAdd).toHaveBeenCalledTimes(1)
    const yes = findDeep(w, 'set-confirm')
    expect(yes).not.toBeNull()
    expect(yes.textContent.trim()).toBe('Да, добавить')
    expect(findDeep(w, 'set-skip').textContent.trim()).toBe('Пропустить')

    await click(yes)
    expect(calls()[1]).toMatchObject({ path: '/p/a.jpg', key: 'Кира', confirm: true })
    expect(calls()[2]).toMatchObject({ path: '/p/b.jpg', key: 'Кира' })
    expect(findDeep(w, 'set-summary').textContent).toMatch(/Добавлено 2 из 2/)
  })

  it('4: confirm — «Пропустить» does not resend, counts the file as skipped, continues', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([{ key: 'Кира', count: 3, front: 3, half: 0, profile: 0, last_added: '' }]))
    PersonSetAdd.mockImplementation(({ path, confirm }) => {
      if (path === '/p/a.jpg' && !confirm) return Promise.resolve({ confirm: 'похоже на уже добавленное фото' })
      return OK(path)
    })
    const w = await mountDialog(['/p/a.jpg', '/p/b.jpg'])
    await pickExisting(w, 'Кира')
    await click(findDeep(w, 'set-add'))
    await click(findDeep(w, 'set-skip'))

    expect(calls().filter(c => c.path === '/p/a.jpg')).toHaveLength(1)
    expect(calls().some(c => c.confirm === true)).toBe(false)
    expect(calls().map(c => c.path)).toEqual(['/p/a.jpg', '/p/b.jpg'])
    expect(findDeep(w, 'set-summary').textContent).toMatch(/Добавлено 1 из 2/)
  })

  it('5: an error on one file does not stop the rest; summary and per-file reasons/warnings', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([{ key: 'Кира', count: 3, front: 3, half: 0, profile: 0, last_added: '' }]))
    PersonSetAdd.mockImplementation(({ path }) => {
      if (path === '/p/a.jpg') return Promise.reject(new Error('на фото нет лица'))
      if (path === '/p/b.jpg') return Promise.resolve({ photo: { id: 2, source_name: 'b.jpg' }, warnings: ['фото слегка размыто'] })
      return OK(path)
    })
    const w = await mountDialog()
    await pickExisting(w, 'Кира')
    await click(findDeep(w, 'set-add'))

    expect(calls().map(c => c.path)).toEqual(PATHS)
    const summary = findDeep(w, 'set-summary')
    expect(summary).not.toBeNull()
    expect(summary.textContent).toMatch(/Добавлено 2 из 3/)
    const items = findAllDeep(w, 'set-item').map(e => e.textContent)
    expect(items.some(s => s.includes('на фото нет лица'))).toBe(true)
    expect(items.some(s => s.includes('фото слегка размыто'))).toBe(true)
  })

  it('5: zero added — summary says 0 of N', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([{ key: 'Кира', count: 3, front: 3, half: 0, profile: 0, last_added: '' }]))
    PersonSetAdd.mockImplementation(() => Promise.reject(new Error('плохой файл')))
    const w = await mountDialog(['/p/a.jpg', '/p/b.jpg'])
    await pickExisting(w, 'Кира')
    await click(findDeep(w, 'set-add'))
    expect(PersonSetAdd).toHaveBeenCalledTimes(2)
    expect(findDeep(w, 'set-summary').textContent).toMatch(/Добавлено 0 из 2/)
  })
})

describe('conditions 6–7: «Люди» → «Наборы фото»', () => {
  const KIRA = { key: 'Кира', count: 12, front: 7, half: 3, profile: 2, last_added: '2026-11-19T10:00:00Z' }
  const OLYA = { key: 'Оля', count: 6, front: 6, half: 0, profile: 0, last_added: '2026-11-19T10:00:00Z' }
  const NADYA = { key: 'Надя', count: 3, front: 3, half: 0, profile: 0, last_added: '2026-11-19T10:00:00Z' }

  async function mountPeople() {
    const w = mount(PeoplePage, { attachTo: document.body })
    await settle()
    return w
  }

  function cardOf(w, key) {
    return w.findAll(TID('set-card')).find(c => c.text().includes(key))
  }

  it('6: card «Наборы фото» lists each set with count and angles', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([KIRA, OLYA]))
    const w = await mountPeople()
    expect(w.text()).toContain('Наборы фото')
    const cards = w.findAll(TID('set-card'))
    expect(cards).toHaveLength(2)
    const kira = cardOf(w, 'Кира')
    expect(kira).toBeTruthy()
    expect(kira.find(TID('set-count')).text()).toContain('12')
    const angles = kira.find(TID('set-angles')).text()
    expect(angles).toContain('7')
    expect(angles).toContain('3')
    expect(angles).toContain('2')
  })

  it('6: fewer than 10 photos — hint shown and «Обучить» disabled; 10+ — no hint, enabled', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([KIRA, OLYA]))
    const w = await mountPeople()
    const olya = cardOf(w, 'Оля')
    expect(olya.find(TID('set-missing')).exists()).toBe(true)
    expect(olya.find(TID('set-train')).element.disabled).toBe(true)
    const kira = cardOf(w, 'Кира')
    expect(kira.find(TID('set-missing')).exists()).toBe(false)
    expect(kira.find(TID('set-train')).element.disabled).toBe(false)
  })

  it('6: exactly 10 photos is enough to train', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([{ ...KIRA, count: 10 }]))
    const w = await mountPeople()
    const kira = cardOf(w, 'Кира')
    expect(kira.find(TID('set-missing')).exists()).toBe(false)
    expect(kira.find(TID('set-train')).element.disabled).toBe(false)
  })

  it('6: «Фото» shows thumbnails; × removes the photo by id', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([KIRA]))
    PersonSetList.mockImplementation(key => Promise.resolve(key === 'Кира'
      ? [{ id: 41, source_name: 'a.jpg' }, { id: 42, source_name: 'b.jpg' }]
      : []))
    const w = await mountPeople()
    const kira = cardOf(w, 'Кира')
    expect(kira.findAll(TID('set-photo'))).toHaveLength(0)
    await kira.find(TID('set-toggle')).trigger('click')
    await settle()
    expect(PersonSetList).toHaveBeenCalledWith('Кира')
    const photos = cardOf(w, 'Кира').findAll(TID('set-photo'))
    expect(photos).toHaveLength(2)
    const remove = photos[1].find(TID('set-remove')).exists()
      ? photos[1].find(TID('set-remove'))
      : cardOf(w, 'Кира').findAll(TID('set-remove'))[1]
    expect(remove.exists()).toBe(true)
    await remove.trigger('click')
    await settle()
    expect(PersonSetRemove).toHaveBeenCalledWith(42)
  })

  it('6: «Лицо из набора» — ≥5 photos and LoRAs: calls AddPersonIdentityFromSet for each LoRA of that Имя', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([OLYA]))
    ListPeople.mockImplementation(() => Promise.resolve([
      { name: 'olya_z', lora: 'olya_z', base: 'Z-Image', display_name: 'Оля', steps: 2000 },
      { name: 'olya_x', lora: 'olya_x', base: 'SDXL', display_name: 'Оля', steps: 2000 },
      { name: 'other', lora: 'other', base: 'Z-Image', display_name: 'Другая', steps: 2000 },
    ]))
    const w = await mountPeople()
    const btn = cardOf(w, 'Оля').find(TID('set-face'))
    expect(btn.exists()).toBe(true)
    await btn.trigger('click')
    await settle()
    const got = AddPersonIdentityFromSet.mock.calls.map(c => c.join('|')).sort()
    expect(got).toEqual(['Оля|olya_x', 'Оля|olya_z'])
  })

  it('6: LoRAs match the set ignoring case and extra spaces', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([OLYA]))
    ListPeople.mockImplementation(() => Promise.resolve([{ name: 'olya_z', lora: 'olya_z', base: 'Z-Image', display_name: ' оля ', steps: 2000 }]))
    const w = await mountPeople()
    await cardOf(w, 'Оля').find(TID('set-face')).trigger('click')
    await settle()
    expect(AddPersonIdentityFromSet.mock.calls).toEqual([['Оля', 'olya_z']])
  })

  it('7: last set removed while «Из набора» is chosen — form returns to «Из папки»', async () => {
    let list = [KIRA]
    PersonSetSummary.mockImplementation(() => Promise.resolve(list))
    PersonSetList.mockImplementation(() => Promise.resolve([{ id: 7, source_name: 'a.jpg' }]))
    const w = await mountPeople()
    await cardOf(w, 'Кира').find(TID('set-train')).trigger('click')
    await settle()
    expect(w.find(TID('people-set-key')).exists()).toBe(true)
    list = []
    await cardOf(w, 'Кира').find(TID('set-toggle')).trigger('click')
    await settle()
    await w.find(TID('set-remove')).trigger('click')
    await settle()
    expect(w.find(TID('people-set-key')).exists()).toBe(false)
    expect(w.find(TID('people-folder')).exists()).toBe(true)
  })

  it('6: «Лицо из набора» hidden when the person has no LoRAs', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([OLYA]))
    ListPeople.mockImplementation(() => Promise.resolve([{ name: 'other', lora: 'other', base: 'Z-Image', display_name: 'Другая', steps: 2000 }]))
    const w = await mountPeople()
    expect(cardOf(w, 'Оля').find(TID('set-face')).exists()).toBe(false)
  })

  it('6: «Лицо из набора» hidden when the set has fewer than 5 photos', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([NADYA]))
    ListPeople.mockImplementation(() => Promise.resolve([{ name: 'nadya_z', lora: 'nadya_z', base: 'Z-Image', display_name: 'Надя', steps: 2000 }]))
    const w = await mountPeople()
    expect(cardOf(w, 'Надя').find(TID('set-face')).exists()).toBe(false)
  })

  it('6: «Лицо из набора» shown at exactly 5 photos', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([{ ...NADYA, count: 5 }]))
    ListPeople.mockImplementation(() => Promise.resolve([{ name: 'nadya_z', lora: 'nadya_z', base: 'Z-Image', display_name: 'Надя', steps: 2000 }]))
    const w = await mountPeople()
    expect(cardOf(w, 'Надя').find(TID('set-face')).exists()).toBe(true)
  })

  it('7: no sets — no source switch in the training form', async () => {
    const w = await mountPeople()
    expect(w.find(TID('people-source-set')).exists()).toBe(false)
    expect(w.find(TID('people-source-folder')).exists()).toBe(false)
  })

  it('7: sets exist — the form has «Из набора»/«Из папки»', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([KIRA]))
    const w = await mountPeople()
    expect(w.find(TID('people-source-set')).exists()).toBe(true)
    expect(w.find(TID('people-source-folder')).exists()).toBe(true)
  })

  it('7: «Обучить» on a set card switches to «Из набора» with that set; training calls TrainPersonFromSet without a folder', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([OLYA, KIRA]))
    const w = await mountPeople()
    await cardOf(w, 'Кира').find(TID('set-train')).trigger('click')
    await settle()

    const src = w.find(TID('people-source-set')).element
    if (src.tagName === 'INPUT') expect(src.checked).toBe(true)
    const setKey = w.find(TID('people-set-key'))
    expect(setKey.exists()).toBe(true)
    expect(setKey.element.value).toBe('Кира')

    await w.find(TID('people-name')).setValue('kira')
    await w.find(TID('people-gender')).setValue('woman')
    await w.find(TID('people-train')).trigger('click')
    await settle()

    expect(PickPeopleFolder).not.toHaveBeenCalled()
    expect(TrainPerson).not.toHaveBeenCalled()
    expect(TrainPersonFromSet).toHaveBeenCalledTimes(1)
    expect(TrainPersonFromSet).toHaveBeenCalledWith('Кира', 'kira', 'woman', 'Z-Image', 'quick', false, false)
  })

  it('7: hold checkbox is passed to TrainPersonFromSet', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([KIRA]))
    const w = await mountPeople()
    await cardOf(w, 'Кира').find(TID('set-train')).trigger('click')
    await settle()
    await w.find(TID('people-name')).setValue('kira')
    await w.find(TID('people-gender')).setValue('woman')
    await w.find(TID('people-hold')).setValue(true)
    await w.find(TID('people-train')).trigger('click')
    await settle()
    expect(TrainPersonFromSet).toHaveBeenCalledWith('Кира', 'kira', 'woman', 'Z-Image', 'quick', true, false)
  })

  it('7: switching back to «Из папки» trains from the folder as before', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([KIRA]))
    TrainPerson.mockImplementation(() => Promise.resolve({ id: 'j2', name: 'kira', base: 'Z-Image', status: 'queued', progress: { step: 0, steps: 2000 } }))
    const w = await mountPeople()
    await activate(w.find(TID('people-source-folder')).element)
    await w.findAll('button').find(b => b.text() === t('people.folder_pick')).trigger('click')
    await settle()
    await w.find(TID('people-name')).setValue('kira')
    await w.find(TID('people-gender')).setValue('woman')
    await w.find(TID('people-train')).trigger('click')
    await settle()
    expect(TrainPersonFromSet).not.toHaveBeenCalled()
    expect(TrainPerson).toHaveBeenCalledTimes(1)
    expect(TrainPerson.mock.calls[0][0]).toBe('/photos/kira')
  })
})
