import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import type { Ref } from 'vue'
import type { ProvidertemplatesGetResponse } from '@memohai/sdk'

interface QueryOptions {
  key: () => string[]
  query: () => Promise<unknown>
  enabled: () => boolean
}

const mocks = vi.hoisted(() => ({
  getTemplate: vi.fn(),
  templateData: undefined as unknown as Ref<ProvidertemplatesGetResponse | null | undefined>,
  queryOptions: undefined as unknown as QueryOptions,
}))

vi.mock('@memohai/sdk', () => ({
  getProviderTemplatesById: mocks.getTemplate,
}))

vi.mock('@pinia/colada', async () => {
  const { ref } = await import('vue')
  mocks.templateData = ref()
  return {
    useQuery: (options: QueryOptions) => {
      mocks.queryOptions = options
      return {
        data: mocks.templateData,
        isLoading: ref(false),
      }
    },
  }
})

import { useProviderTemplateModels } from './useProviderTemplateModels'

describe('useProviderTemplateModels', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.templateData.value = undefined
  })

  it('loads a template detail only when a template ID is available', async () => {
    const templateId = ref<string>()
    useProviderTemplateModels(templateId)

    expect(mocks.queryOptions.enabled()).toBe(false)
    expect(mocks.queryOptions.key()).toEqual(['provider-template-models', ''])

    templateId.value = ' template-anthropic '
    expect(mocks.queryOptions.enabled()).toBe(true)
    expect(mocks.queryOptions.key()).toEqual(['provider-template-models', 'template-anthropic'])

    const detail = { id: 'template-anthropic', models: [] }
    mocks.getTemplate.mockResolvedValue({ data: detail })
    await expect(mocks.queryOptions.query()).resolves.toEqual(detail)
    expect(mocks.getTemplate).toHaveBeenCalledWith({
      path: { id: 'template-anthropic' },
      throwOnError: true,
    })
  })

  it('returns model drafts without tenant instance IDs', async () => {
    const { models } = useProviderTemplateModels(ref('template-anthropic'))
    mocks.templateData.value = {
      id: 'template-anthropic',
      models: [
        {
          id: 'template-model-id',
          model_id: 'claude-sonnet-4-5',
          name: 'Claude Sonnet 4.5',
          type: 'chat',
          config: { context_window: 200000 },
        },
        { id: 'empty-model', model_id: '  ' },
      ],
    }
    await nextTick()

    expect(models.value).toEqual([{
      model_id: 'claude-sonnet-4-5',
      name: 'Claude Sonnet 4.5',
      type: 'chat',
      config: { context_window: 200000 },
    }])
    expect(models.value[0]).not.toHaveProperty('id')
  })
})
