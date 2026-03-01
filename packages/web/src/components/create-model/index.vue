<template>
  <section class="ml-auto">
    <FormDialogShell
      v-model:open="open"
      :title="title === 'edit' ? $t('models.editModel') : $t('models.addModel')"
      :cancel-text="$t('common.cancel')"
      :submit-text="title === 'edit' ? $t('common.save') : $t('models.addModel')"
      :submit-disabled="!canSubmit"
      :loading="isLoading"
      @submit="addModel"
    >
      <template #trigger>
        <Button variant="default">
          {{ $t('models.addModel') }}
        </Button>
      </template>
      <template #body>
        <div class="flex flex-col gap-3 mt-4">
          <!-- Type -->
          <FormField
            v-slot="{ componentField }"
            name="type"
          >
            <FormItem>
              <Label class="mb-2">
                {{ $t('common.type') }}
              </Label>
              <FormControl>
                <Select v-bind="componentField">
                  <SelectTrigger
                    class="w-full"
                    :aria-label="$t('common.type')"
                  >
                    <SelectValue :placeholder="$t('common.typePlaceholder')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="chat">
                        Chat
                      </SelectItem>
                      <SelectItem value="embedding">
                        Embedding
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </FormControl>
            </FormItem>
          </FormField>

          <!-- Client Type (chat only) -->
          <div v-if="selectedType === 'chat'">
            <Label class="mb-2">
              {{ $t('models.clientType') }}
            </Label>
            <SearchableSelectPopover
              v-model="clientTypeModel"
              :options="clientTypeOptions"
              :placeholder="$t('models.clientTypePlaceholder')"
              :aria-label="$t('models.clientType')"
              :search-placeholder="$t('models.clientTypePlaceholder')"
              :search-aria-label="$t('models.clientType')"
              class="mt-2"
              :show-group-headers="false"
            >
              <template #trigger="{ open, displayLabel }">
                <Button
                  variant="outline"
                  role="combobox"
                  :aria-expanded="open"
                  class="w-full justify-between font-normal mt-2"
                >
                  <span class="truncate">
                    {{ displayLabel || $t('models.clientTypePlaceholder') }}
                  </span>
                  <FontAwesomeIcon
                    :icon="['fas', 'chevron-down']"
                    class="ml-2 size-3 shrink-0 text-muted-foreground"
                  />
                </Button>
              </template>
            </SearchableSelectPopover>
          </div>

          <!-- Model -->
          <FormField
            v-slot="{ componentField }"
            name="model_id"
          >
            <FormItem>
              <Label class="mb-2">
                {{ $t('models.model') }}
              </Label>
              <FormControl>
                <Input
                  type="text"
                  :placeholder="$t('models.modelPlaceholder')"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>

          <!-- Display Name -->
          <FormField
            name="name"
          >
            <FormItem>
              <Label class="mb-2">
                {{ $t('models.displayName') }}
                <span class="text-muted-foreground text-xs ml-1">({{ $t('common.optional') }})</span>
              </Label>
              <FormControl>
                <Input
                  type="text"
                  :placeholder="$t('models.displayNamePlaceholder')"
                  :model-value="form.values.name ?? ''"
                  @input="onNameInput"
                />
              </FormControl>
            </FormItem>
          </FormField>

          <!-- Dimensions (embedding only) -->
          <FormField
            v-if="selectedType === 'embedding'"
            v-slot="{ componentField }"
            name="dimensions"
          >
            <FormItem>
              <Label class="mb-2">
                {{ $t('models.dimensions') }}
              </Label>
              <FormControl>
                <Input
                  type="number"
                  :placeholder="$t('models.dimensionsPlaceholder')"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>

          <!-- Input Modalities (chat only) -->
          <div v-if="selectedType === 'chat'">
            <Label class="mb-2">
              {{ $t('models.inputModalities') }}
            </Label>
            <div class="flex flex-wrap gap-3 mt-2">
              <label
                v-for="mod in availableInputModalities"
                :key="mod"
                class="flex items-center gap-1.5 text-sm"
              >
                <Checkbox
                  :model-value="selectedModalities.includes(mod)"
                  :disabled="mod === 'text'"
                  @update:model-value="(val: boolean) => toggleModality(mod, val)"
                />
                {{ $t(`models.modality.${mod}`) }}
              </label>
            </div>
          </div>

          <!-- Supports Reasoning (chat only) -->
          <div
            v-if="selectedType === 'chat'"
            class="flex items-center justify-between"
          >
            <Label>{{ $t('models.supportsReasoning') }}</Label>
            <Switch
              :model-value="supportsReasoning"
              @update:model-value="(val) => supportsReasoning = !!val"
            />
          </div>
        </div>
      </template>
    </FormDialogShell>
  </section>
</template>

<script setup lang="ts">
import {
  Input,
  Button,
  FormField,
  FormControl,
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
  FormItem,
  Checkbox,
  Label,
  Switch,
} from '@memoh/ui'
import { useForm } from 'vee-validate'
import { inject, computed, watch, nextTick, type Ref, ref } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postModels, putModelsById, putModelsModelByModelId } from '@memoh/sdk'
import type { ModelsGetResponse } from '@memoh/sdk'
import { useI18n } from 'vue-i18n'
import { CLIENT_TYPE_LIST, CLIENT_TYPE_META } from '@/constants/client-types'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'
import { useDialogMutation } from '@/composables/useDialogMutation'

const availableInputModalities = ['text', 'image', 'audio', 'video', 'file'] as const
const selectedModalities = ref<string[]>(['text'])
const supportsReasoning = ref(false)
const { t } = useI18n()
const { run } = useDialogMutation()

