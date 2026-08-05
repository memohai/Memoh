// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h } from 'vue'
import type { ProviderPreset } from '@/constants/provider-presets'

const mocks = vi.hoisted(() => ({
  deleteModel: vi.fn(),
  getProviderModels: vi.fn(),
  getProviderTemplates: vi.fn(),
  getProviderByName: vi.fn(),
  importModels: vi.fn(),
  postProvider: vi.fn(),
  postProviderFromTemplate: vi.fn(),
  testProvider: vi.fn(),
  updateModel: vi.fn(),
  updateProvider: vi.fn(),
  invalidateQueries: vi.fn(),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('@pinia/colada', async () => {
  const { ref } = await import('vue')
  return {
    useMutation: (options: { mutation: (value: never) => Promise<unknown> }) => ({
      mutateAsync: options.mutation,
      isLoading: ref(false),
    }),
    useQuery: (options: { query: () => Promise<unknown> }) => {
      const state = ref({ data: [] })
      return {
        state,
        refresh: async () => {
          state.value.data = await options.query() as never[]
        },
      }
    },
    useQueryCache: () => ({ invalidateQueries: mocks.invalidateQueries }),
  }
})

vi.mock('@memohai/sdk', () => ({
  deleteModelsById: mocks.deleteModel,
  getProviderTemplates: mocks.getProviderTemplates,
  getProvidersByIdModels: mocks.getProviderModels,
  getProvidersNameByName: mocks.getProviderByName,
  postProviders: mocks.postProvider,
  postProvidersByIdImportModels: mocks.importModels,
  postProvidersByIdTest: mocks.testProvider,
  postProvidersFromTemplate: mocks.postProviderFromTemplate,
  putModelsById: mocks.updateModel,
  putProvidersById: mocks.updateProvider,
}))

import { useProviderSetup } from './useProviderSetup'

function mountSetup(preset: ProviderPreset | null) {
  let setup: ReturnType<typeof useProviderSetup> | undefined
  const ready = vi.fn()
  const app = createApp({
    setup() {
      setup = useProviderSetup({ selectedPreset: () => preset, onProviderReady: ready })
      return () => h('div')
    },
  })
  app.mount(document.createElement('div'))
  return { app, ready, setup: setup! }
}

describe('useProviderSetup', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.testProvider.mockResolvedValue({ data: { status: 'ok' } })
    mocks.importModels.mockResolvedValue({
      data: { created: 2, updated: 0, skipped: 0, models: ['deepseek-v4-flash', 'deepseek-v4-pro'] },
    })
    mocks.getProviderModels.mockResolvedValue({
      data: [
        { id: 'pro-id', model_id: 'deepseek-v4-pro', name: 'Pro', provider_id: 'provider-id', type: 'chat', enable: false },
        { id: 'flash-id', model_id: 'deepseek-v4-flash', name: 'Flash', provider_id: 'provider-id', type: 'chat', enable: false },
      ],
    })
    mocks.updateModel.mockResolvedValue({ data: {} })
  })

  it('materializes a preset through its template', async () => {
    const preset = {
      id: 'deepseek',
      name: 'DeepSeek',
      clientType: 'openai-completions',
      baseUrl: 'https://api.deepseek.com/v1',
      source: 'deepseek.yaml',
    }
    mocks.getProviderTemplates.mockResolvedValue({
      data: [{ id: 'template-id', domain: 'llm', key: 'deepseek', name: 'DeepSeek' }],
    })
    mocks.postProviderFromTemplate.mockResolvedValue({ data: { id: 'provider-id' } })
    const { app, ready, setup } = mountSetup(preset)
    setup.initFormValues(preset)
    setup.formValues.value.api_key = 'sk-test'

    await setup.saveAndNext()

    expect(mocks.postProviderFromTemplate).toHaveBeenCalledWith({
      body: {
        template_id: 'template-id',
        domain: 'llm',
        name: 'DeepSeek',
        config: { base_url: 'https://api.deepseek.com/v1', api_key: 'sk-test' },
      },
      throwOnError: true,
    })
    expect(mocks.postProvider).not.toHaveBeenCalled()
    expect(mocks.getProviderByName).not.toHaveBeenCalled()
    expect(mocks.updateModel).not.toHaveBeenCalled()
    expect(ready).toHaveBeenCalledWith({ providerId: 'provider-id' })
    app.unmount()
  })

  it('recovers the same template provider when the create response was lost', async () => {
    const preset = {
      id: 'deepseek',
      name: 'DeepSeek',
      clientType: 'openai-completions',
      baseUrl: 'https://api.deepseek.com/v1',
      source: 'deepseek.yaml',
    }
    mocks.getProviderTemplates.mockResolvedValue({
      data: [{ id: 'template-id', domain: 'llm', key: 'deepseek' }],
    })
    mocks.postProviderFromTemplate.mockRejectedValue({ code: 'provider.name_taken' })
    mocks.getProviderByName.mockResolvedValue({
      data: { id: 'provider-id', provider_template_id: 'template-id' },
    })
    mocks.updateProvider.mockResolvedValue({ data: { id: 'provider-id' } })
    const { app, ready, setup } = mountSetup(preset)
    setup.initFormValues(preset)
    setup.formValues.value.api_key = 'sk-test'

    await setup.saveAndNext()

    expect(mocks.updateProvider).toHaveBeenCalledWith({
      path: { id: 'provider-id' },
      body: {
        config: { base_url: 'https://api.deepseek.com/v1', api_key: 'sk-test' },
        enable: true,
      },
      throwOnError: true,
    })
    expect(ready).toHaveBeenCalledWith({ providerId: 'provider-id' })
    app.unmount()
  })

  it('does not recover a same-name custom provider for a template preset', async () => {
    const preset = {
      id: 'deepseek',
      name: 'DeepSeek',
      clientType: 'openai-completions',
      baseUrl: 'https://api.deepseek.com/v1',
      source: 'deepseek.yaml',
    }
    mocks.getProviderTemplates.mockResolvedValue({
      data: [{ id: 'template-id', domain: 'llm', key: 'deepseek' }],
    })
    mocks.postProviderFromTemplate.mockRejectedValue({ code: 'provider.name_taken' })
    mocks.getProviderByName.mockResolvedValue({
      data: { id: 'custom-provider-id' },
    })
    const { app, ready, setup } = mountSetup(preset)
    setup.initFormValues(preset)
    setup.formValues.value.api_key = 'sk-test'

    await setup.saveAndNext()

    expect(mocks.updateProvider).not.toHaveBeenCalled()
    expect(ready).not.toHaveBeenCalled()
    app.unmount()
  })

  it('keeps the generic provider endpoint for the custom entry', async () => {
    mocks.getProviderByName.mockResolvedValue({ data: null })
    mocks.postProvider.mockResolvedValue({ data: { id: 'provider-id' } })
    const { app, setup } = mountSetup(null)
    setup.initFormValues(null)
    setup.formValues.value = {
      name: 'My Provider',
      api_key: 'sk-test',
      base_url: 'https://example.com/v1',
      client_type: 'openai-completions',
    }

    await setup.saveAndNext()

    expect(mocks.postProvider).toHaveBeenCalledWith({
      body: {
        name: 'My Provider',
        client_type: 'openai-completions',
        config: { base_url: 'https://example.com/v1', api_key: 'sk-test' },
      },
      throwOnError: true,
    })
    expect(mocks.getProviderTemplates).not.toHaveBeenCalled()
    expect(mocks.postProviderFromTemplate).not.toHaveBeenCalled()
    app.unmount()
  })


  it('does not advance when an imported provider has no chat model', async () => {
    const preset = {
      id: 'deepseek',
      name: 'DeepSeek',
      clientType: 'openai-completions',
      baseUrl: 'https://api.deepseek.com/v1',
      source: 'deepseek.yaml',
    }
    mocks.getProviderTemplates.mockResolvedValue({
      data: [{ id: 'template-id', domain: 'llm', key: 'deepseek' }],
    })
    mocks.postProviderFromTemplate.mockResolvedValue({ data: { id: 'provider-id' } })
    mocks.importModels.mockResolvedValue({
      data: { created: 1, updated: 0, skipped: 0, models: ['embedding'] },
    })
    mocks.getProviderModels.mockResolvedValue({
      data: [{ id: 'embedding-id', model_id: 'embedding', type: 'embedding', enable: false }],
    })
    const { app, ready, setup } = mountSetup(preset)
    setup.initFormValues(preset)
    setup.formValues.value.api_key = 'sk-test'

    await setup.saveAndNext()

    expect(ready).not.toHaveBeenCalled()
    expect(setup.errorState.value).toBe('noModels')
    app.unmount()
  })

  it('reports an HTTP failure when the imported model list cannot be loaded', async () => {
    mocks.getProviderByName.mockResolvedValue({ data: null })
    mocks.postProvider.mockResolvedValue({ data: { id: 'provider-id' } })
    mocks.getProviderModels.mockRejectedValue(new Error('network unavailable'))
    const { app, ready, setup } = mountSetup(null)
    setup.initFormValues(null)
    setup.formValues.value = {
      name: 'My Provider',
      api_key: 'sk-test',
      base_url: 'https://example.com/v1',
      client_type: 'openai-completions',
    }

    await setup.saveAndNext()

    expect(mocks.getProviderModels).toHaveBeenCalledWith({
      path: { id: 'provider-id' },
      throwOnError: true,
    })
    expect(ready).not.toHaveBeenCalled()
    expect(setup.errorState.value).toBe('http')
    app.unmount()
  })
})
