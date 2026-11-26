export const RESOLUTION_1080_LONG = 1920
export const RESOLUTION_2K_LONG = 2560
export const RESOLUTION_4K_LONG = 3840
export const RESOLUTION_LEVELS = [
  { label: '1080p', long: RESOLUTION_1080_LONG },
  { label: '2K', long: RESOLUTION_2K_LONG },
  { label: '4K', long: RESOLUTION_4K_LONG },
]

export function resolutionDimensions(width, height, target) {
  if (!(width > 0 && height > 0)) return null
  const scale = target / Math.max(width, height)
  return { width: Math.max(1, Math.round(width * scale)), height: Math.max(1, Math.round(height * scale)) }
}
