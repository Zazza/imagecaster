import * as ort from 'onnxruntime-web/wasm'
import wasmURL from 'onnxruntime-web/ort-wasm-simd-threaded.wasm?url'
import runtimeURL from 'onnxruntime-web/ort-wasm-simd-threaded.mjs?url'

const RGB_CHANNELS = 3
const RGBA_CHANNELS = 4
const LOW_RES_SIDE = 256
const MASK_CHANNELS = 1
const PAD_LABEL = -1
const MAX_HINTS = 64
const CPU_THREAD_COUNT = 1
const NO_PRIOR_MASK = 0
const MASK_LOGIT_THRESHOLD = 0

ort.env.wasm.numThreads = CPU_THREAD_COUNT
ort.env.wasm.proxy = false
ort.env.wasm.wasmPaths = { wasm: wasmURL, mjs: runtimeURL }
let encoder = null
let decoder = null
let embeddings = null
let encodedSource = ''

self.onmessage = async ({ data: request }) => {
  const { id, source, width, height, scaledWidth, scaledHeight, hints, models } = request
  try {
    encoder ||= await ort.InferenceSession.create(models.encoder_url, { executionProviders: ['wasm'] })
    decoder ||= await ort.InferenceSession.create(models.decoder_url, { executionProviders: ['wasm'] })
    if (encodedSource !== source) {
      embeddings?.dispose()
      embeddings = null
      const rgba = new Uint8ClampedArray(request.pixels)
      const rgb = new Float32Array(scaledWidth * scaledHeight * RGB_CHANNELS)
      for (let i = 0; i < scaledWidth * scaledHeight; i++) {
        for (let c = 0; c < RGB_CHANNELS; c++) rgb[i * RGB_CHANNELS + c] = rgba[i * RGBA_CHANNELS + c]
      }
      const input = new ort.Tensor('float32', rgb, [scaledHeight, scaledWidth, RGB_CHANNELS])
      const encoded = await encoder.run({ input_image: input })
      input.dispose()
      embeddings = encoded.image_embeddings
      encodedSource = source
    }
    const points = hints.slice(-MAX_HINTS)
    const coords = new Float32Array((points.length + 1) * 2)
    const labels = new Float32Array(points.length + 1)
    points.forEach((point, i) => {
      coords[i * 2] = Math.max(0, Math.min(width - 1, point.x)) * scaledWidth / width
      coords[i * 2 + 1] = Math.max(0, Math.min(height - 1, point.y)) * scaledHeight / height
      labels[i] = point.label
    })
    labels[points.length] = PAD_LABEL
    const inputs = {
      point_coords: new ort.Tensor('float32', coords, [1, points.length + 1, 2]),
      point_labels: new ort.Tensor('float32', labels, [1, points.length + 1]),
      mask_input: new ort.Tensor('float32', new Float32Array(LOW_RES_SIDE * LOW_RES_SIDE), [1, MASK_CHANNELS, LOW_RES_SIDE, LOW_RES_SIDE]),
      has_mask_input: new ort.Tensor('float32', new Float32Array([NO_PRIOR_MASK]), [1]),
      orig_im_size: new ort.Tensor('float32', new Float32Array([height, width]), [2]),
    }
    const outputs = await decoder.run({ image_embeddings: embeddings, ...inputs })
    const mask = new Uint8Array(width * height)
    for (let i = 0; i < mask.length; i++) mask[i] = outputs.masks.data[i] > MASK_LOGIT_THRESHOLD ? 1 : 0
    Object.values(inputs).forEach(tensor => tensor.dispose())
    Object.values(outputs).forEach(tensor => tensor.dispose())
    self.postMessage({ id, width, height, data: mask }, [mask.buffer])
  } catch (e) {
    self.postMessage({ id, error: String(e.message || e) })
  }
}
