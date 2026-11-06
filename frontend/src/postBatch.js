import { reactive } from 'vue'

export function createBatch(items, deps) {
  const batch = reactive({
    items: items.map(it => ({ name: it.name, path: it.path, versions: [it.path], failed: false })),
    steps: 0,
    get canUndo() {
      return this.steps > 0
    },
    currentPath(i) {
      const v = this.items[i]?.versions
      return v ? v[v.length - 1] : ''
    },
    async applyStep(op, opts = {}) {
      const isCancelled = opts.isCancelled || (() => false)
      const total = this.items.length
      const next = this.items.map((_, i) => this.currentPath(i))
      const pending = []
      let changed = 0
      let failed = 0
      let cancelled = false
      let done = 0

      const settle = async (i, res) => {
        let path = ''
        try {
          if (res) path = await deps.saveImage(res)
        } catch {
          path = ''
        }
        const item = this.items[i]
        if (path) {
          changed++
          item.failed = false
          next[i] = path
        } else if (isCancelled()) {
          cancelled = true
        } else {
          failed++
          item.failed = true
        }
        done++
        opts.onProgress?.(done, total)
      }

      for (let i = 0; i < total; i++) {
        if (cancelled || isCancelled()) {
          cancelled = true
          continue
        }
        let res = ''
        try {
          const b64 = await deps.readImage(this.currentPath(i))
          res = b64 ? await op(b64, i) : ''
        } catch {
          res = ''
        }
        if (typeof res === 'function') {
          const wait = res
          pending.push((async () => {
            let out = ''
            try {
              out = await wait()
            } catch {
              out = ''
            }
            await settle(i, out)
          })())
        } else {
          await settle(i, res)
        }
      }
      await Promise.all(pending)
      if (changed > 0) {
        this.items.forEach((item, i) => item.versions.push(next[i]))
        this.steps++
      }
      return { changed, failed, cancelled }
    },
    removeItem(i) {
      if (!Number.isInteger(i) || i < 0 || i >= this.items.length) return false
      this.items.splice(i, 1)
      return true
    },
    undo() {
      if (this.steps === 0) return false
      for (const item of this.items) {
        item.versions.pop()
        item.failed = false
      }
      this.steps--
      return true
    },
    async exportAll(fn, opts = {}) {
      const isCancelled = opts.isCancelled || (() => false)
      const total = this.items.length
      let saved = 0
      let skipped = 0
      for (let i = 0; i < total; i++) {
        if (isCancelled()) return { saved, skipped, cancelled: true }
        try {
          const b64 = await deps.readImage(this.currentPath(i))
          if (b64 && await fn(b64, this.items[i], i)) saved++
          else skipped++
        } catch {
          skipped++
        }
        opts.onProgress?.(i + 1, total)
      }
      return { saved, skipped, cancelled: false }
    },
  })
  return batch
}

export function batchExportSize({ width, height, lockRatio, previewWidth, previewHeight }) {
  if (width === previewWidth && height === previewHeight) return { width: 0, height: 0 }
  if (lockRatio) return { width: Math.max(width, height), height: 0 }
  return { width, height }
}

export function batchFileName(name, format) {
  const base = (name || '').replace(/\.[^.]+$/, '') || 'image'
  return `${base}.${format}`
}
