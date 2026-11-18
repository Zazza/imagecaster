import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks, EventsEmit } from './mocks/runtime'

import AppFooter from '../components/AppFooter.vue'
import {
  ListSessions, SwitchSession, GetActiveSessionItem, GetSessionItems, MoveSessionItems,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const MOVE_LABEL = /Перенести в|Move to/

const SESSIONS = [
  { id: 11, name: 'Первая', created_at: '', updated_at: '' },
  { id: 22, name: 'Вторая', created_at: '', updated_at: '' },
  { id: 33, name: 'Третья', created_at: '', updated_at: '' },
]

let activeSession
let items

function freshItems() {
  return {
    11: [{ id: 201, source: 'generate', created_at: '2026-11-06T09:00:00Z', is_active: false }],
    22: [
      { id: 101, source: 'generate', created_at: '2026-11-06T10:00:00Z', is_active: false },
      { id: 102, source: 'generate', created_at: '2026-11-06T10:01:00Z', is_active: false },
      { id: 103, source: 'generate', created_at: '2026-11-06T10:02:00Z', is_active: true },
    ],
    33: [],
  }
}

function backendMove(ids, sessionID) {
  const target = Number(sessionID)
  let n = 0
  for (const sid of Object.keys(items)) {
    if (Number(sid) === target) continue
    const keep = []
    for (const it of items[sid]) {
      if (ids.includes(it.id)) {
        items[target].push({ ...it, is_active: false })
        n++
      } else {
        keep.push(it)
      }
    }
    items[sid] = keep
  }
  EventsEmit('session:removed', { ids, moved_to: target })
  return Promise.resolve(n)
}

function buttonExact(wrapper, text) {
  return wrapper.findAll('button').find(b => b.text().trim() === text)
}

function moveButton(wrapper) {
  return wrapper.findAll('button').find(b => MOVE_LABEL.test(b.text()))
}

function cards(wrapper) {
  return wrapper.findAll('.si-card')
}

function cardIds(wrapper) {
  return cards(wrapper).map(c => Number(c.attributes('data-item-id')))
}

function inSwitcher(el) {
  return !!el.element.closest('select.session-select')
}

function folderEntries(wrapper, name) {
  const all = wrapper
    .findAll('button, option, li, a, [role="menuitem"], [role="option"]')
    .filter(el => !inSwitcher(el) && el.text().includes(name) && !MOVE_LABEL.test(el.text()))
  return all.filter(el => !all.some(other => other !== el && el.element.contains(other.element)))
}

async function chooseFolder(wrapper, name) {
  const entries = folderEntries(wrapper, name)
  expect(entries.length, `folder "${name}" in move list`).toBeGreaterThan(0)
  const el = entries[0]
  if (el.element.tagName === 'OPTION') {
    const owner = wrapper.findAll('select').find(sel => sel.element === el.element.closest('select'))
    await owner.setValue(el.element.value)
  } else {
    await el.trigger('click')
  }
  await settle()
}

async function mountOpen() {
  const wrapper = mount(AppFooter)
  await settle()
  const label = t('footer.session')
  const btn = wrapper.findAll('button').find(b => b.text().startsWith(label))
  expect(btn, `button "${label}"`).toBeTruthy()
  await btn.trigger('click')
  await settle()
  return wrapper
}

async function enterSelect(wrapper) {
  const btn = buttonExact(wrapper, t('footer.select_mode'))
  expect(btn, 'select mode button').toBeTruthy()
  await btn.trigger('click')
  await settle()
}

async function selectCards(wrapper, indexes) {
  for (const i of indexes) {
    await cards(wrapper)[i].trigger('click')
    await settle()
  }
}

async function openMoveList(wrapper) {
  const btn = moveButton(wrapper)
  expect(btn, 'move button').toBeTruthy()
  expect(btn.element.disabled).toBe(false)
  await btn.trigger('click')
  await settle()
}

describe('SessionPanel move selected photos to another folder', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearEventMocks()
    useRemixQueue().clearPostQueue()
    activeSession = 22
    items = freshItems()
    ListSessions.mockImplementation(() => Promise.resolve(SESSIONS.map(s => ({
      ...s, item_count: (items[s.id] || []).length, is_active: s.id === activeSession,
    }))))
    SwitchSession.mockImplementation(id => {
      activeSession = Number(id)
      return Promise.resolve()
    })
    GetActiveSessionItem.mockImplementation(() => Promise.resolve((items[activeSession] || []).find(i => i.is_active) || null))
    GetSessionItems.mockImplementation(() => Promise.resolve((items[activeSession] || []).map(i => ({ ...i }))))
    MoveSessionItems.mockImplementation(backendMove)
  })

  it('move button is not shown outside select mode', async () => {
    const wrapper = await mountOpen()
    expect(cards(wrapper)).toHaveLength(3)
    expect(moveButton(wrapper)).toBeFalsy()
  })

  it('move button is shown in select mode and disabled while nothing is selected', async () => {
    const wrapper = await mountOpen()
    await enterSelect(wrapper)
    const btn = moveButton(wrapper)
    expect(btn, 'move button').toBeTruthy()
    expect(btn.element.disabled).toBe(true)
  })

  it('move button shows the number of selected photos', async () => {
    const wrapper = await mountOpen()
    await enterSelect(wrapper)
    await selectCards(wrapper, [0, 2])
    const btn = moveButton(wrapper)
    expect(btn.text()).toContain('(2)')
    expect(btn.element.disabled).toBe(false)
  })

  it('folder list offers the other folders and not the current one', async () => {
    const wrapper = await mountOpen()
    await enterSelect(wrapper)
    await selectCards(wrapper, [0])
    await openMoveList(wrapper)
    expect(folderEntries(wrapper, 'Первая').length).toBeGreaterThan(0)
    expect(folderEntries(wrapper, 'Третья').length).toBeGreaterThan(0)
    expect(folderEntries(wrapper, 'Вторая')).toHaveLength(0)
  })

  it.each([
    { folder: 'Третья', folderId: 33, picks: [0, 2] },
    { folder: 'Первая', folderId: 11, picks: [1] },
  ])('choosing $folder moves the selected photos there and leaves select mode', async ({ folder, folderId, picks }) => {
    const wrapper = await mountOpen()
    await enterSelect(wrapper)
    const ids = cardIds(wrapper)
    const chosen = picks.map(i => ids[i])
    await selectCards(wrapper, picks)
    await openMoveList(wrapper)
    await chooseFolder(wrapper, folder)

    expect(MoveSessionItems).toHaveBeenCalledTimes(1)
    const [sentIds, sentFolder] = MoveSessionItems.mock.calls[0]
    expect([...sentIds].map(Number).sort()).toEqual([...chosen].sort())
    expect(Number(sentFolder)).toBe(folderId)

    expect(wrapper.findAll('.si-card.si-selected')).toHaveLength(0)
    expect(moveButton(wrapper)).toBeFalsy()
    expect(buttonExact(wrapper, t('footer.select_mode'))).toBeTruthy()
  })

  it('moved photos disappear from the current folder, the rest stay', async () => {
    const wrapper = await mountOpen()
    await enterSelect(wrapper)
    const ids = cardIds(wrapper)
    await selectCards(wrapper, [0, 2])
    await openMoveList(wrapper)
    await chooseFolder(wrapper, 'Третья')

    expect(cardIds(wrapper)).toEqual([ids[1]])
    expect(activeSession).toBe(22)
  })

  it('a failed move shows the error and keeps the photos', async () => {
    MoveSessionItems.mockImplementation(() => Promise.reject(new Error('disk full')))
    const wrapper = await mountOpen()
    await enterSelect(wrapper)
    await selectCards(wrapper, [0])
    await openMoveList(wrapper)
    await chooseFolder(wrapper, 'Первая')

    expect(MoveSessionItems).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('disk full')
    expect(cardIds(wrapper)).toEqual([101, 102, 103])
  })

  it('a partly failed move keeps the error visible and drops moved photos from the selection', async () => {
    MoveSessionItems.mockImplementation((ids, sessionID) => {
      const target = Number(sessionID)
      const first = items[22].find(i => i.id === ids[0])
      items[22] = items[22].filter(i => i.id !== first.id)
      items[target].push({ ...first, is_active: false })
      setTimeout(() => EventsEmit('session:removed', { ids: [first.id], moved_to: target }), 0)
      return Promise.reject(new Error('перенесено 1 из 2'))
    })
    const wrapper = await mountOpen()
    await enterSelect(wrapper)
    await selectCards(wrapper, [0, 1])
    await openMoveList(wrapper)
    await chooseFolder(wrapper, 'Третья')
    await new Promise(r => setTimeout(r, 10))
    await settle()

    expect(wrapper.text()).toContain('перенесено 1 из 2')
    expect(cardIds(wrapper)).toEqual([102, 103])
    expect(wrapper.findAll('.si-card.si-selected')).toHaveLength(1)
  })
})
