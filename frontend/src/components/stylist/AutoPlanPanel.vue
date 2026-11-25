<script setup>
import { computed } from 'vue'
import { locale, t } from '../../i18n/index.js'

const props = defineProps({
  proposal: { type: Object, required: true },
  targetName: { type: String, default: '' },
  garment: { type: Boolean, default: false },
  garmentDescription: { type: String, default: '' },
  current: { type: Boolean, default: false },
  busy: { type: Boolean, default: false },
})
defineEmits(['apply', 'retry'])
const targetKind = computed(() => ['base', 'preset', 'pipe'].find(kind => props.proposal.target?.startsWith(kind + ':')))
const cleanText = value => typeof value === 'string' && !/^\s*(auto|default outfit|default background|default hair|default makeup)\s*$/i.test(value) && !/\{\{|\}\}|\$\{/.test(value) ? value : ''
const text = (item, key) => cleanText(locale.value === 'ru' ? item[key + '_ru'] || item[key] : item[key])
const aspectText = (series, key) => key === 'outfit' && props.garment
  ? [t('stylist.auto_photo_outfit'), cleanText(props.garmentDescription)].filter(Boolean).join(' · ')
  : text(series, key) || t('stylist.auto_detail_pending')
const shotText = shot => (locale.value === 'ru' ? cleanText(shot.prompt_ru) : '') || cleanText(shot.prompt) || cleanText(shot.pose) || t('stylist.auto_detail_pending')
const framingText = shot => ['close', 'waist', 'knee', 'full'].includes(shot.framing) ? t('stylist.framing_' + shot.framing) : ''
</script>

<template>
  <section class="card proposal" data-testid="stylist-auto-proposal">
    <header class="proposal-header">
      <span class="proposal-eyebrow">{{ t('stylist.auto_proposal') }}</span>
      <h3>{{ text(proposal, 'concept') || t('stylist.auto_concept_fallback') }}</h3>
      <dl class="proposal-summary">
        <div><dt>{{ t('stylist.auto_light') }}</dt><dd>{{ text(proposal, 'light') || t('stylist.auto_detail_pending') }}</dd></div>
        <div><dt>{{ t('stylist.auto_focus') }}</dt><dd>{{ text(proposal, 'focus') || t('stylist.auto_detail_pending') }}</dd></div>
        <div><dt>{{ t(targetKind ? 'stylist.kind_' + targetKind : 'stylist.auto_target') }}</dt><dd>{{ proposal.target_name || targetName || proposal.target }}</dd></div>
        <div v-if="proposal.target_model"><dt>{{ t('stylist.auto_model') }}</dt><dd>{{ proposal.target_model }}</dd></div>
        <div v-if="text(proposal, 'target_reason')"><dt>{{ t('stylist.auto_reason') }}</dt><dd>{{ text(proposal, 'target_reason') }}</dd></div>
      </dl>
    </header>
    <div class="proposal-series-grid">
      <article v-for="(s, i) in proposal.series" :key="i" class="proposal-series" data-testid="stylist-auto-series">
        <h4>{{ t('stylist.series_title', { n: i + 1 }) }}</h4>
        <dl class="proposal-aspects">
          <div v-for="aspect in ['outfit', 'background', 'hair', 'makeup']" :key="aspect"><dt>{{ t('stylist.tab_' + aspect) }}</dt><dd>{{ aspectText(s, aspect) }}</dd></div>
          <div class="proposal-direction"><dt>{{ t('stylist.auto_direction') }}</dt><dd>{{ text(s, 'direction') || t('stylist.auto_detail_pending') }}</dd></div>
        </dl>
        <ol class="proposal-shots">
          <li v-for="(shot, j) in s.shots" :key="j"><span v-if="framingText(shot)" class="proposal-framing">{{ framingText(shot) }}</span><p>{{ shotText(shot) }}</p></li>
        </ol>
      </article>
    </div>
    <footer class="proposal-footer">
      <p v-for="(warning, i) in proposal.warnings" :key="i" class="proposal-warning" role="status">{{ warning }}</p>
      <p class="form-hint" :class="{ 'proposal-warning': !current }">{{ current ? t('stylist.auto_apply_hint') : t('stylist.auto_stale') }}</p>
      <div class="proposal-actions">
        <button class="btn btn-primary" :disabled="busy || !current" data-testid="stylist-auto-apply" @click="$emit('apply')">{{ t('stylist.auto_apply') }}</button>
        <button class="btn btn-secondary" :disabled="busy" data-testid="stylist-auto-retry" @click="$emit('retry')">{{ t('stylist.auto_retry') }}</button>
      </div>
    </footer>
  </section>
</template>

<style scoped>
.proposal { flex: 0 0 auto; padding: 0; overflow: hidden; }
.proposal-header, .proposal-footer { padding: 10px 12px; }
.proposal-header { background: var(--surface-2); border-bottom: 1px solid var(--border); }
.proposal-eyebrow { color: var(--text-dim); font-size: var(--fs-xs); }
h3 { margin: 4px 0 8px; font-size: 1rem; line-height: 1.4; font-weight: 600; }
h4 { margin: 0 0 8px; font-size: var(--fs-base); }
dl, dd { margin: 0; }
dt { color: var(--text-dim); font-size: var(--fs-xs); }
dd, .proposal-shots p { line-height: 1.45; overflow-wrap: anywhere; min-width: 0; }
.proposal-summary { display: flex; flex-wrap: wrap; gap: 6px 18px; }
.proposal-summary > div { display: flex; align-items: baseline; gap: 6px; min-width: 0; }
.proposal-summary dt { flex-shrink: 0; }
.proposal-series-grid { padding: 10px 12px; display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 340px), 1fr)); gap: 10px; }
.proposal-series { padding: 10px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface-1); min-width: 0; }
.proposal-aspects { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 240px), 1fr)); gap: 6px 12px; }
.proposal-aspects > div { display: grid; grid-template-columns: 70px minmax(0, 1fr); align-items: baseline; gap: 6px; }
.proposal-aspects > .proposal-direction { grid-column: 1 / -1; }
.proposal-shots { margin: 10px 0 0; padding-left: 20px; border-top: 1px solid var(--border); }
.proposal-shots li { padding: 6px 0 0 3px; line-height: 1.45; overflow-wrap: anywhere; }
.proposal-shots li::marker { color: var(--text-dim); font-weight: 600; }
.proposal-framing { font-size: var(--fs-xs); color: var(--text-dim); margin-right: 8px; }
.proposal-shots p { display: inline; margin: 0; }
.proposal-footer { border-top: 1px solid var(--border); display: grid; gap: 8px; }
.proposal-footer p { margin: 0; }
.proposal-warning { padding: 6px 8px; border-left: 3px solid var(--warning, var(--text-dim)); background: var(--surface-2); border-radius: var(--radius-sm); line-height: 1.45; }
.proposal-actions { display: flex; flex-wrap: wrap; gap: 8px; }
@media (max-width: 600px) { .proposal-summary > div { flex: 1 1 100%; } }
</style>
