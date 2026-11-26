<script setup>
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { b64Url } from '../imageDataUrl.js'
import { locale } from '../i18n/index.js'
import { RESOLUTION_LEVELS, resolutionDimensions } from '../resolutionUpscale.js'

const props = defineProps({ image: { type: String, default: '' }, itemId: { type: Number, default: 0 }, inline: Boolean, blocked: Boolean, batch: { type: Object, default: null } })
const emit = defineEmits(['completed', 'busy'])
const dimensions = ref(null)
const pending = ref(false)
const error = ref('')
let epoch = 0
let active = null
const off = []
const text = (ru, en) => locale.value === 'ru' ? ru : en
function setPending(value) { pending.value = value; emit('busy', value) }
function imageSize(b64) {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve({ width: image.naturalWidth, height: image.naturalHeight })
    image.onerror = () => reject(new Error(text('Не удалось прочитать размер', 'Could not read image size')))
    image.src = b64Url(b64)
  })
}
watch(() => [props.image, props.itemId], () => {
  const version = ++epoch
  dimensions.value = null
  setPending(false)
  error.value = ''
  if (!props.image) return
  const image = new Image()
  image.onload = () => {
    if (version === epoch) dimensions.value = { width: image.naturalWidth, height: image.naturalHeight }
  }
  image.onerror = () => { if (version === epoch) error.value = text('Не удалось прочитать размер', 'Could not read image size') }
  image.src = b64Url(props.image)
}, { immediate: true })
function targetSize(level) {
  return dimensions.value ? resolutionDimensions(dimensions.value.width, dimensions.value.height, level.long) : null
}
function disabled(level) {
  return props.blocked || pending.value || props.batch?.batchBusy.value || props.batch?.batchExporting.value || !dimensions.value || (!props.batch?.batchActive.value && Math.max(dimensions.value.width, dimensions.value.height) >= level.long)
}
async function reconcile(completedEvent) {
  const request = active
  if (!request?.id) return
  let completed = false
  try {
    const job = completedEvent?.job_id === request.id && completedEvent.result
      ? { status: 'completed', result: completedEvent.result }
      : await api.getQueueJobResult(request.id)
    if (active !== request) return
    if (!job || job.status === 'cancelled' || job.status === 'failed') {
      active = null
      if (request.epoch === epoch) {
        setPending(false)
        error.value = job?.error || text('Увеличение отменено', 'Upscale cancelled')
      }
    } else if (job.status === 'completed') {
      completed = true
      if (request.epoch === epoch) {
        const result = typeof job.result === 'string' ? JSON.parse(job.result) : job.result
        if (!result?.session_item_id) throw new Error(text('Нет результата увеличения', 'Upscale result is missing'))
        const image = result.image_base64 || await api.getSessionImage(result.session_item_id)
        if (!image) throw new Error(text('Нет изображения результата', 'Result image is missing'))
        if (active !== request) return
        active = null
        if (request.epoch === epoch) { setPending(false); emit('completed', { ...result, image_base64: image }) }
      } else {
        active = null
      }
    }
  } catch (e) {
    if (active !== request) return
    if (completed) active = null
    if (request.epoch === epoch) {
      if (completed) setPending(false)
      error.value = String(e)
    }
  }
}
function retryCheck() {
  error.value = ''
  return reconcile()
}
async function upscale(level) {
  if (disabled(level)) return
  if (props.batch?.batchActive.value) {
    setPending(true)
    error.value = ''
    try {
      await props.batch.runBatchStep(async b64 => {
        const size = await imageSize(b64)
        if (Math.max(size.width, size.height) >= level.long) return b64
        const job = await props.batch.trackBatchJob(api.enqueueImageResolutionUpscale(b64, level.long))
        return job ? () => job.wait : ''
      })
    } catch (e) {
      error.value = String(e)
    } finally { setPending(false) }
    return
  }
  const request = { epoch, itemId: props.itemId, id: null }
  active = request
  setPending(true)
  error.value = ''
  try {
    request.id = request.itemId
      ? await api.enqueueResolutionUpscale(request.itemId, level.long)
      : await api.enqueueImageResolutionUpscale(props.image, level.long)
    await reconcile()
  } catch (e) {
    if (active === request) active = null
    if (request.epoch === epoch) { setPending(false); error.value = String(e) }
  }
}
onMounted(() => {
  off.push(EventsOn('queue:changed', reconcile))
  off.push(EventsOn('queue:completed', reconcile))
  off.push(EventsOn('queue:failed', reconcile))
})
onUnmounted(() => { ++epoch; setPending(false); off.forEach(stop => stop()) })
</script>

<template>
  <div v-if="image" class="resolution-upscale" :class="{ 'resolution-inline': inline }" @pointerdown.stop @click.stop @wheel.stop>
    <span v-if="dimensions">{{ dimensions.width }}×{{ dimensions.height }}</span>
    <button data-testid="resolution-level" v-for="level in RESOLUTION_LEVELS" :key="level.long" class="btn btn-sm btn-secondary" :disabled="disabled(level)" :title="targetSize(level) ? `${targetSize(level).width}×${targetSize(level).height}` : level.label" @click="upscale(level)">
      {{ level.label }} <small v-if="targetSize(level)">{{ targetSize(level).width }}×{{ targetSize(level).height }}</small>
    </button>
    <span v-if="batch?.batchActive.value">{{ text('Для всех изображений', 'For all images') }}</span>
    <span v-if="pending" role="status">{{ text('Увеличение…', 'Upscaling…') }}</span>
    <span v-if="error" role="alert">{{ error }}</span>
    <button v-if="error && pending" class="btn btn-sm btn-secondary" @click="retryCheck">{{ text('Проверить снова', 'Check again') }}</button>
  </div>
</template>

<style scoped>
.resolution-upscale { position: absolute; left: 12px; bottom: 12px; z-index: 4; display: flex; flex-wrap: wrap; align-items: center; gap: 6px; max-width: calc(100% - 24px); padding: 6px; border-radius: 8px; background: var(--bg-panel, #242424); color: var(--text-primary, #fff); font-size: 12px; }
.resolution-inline { position: static; max-width: 100%; padding: 0; background: transparent; color: inherit; }
.resolution-inline > span:first-child { flex-basis: 100%; }
.resolution-inline > button { display: flex; flex: 1 1 0; min-width: 0; flex-direction: column; gap: 2px; }
.resolution-inline small { margin-left: 0; }
small { margin-left: 4px; opacity: .75; }
</style>
