export function hiddenPresetIds(statuses) {
  const ids = new Set()
  for (const s of statuses || []) {
    if (s && s.installed === false) ids.add(s.id)
  }
  return ids
}

export function filterAvailable(presets, statuses, showAll = false) {
  if (showAll || !statuses) return [...presets]
  const hidden = hiddenPresetIds(statuses)
  return presets.filter(p => !hidden.has(p.id))
}
