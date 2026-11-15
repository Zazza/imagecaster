import { MAX_IMAGE_SIZE } from './constants.js'
import { t } from './i18n/index.js'

export function readImageFile(file) {
  return new Promise((resolve, reject) => {
    if (!file || !file.type?.startsWith('image/')) {
      reject(new Error(t('fi.error_drop_image')))
      return
    }
    if (file.size > MAX_IMAGE_SIZE) {
      reject(new Error(t('fi.error_image_too_large')))
      return
    }
    const reader = new FileReader()
    reader.onload = () => {
      const base64 = String(reader.result || '').split(',')[1] || ''
      if (base64) resolve(base64)
      else reject(new Error(t('fi.error_drop_image')))
    }
    reader.onerror = () => reject(reader.error || new Error(t('fi.error_drop_image')))
    reader.readAsDataURL(file)
  })
}
