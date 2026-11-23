import { GetModelCatalog, GetServerModels } from '../../wailsjs/go/main/App.js'

export const INSTALLED_EDITOR = 'flux-2-klein-4b.safetensors'
export function mockInstalledEditor() {
  GetModelCatalog.mockResolvedValue({ sd_models: [{ name: 'flux-2-klein-4b', filename: INSTALLED_EDITOR, modules: [] }] })
  GetServerModels.mockImplementation(async type => type === 'sd' ? [{ name: INSTALLED_EDITOR }] : [])
}
