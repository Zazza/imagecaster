export const WELCOME_KEY = 'imagecaster_welcome'

export function welcomeTab(state, currentVersion) {
  const s = state || {}
  const fresh = !s.seenVersion
  const newVersion = !!currentVersion && !fresh && s.seenVersion !== currentVersion
  if (newVersion) return 'news'
  return s.dismissed ? null : 'about'
}

export function welcomeClosed(state, currentVersion, dontShow) {
  const s = state || {}
  return { dismissed: !!s.dismissed || !!dontShow, seenVersion: currentVersion || s.seenVersion || '' }
}

export function isFreshInstall(plan, sdModelCount) {
  if (sdModelCount !== 0 || !plan || !Array.isArray(plan.items)) return false
  return plan.items.some(i => i && i.type === 'sd')
}

export function startupTab(state, currentVersion, fresh) {
  const tab = welcomeTab(state, currentVersion)
  if (tab) return tab
  return fresh ? 'models' : null
}
