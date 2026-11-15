<script setup>
import { ref, computed, onMounted, nextTick } from 'vue'
import { t } from '../../i18n/index.js'

const props = defineProps({
  logs: { type: Array, required: true },
})
const emit = defineEmits(['clear'])
const filterLevel = defineModel('filter', { type: String, default: 'all' })

const logContainer = ref(null)

const levels = [
  ['all', 'footer.filter_all'],
  ['error', 'footer.filter_errors'],
  ['warn', 'footer.filter_warnings'],
  ['info', 'footer.filter_info'],
  ['debug', 'footer.filter_debug'],
]

const filteredLogs = computed(() => {
  if (filterLevel.value === 'all') return props.logs
  return props.logs.filter(l => l.level === filterLevel.value)
})

function levelClass(level) {
  switch (level) {
    case 'error': return 'log-error'
    case 'warn': return 'log-warn'
    case 'info': return 'log-info'
    case 'debug': return 'log-debug'
    default: return ''
  }
}

function scrollToBottom() {
  nextTick(() => {
    if (logContainer.value) {
      logContainer.value.scrollTop = logContainer.value.scrollHeight
    }
  })
}

onMounted(scrollToBottom)

defineExpose({ scrollToBottom })
</script>

<template>
  <div class="footer-log">
    <div class="log-toolbar">
      <div class="log-filters">
        <button v-for="[level, key] in levels" :key="level" class="filter-btn" :class="{ active: filterLevel === level }" @click.stop="filterLevel = level">{{ t(key) }}</button>
      </div>
      <button class="filter-btn" @click.stop="emit('clear')">{{ t('footer.btn_clear_logs') }}</button>
    </div>
    <div class="log-entries" ref="logContainer">
      <div v-for="entry in filteredLogs" :key="entry.id" class="log-entry" :class="levelClass(entry.level)">
        <span class="log-time">{{ entry.timestamp }}</span>
        <span class="log-level">{{ entry.level.toUpperCase() }}</span>
        <span class="log-msg">{{ entry.message }}</span>
      </div>
      <div v-if="logs.length === 0" class="log-empty">{{ t('footer.no_logs') }}</div>
    </div>
  </div>
</template>

<style scoped>
.footer-log {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-top: 1px solid var(--border);
}
.log-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 12px;
  background: var(--surface-2);
  border-bottom: 1px solid var(--border);
}

.log-filters {
  display: flex;
  gap: 4px;
}

.filter-btn {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  padding: 2px 8px;
  border-radius: 3px;
  font-size: var(--fs-xs);
  cursor: pointer;
}

.filter-btn.active {
  background: var(--accent-bg);
  border-color: var(--accent);
  color: var(--text-bright);
}

.filter-btn:hover { border-color: var(--text-dim); }

.log-entries {
  flex: 1;
  overflow-y: auto;
  padding: 4px 0;
  font-family: 'Menlo', 'Consolas', 'Courier New', monospace;
  font-size: var(--fs-sm);
  line-height: 1.6;
}

.log-entry {
  display: flex;
  gap: 8px;
  padding: 2px 12px;
}

.log-entry:hover { background: var(--surface-2); }

.log-time { color: var(--text-dim); flex-shrink: 0; width: 60px; }
.log-level { flex-shrink: 0; width: 44px; font-weight: 600; font-size: var(--fs-xs); }

.log-error .log-level { color: var(--danger); }
.log-error { background: rgba(224, 85, 85, 0.06); }
.log-warn .log-level { color: var(--warning); }
.log-info .log-level { color: var(--accent); }
.log-debug .log-level { color: var(--text-dim); }
.log-debug { color: var(--text-dim); }

.log-msg { word-break: break-word; min-width: 0; }

.log-empty {
  text-align: center;
  color: var(--text-dim);
  padding: 20px;
  font-family: inherit;
  font-size: var(--fs-md);
}</style>
