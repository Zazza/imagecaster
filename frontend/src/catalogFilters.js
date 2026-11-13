const MODEL_EXTS = ['.safetensors', '.gguf', '.ckpt']

export function modelStem(name) {
  let base = String(name || '').trim().split(/[\\/]/).pop()
  const lower = base.toLowerCase()
  for (const ext of MODEL_EXTS) {
    if (lower.endsWith(ext)) {
      base = base.slice(0, -ext.length)
      break
    }
  }
  return base.toLowerCase()
}

export function mainFilename(m) {
  return m.filename || m.name + '.safetensors'
}

function variantsOf(m) {
  return Array.isArray(m?.variants) ? m.variants : []
}

export function defaultVariant(m) {
  const vs = variantsOf(m)
  return vs.find(v => v.default) || vs[0] || null
}

export function fitVariant(m, vramMB) {
  const vs = variantsOf(m)
  if (!vs.length) return null
  if (!(vramMB > 0)) return defaultVariant(m)
  let best = null
  let smallest = null
  for (const v of vs) {
    if (!smallest || v.size_gb < smallest.size_gb) smallest = v
    if (v.vram_gb * 1024 <= vramMB && (!best || v.size_gb > best.size_gb)) best = v
  }
  return best || smallest
}

export function variantFits(v, vramMB) {
  return !(vramMB > 0) || v.vram_gb * 1024 <= vramMB
}

export function installedFile(m, sdFiles) {
  const stem = modelStem(mainFilename(m))
  return (sdFiles || []).find(f => modelStem(f.name) === stem) || null
}

export function installedVariant(m, sdFiles) {
  const file = installedFile(m, sdFiles)
  if (!file) return { present: false, variant: null }
  const variant = variantsOf(m).find(v => {
    const want = v.size_gb * 1e9
    return want > 0 && Math.abs((file.size || 0) - want) <= want * 0.03
  }) || null
  return { present: true, variant }
}

export function variantSwitchSteps(m, variant, files) {
  const steps = []
  for (const mod of m.modules || []) {
    if (!(files?.[mod.type] || []).some(f => f.name === mod.filename)) {
      steps.push({ op: 'download', type: mod.type, filename: mod.filename, url: mod.url })
    }
  }
  const download = { op: 'download', type: 'sd', filename: variant.save_as, url: variant.url }
  const old = installedFile(m, files?.sd)
  if (!old) {
    steps.push(download)
  } else if (old.name.toLowerCase() === String(variant.save_as).toLowerCase()) {
    steps.push({ op: 'delete', type: 'sd', filename: old.name }, download)
  } else {
    steps.push(download, { op: 'delete', type: 'sd', filename: old.name })
  }
  return steps
}

export function estimateVRAM(m, sizeGB) {
  let peak = sizeGB || 0
  let vae = 0
  for (const mod of m.modules || []) {
    const name = String(mod.filename || '').toLowerCase()
    if (name.includes('mmproj') || name.endsWith('.json')) continue
    if (mod.type === 'vae') vae += mod.size_gb || 0
    else peak = Math.max(peak, mod.size_gb || 0)
  }
  return Math.round((peak + vae + 1.5) * 10) / 10
}

function minVRAM(m) {
  const vs = variantsOf(m)
  if (!vs.length) return estimateVRAM(m, m.size_gb)
  return Math.min(...vs.map(v => v.vram_gb))
}

export function filterSDModels(models, filters, ctx) {
  const q = String(filters?.query || '').trim().toLowerCase()
  const vramMB = ctx?.vramMB || 0
  return (models || []).filter(m => {
    if (q && ![m.name, m.description, m.description_ru, m.base].some(s => String(s || '').toLowerCase().includes(q))) return false
    if (filters?.base && m.base !== filters.base) return false
    if (filters?.installed === 'yes' && !ctx.isInstalled(m)) return false
    if (filters?.installed === 'no' && ctx.isInstalled(m)) return false
    if (filters?.fits && vramMB > 0 && minVRAM(m) * 1024 > vramMB) return false
    return true
  })
}

export function modelBases(models) {
  return [...new Set((models || []).map(m => m.base).filter(Boolean))].sort()
}
