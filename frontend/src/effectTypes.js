export const EFFECT_TYPE_GROUPS = [
  { type: 'post', labelKey: 'post.group_effect_base' },
  { type: 'post_material', labelKey: 'post.group_effect_material' },
  { type: 'post_skin', labelKey: 'post.group_effect_skin' },
  { type: 'post_light', labelKey: 'post.group_effect_light' },
  { type: 'post_film', labelKey: 'post.group_effect_film' },
  { type: 'post_transform', labelKey: 'post.group_effect_transform' },
]

export function isEffectType(name) {
  return name === 'post' || (typeof name === 'string' && name.startsWith('post_'))
}

export function effectTypeLabel(name, t) {
  const g = EFFECT_TYPE_GROUPS.find(g => g.type === name)
  if (!g) return name
  return t('presets.effect_prefix') + ': ' + t(g.labelKey)
}