const formSchema = toTypedSchema(z.object({
  type: z.string().min(1),
  client_type: z.string().optional(),
  model_id: z.string().min(1),
  name: z.string().optional(),
  dimensions: z.coerce.number().min(1).optional(),
}))

const form = useForm({
  validationSchema: formSchema,
  initialValues: {
    type: 'chat',
  },
})

const selectedType = computed(() => form.values.type || 'chat')

const clientTypeModel = computed({
  get: () => form.values.client_type || '',
  set: (value: string) => form.setFieldValue('client_type', value),
})

const clientTypeOptions = computed<SearchableSelectOption[]>(() =>
  CLIENT_TYPE_LIST.map((ct) => ({
    value: ct.value,
    label: ct.label,
    description: ct.hint,
    keywords: [ct.label, ct.hint, CLIENT_TYPE_META[ct.value]?.value ?? ct.value],
  })),
)

const open = inject<Ref<boolean>>('openModel', ref(false))
const title = inject<Ref<'edit' | 'title'>>('openModelTitle', ref('title'))
const editInfo = inject<Ref<ModelsGetResponse | null>>('openModelState', ref(null))

const canSubmit = computed(() => {
  if (title.value === 'edit') return true
  const { type, model_id, client_type } = form.values
  if (!type || !model_id) return false
  if (type === 'chat' && !client_type) return false
  return true
})

function toggleModality(mod: string, checked: boolean) {
  if (checked) {
    selectedModalities.value = [...selectedModalities.value, mod]
  } else {
    selectedModalities.value = selectedModalities.value.filter(m => m !== mod)
  }
}

const userEditedName = ref(false)

watch(
  () => form.values.model_id,
  (newModelId) => {
    if (!userEditedName.value && newModelId !== undefined) {
      form.setFieldValue('name', newModelId)
    }
  },
)

function onNameInput(e: Event) {
  userEditedName.value = true
  form.setFieldValue('name', (e.target as HTMLInputElement).value)
}

const { id } = defineProps<{ id: string }>()

const queryCache = useQueryCache()
const { mutateAsync: createModel, isLoading: createLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    const { data: result } = await postModels({ body: data as any, throwOnError: true })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['provider-models'] }),
})
const { mutateAsync: updateModel, isLoading: updateLoading } = useMutation({
  mutation: async ({ id, data }: { id: string; data: Record<string, unknown> }) => {
    const { data: result } = await putModelsById({
      path: { id },
      body: data as any,
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['provider-models'] }),
})
const { mutateAsync: updateModelByLegacyModelID, isLoading: updateLegacyLoading } = useMutation({
  mutation: async ({ modelId, data }: { modelId: string; data: Record<string, unknown> }) => {
    const { data: result } = await putModelsModelByModelId({
      path: { modelId },
      body: data as any,
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['provider-models'] }),
})
const isLoading = computed(() => createLoading.value || updateLoading.value || updateLegacyLoading.value)

async function addModel() {
  
  const isEdit = title.value === 'edit' && !!editInfo?.value 
  const fallback = editInfo?.value

  const type = form.values.type || (isEdit ? fallback!.type : 'chat')
  const client_type = type === 'chat'
    ? (form.values.client_type || (isEdit ? fallback!.client_type : ''))
    : undefined
  const model_id = form.values.model_id || (isEdit ? fallback!.model_id : '')
  const name = form.values.name ?? (isEdit ? fallback!.name : '')
  const dimensions = form.values.dimensions ?? (isEdit ? fallback!.dimensions : undefined)

  if (!type || !model_id) return
  if (type === 'chat' && !client_type) return

  const payload: Record<string, unknown> = {
    type,
    model_id,
    llm_provider_id: id,
  }

  if (type === 'chat' && client_type) {
    payload.client_type = client_type
  }

  if (name) {
    payload.name = name
  }

  if (type === 'embedding' && dimensions) {
    payload.dimensions = dimensions
  }

  if (type === 'chat') {
    payload.input_modalities = selectedModalities.value.length > 0 ? selectedModalities.value : ['text']
    payload.supports_reasoning = supportsReasoning.value
  }

  await run(
    () => {
      if (isEdit) {
        const modelUUID = fallback?.id
        if (modelUUID) {
          return updateModel({ id: modelUUID, data: payload as any })
        }
        return updateModelByLegacyModelID({ modelId: fallback!.model_id, data: payload as any })
      }
      return createModel(payload as any)
    },
    {
      fallbackMessage: t('common.saveFailed'),
      onSuccess: () => {
        open.value = false
      },
    },
  )
}

watch(open, async () => {
  if (!open.value) {
    title.value = 'title'
    editInfo.value = null
    return
  }

  await nextTick()

  if (editInfo?.value) {
    const { client_type, type, model_id, name, dimensions, input_modalities } = editInfo.value
    form.resetForm({ values: { type: type || 'chat', client_type: client_type || '', model_id, name, dimensions } })
    selectedModalities.value = input_modalities ?? ['text']
    supportsReasoning.value = !!editInfo.value.supports_reasoning
    userEditedName.value = !!(name && name !== model_id)
  } else {
    form.resetForm({ values: { type: 'chat', client_type: '', model_id: '', name: '', dimensions: undefined } })
    selectedModalities.value = ['text']
    supportsReasoning.value = false
    userEditedName.value = false
  }
}, {
  immediate: true,
})

// Clear client_type when switching to embedding
watch(selectedType, (newType) => {
  if (newType === 'embedding') {
    form.setFieldValue('client_type', '')
  }
})
</script>
