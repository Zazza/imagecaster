import { vi } from 'vitest'

const actual = await vi.importActual('../../wailsjs/go/main/App.js')

export const appDefaults = {
  ListPresets: () => [],
  GetDefaultEditGarmentPrompt: () => '',
  GetSettings: () => ({}),
  ListDescriptions: () => [],
  ListPresetTypes: () => [],
  GetDefaultPromptInstructionProse: () => '',
  ListCompoundPresets: () => [],
  ListExportPresets: () => [],
  ListPostRecipes: () => [],
  AutoAdjust: () => ({ black_point: 0, white_point: 255, gamma: 1, temperature: 0 }),
  CompilePostStyle: () => ({ steps: [], adjust: {} }),
  DescribeForStyle: () => '',
  BatchSaveImage: () => '',
  EnqueueLook: () => 0,
  ApplyLookDirect: () => '',
  ListSessions: () => [],
  GetSessionItems: () => [],
  GetActiveSessionItem: () => null,
  GetLLMSetup: () => ({ needed: false, generate: [], vision: [], generate_selected: '', vision_selected: '' }),
  ListResolutions: () => [],
  ListHiresProfiles: () => [],
  GetQueue: () => [],
  GetLogEntries: () => [],
  GetModelPresets: () => ({}),
  GetServerProcessLogs: () => [],
  SyncPresets: () => ({ statuses: [], added: 0 }),
  GetSDModels: () => [],
  GetSDLoRAs: () => [],
  DiscoverServers: () => [],
  CheckServer: () => ({ state: 'ok', url: 'http://s:1', version: '2', missing: [], error: '' }),
  ListPeople: () => [],
  ListTrainingJobs: () => [],
  TrainingStatus: () => 'not_installed',
  TrainingBases: () => ['Z-Image'],
}

export const appExportNames = Object.keys(actual).filter(name => typeof actual[name] === 'function')

for (const name of Object.keys(appDefaults)) {
  if (!appExportNames.includes(name)) throw new Error(`appDefaults: "${name}" is not exported by App.js`)
}

export function createAppMock(overrides = {}) {
  const mock = {}
  for (const name of appExportNames) {
    const value = appDefaults[name]
    mock[name] = vi.fn(() => Promise.resolve(value ? value() : undefined))
  }
  for (const [name, fn] of Object.entries(overrides)) {
    if (!appExportNames.includes(name)) throw new Error(`createAppMock: "${name}" is not exported by App.js`)
    mock[name] = fn
  }
  return mock
}

export default createAppMock()
