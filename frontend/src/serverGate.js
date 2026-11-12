let ready = () => true

export function setServerGate(fn) {
  ready = fn
}

export function serverGate() {
  return ready()
}
