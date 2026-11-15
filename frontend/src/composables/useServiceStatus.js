import { reactive, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { t } from '../i18n/index.js'

export const SERVICE_CHECK_INTERVAL = 30000

export function serviceTitle(key, svc) {
  let main
  if (key === 'llm') {
    main = svc.model || svc.visionModel
      ? t('footer.status_models', { models: (svc.model || '') + (svc.visionModel ? ' / ' + svc.visionModel : '') })
      : t('footer.status_no_model')
  } else {
    main = svc.model || t('footer.status_no_model')
  }
  return main + '\n' + t('footer.status_click_settings')
}

export function useServiceStatus() {
  const services = reactive({
    llm: { available: false, label: 'footer.ai_assistant', model: '', visionModel: '', tab: 'connection' },
    sd: { available: false, label: 'footer.image_engine', model: '', tab: 'connection' },
  })
  let timer = null

  async function checkServices() {
    try {
      const settings = await api.getSettings()
      const status = await api.getServerStatus()
      const health = status.health || {}
      const models = status.models || {}
      services.llm.available = Boolean(health.llm?.healthy)
      const genModel = settings.llm_generate_model || settings.sd_prompt_model || settings.llm_model || ''
      services.llm.model = genModel && genModel !== 'default' ? genModel : ''
      const visModel = settings.llm_analyze_model || settings.vision_model || ''
      services.llm.visionModel = visModel && visModel !== 'default' ? visModel : ''
      services.sd.available = health.sd?.healthy || false
      services.sd.model = models.sd_checkpoint || ''
    } catch (e) {
      console.warn('AppFooter.checkServices:', e)
      services.llm.available = false
      services.sd.available = false
    }
  }

  onMounted(() => {
    checkServices()
    timer = setInterval(checkServices, SERVICE_CHECK_INTERVAL)
  })

  onUnmounted(() => {
    clearInterval(timer)
    timer = null
  })

  return { services, checkServices, serviceTitle }
}
