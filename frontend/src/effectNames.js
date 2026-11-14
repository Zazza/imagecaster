import { t as translate } from './i18n/index.js'

function splitVariant(name) {
  const s = String(name || '')
  const m = s.match(/^(.*?)\s*(\([^()]+\))$/)
  return m ? { base: m[1], variant: m[2] } : { base: s, variant: '' }
}

export function effectNameKey(name) {
  const { base } = splitVariant(name)
  return 'post.effect_' + base.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '')
}

export function effectLabel(name, t = translate) {
  const { variant } = splitVariant(name)
  const k = effectNameKey(name)
  const v = t(k)
  if (v === k) return name
  return variant ? `${v} ${variant}` : v
}
