import { flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'

export const SETTLE_ROUNDS = 8

export async function settle(rounds = SETTLE_ROUNDS) {
  for (let i = 0; i < rounds; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

export async function waitUntil(cond, { timeout = 2000, interval = 10, message = '' } = {}) {
  const deadline = performance.now() + timeout
  let lastError = null
  for (;;) {
    await settle(1)
    try {
      const value = await cond()
      if (value) return value
      lastError = null
    } catch (e) {
      lastError = e
    }
    if (performance.now() > deadline) {
      if (lastError) throw lastError
      throw new Error(`waitUntil: condition not met within ${timeout}ms${message ? ': ' + message : ''}`)
    }
    await new Promise(r => setTimeout(r, interval))
  }
}
