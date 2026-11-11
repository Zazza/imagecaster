import { t } from './i18n/index.js'

export function activeLoraCount(rows) {
  if (!Array.isArray(rows)) return 0
  return rows.filter(r => r && String(r.name || '').trim()).length
}

export function modelLoraText(model, rows) {
  const n = activeLoraCount(rows)
  const parts = []
  if (model) parts.push(model)
  parts.push(n > 0 ? t('model.lora_count', { n }) : t('model.no_lora'))
  return parts.join(' · ')
}
