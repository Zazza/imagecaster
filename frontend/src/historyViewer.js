let opener = null

export function registerHistoryViewer(fn) {
  opener = fn
  return () => {
    if (opener === fn) opener = null
  }
}

export async function openInHistoryViewer(id) {
  if (!opener || !id) return false
  try {
    return !!(await opener(id))
  } catch (e) {
    console.warn('open history viewer:', e)
    return false
  }
}
