const HIDDEN = {
  service: ['ollama'],
  utility: ['ollama', 'python'],
}

export function visibleProcesses(processes, kind) {
  const hidden = HIDDEN[kind] || []
  return Object.entries(processes || {}).filter(([name, proc]) =>
    proc && proc.name && (proc.category === 'utility') === (kind === 'utility') && !hidden.includes(name))
}
