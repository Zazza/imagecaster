export const IDEAS_BATCH_MAX_PER_IDEA = 20

export function clampPerIdea(value) {
  const n = Math.trunc(Number(value))
  if (!Number.isFinite(n) || n < 1) return 1
  return Math.min(IDEAS_BATCH_MAX_PER_IDEA, n)
}

export async function runIdeasBatch({ ideas, perIdea, generatePrompt, enqueue, isStopped = () => false, onProgress = () => {} }) {
  const count = clampPerIdea(perIdea)
  const list = ideas || []
  const jobIds = []
  const errors = []
  let done = 0
  for (const idea of list) {
    if (isStopped()) return { jobIds, errors, stopped: true }
    try {
      const res = await generatePrompt(idea)
      const prompt = res?.prompt || ''
      if (!prompt.trim()) throw new Error('empty prompt')
      const negative = res?.negative_prompt || ''
      for (let index = 0; index < count; index++) {
        if (isStopped()) return { jobIds, errors, stopped: true }
        const jobId = await enqueue({ prompt, negative_prompt: negative, idea, index })
        if (jobId) jobIds.push(jobId)
      }
    } catch (e) {
      errors.push({ idea, error: e?.message || String(e) })
    }
    done++
    onProgress({ done, total: list.length, idea })
  }
  return { jobIds, errors, stopped: false }
}
