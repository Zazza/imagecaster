import { api } from './api.js'

let sink = null

export function setGenerateDescriptionSink(fn) {
  sink = fn
  return () => { if (sink === fn) sink = null }
}

export async function describeToGenerate(text, navigate) {
  if (sink) sink(text || '')
  else await api.updateSettings({ gen_description: text || '' })
  if (navigate) navigate({ page: 'generate' })
}

const SESSION_IMAGE = /^\/api\/img\/(\d+)\.jpg$/

export function describeSource(image) {
  const m = SESSION_IMAGE.exec(image || '')
  return m ? api.getSessionImage(Number(m[1])) : Promise.resolve(image || '')
}

const NOT_LOCAL = 'фото описывает только локальная модель ИИ'

export function describeErrorText(e, t) {
  const msg = String(e?.message || e)
  return msg.includes(NOT_LOCAL) ? t('describe.not_local') : msg
}
