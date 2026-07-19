import { computed, toValue } from 'vue'
import type { MaybeRefOrGetter } from 'vue'
import { useQuery } from '@pinia/colada'
import { getProviderTemplatesById } from '@memohai/sdk'

export interface ProviderTemplateModelDraft {
  model_id: string
  name?: string
  type?: string
  config: Record<string, unknown>
}

export function useProviderTemplateModels(templateId: MaybeRefOrGetter<string | undefined>) {
  const resolvedTemplateId = computed(() => toValue(templateId)?.trim() || undefined)

  const { data: templateDetail, isLoading } = useQuery({
    key: () => ['provider-template-models', resolvedTemplateId.value ?? ''],
    query: async () => {
      const id = resolvedTemplateId.value
      if (!id) return null
      const { data } = await getProviderTemplatesById({
        path: { id },
        throwOnError: true,
      })
      return data ?? null
    },
    enabled: () => !!resolvedTemplateId.value,
  })

  const models = computed<ProviderTemplateModelDraft[]>(() => (templateDetail.value?.models ?? [])
    .filter((model): model is typeof model & { model_id: string } => !!model.model_id?.trim())
    .map(model => ({
      model_id: model.model_id,
      name: model.name,
      type: model.type,
      config: { ...(model.config ?? {}) },
    })))

  return {
    models,
    isLoading,
  }
}
