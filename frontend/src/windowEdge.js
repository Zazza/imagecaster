const TOP_FIX = { 'n-resize': undefined, 'nw-resize': 'w-resize', 'ne-resize': 'e-resize' }

export function frameEdge(edge, maximised) {
  if (!edge || maximised) return undefined
  return edge in TOP_FIX ? TOP_FIX[edge] : edge
}
