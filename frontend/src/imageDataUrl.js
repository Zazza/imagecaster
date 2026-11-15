const SIGNATURES = [
  ['/9j/', 'image/jpeg'],
  ['iVBORw0KGgo', 'image/png'],
  ['UklGR', 'image/webp'],
  ['R0lGOD', 'image/gif'],
]

export function imageMime(b64) {
  for (const [prefix, mime] of SIGNATURES) {
    if (b64.startsWith(prefix)) return mime
  }
  return 'image/png'
}

export function b64Url(b64) {
  if (!b64) return ''
  if (b64.startsWith('data:') || b64.startsWith('/api/')) return b64
  return `data:${imageMime(b64)};base64,` + b64
}
