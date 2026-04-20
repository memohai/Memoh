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
                      <SelectItem value="image">
                        Image
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </FormControl>
            </FormItem>
          </FormField>

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

          <!-- Compatibilities -->
          <div v-if="selectedCompatibilityOptions.length > 0">
            <Label class="mb-4">
              {{ $t('models.compatibilities') }}
            </Label>
            <div class="flex flex-wrap gap-3 mt-2">
              <label
                v-for="opt in selectedCompatibilityOptions"
                :key="opt.value"
                class="flex items-center gap-1.5 text-xs"
              >
                <Checkbox
                  :model-value="selectedCompat.includes(opt.value)"
                  @update:model-value="(val: boolean) => toggleCompat(opt.value, val)"
                />
                {{ $t(`models.compatibility.${opt.value}`) }}
              </label>
            </div>
          </div>

          <!-- Context Window (optional) -->
          <FormField
            v-if="selectedType === 'chat'"
            v-slot="{ componentField }"
            name="context_window"
          >
            <FormItem>
              <Label class="mb-2">
                {{ $t('models.contextWindow') }}
                <span class="text-muted-foreground text-xs ml-1">({{ $t('common.optional') }})</span>
              </Label>
              <FormControl>
                <Input
                  type="number"
                  :placeholder="$t('models.contextWindowPlaceholder')"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>
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
} from '@memohai/ui'
import { useForm } from 'vee-validate'
import { inject, computed, watch, nextTick, type Ref, ref } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postModels, putModelsById, putModelsModelByModelId } from '@memohai/sdk'
import type { ModelsGetResponse, ModelsAddRequest, ModelsUpdateRequest } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import { CHAT_COMPATIBILITY_OPTIONS, IMAGE_COMPATIBILITY_OPTIONS } from '@/constants/compatibilities'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import { useDialogMutation } from '@/composables/useDialogMutation'

const selectedCompat = ref<string[]>([])
const { t } = useI18n()
const { run } = useDialogMutation()

const formSchema = toTypedSchema(z.object({
  type: z.string().min(1),
  model_id: z.string().min(1),
  name: z.string().optional(),
  dimensions: z.coerce.number().min(1).optional(),
  context_window: z.coerce.number().min(1).optional(),
}))

const form = useForm({
  validationSchema: formSchema,
  initialValues: {
    type: 'chat',
  },
})

const selectedType = computed(() => form.values.type || 'chat')
const selectedCompatibilityOptions = computed(() => {
  switch (selectedType.value) {
    case 'chat':
      return CHAT_COMPATIBILITY_OPTIONS
    case 'image':
      return IMAGE_COMPATIBILITY_OPTIONS
    default:
      return []
  }
})

watch(selectedCompatibilityOptions, (options) => {
  const allowed = new Set(options.map(option => option.value))
  selectedCompat.value = selectedCompat.value.filter(value => allowed.has(value))
}, { immediate: true })

const open = inject<Ref<boolean>>('openModel', ref(false))
const title = inject<Ref<'edit' | 'title'>>('openModelTitle', ref('title'))
const editInfo = inject<Ref<ModelsGetResponse | null>>('openModelState', ref(null))

const canSubmit = computed(() => {
  if (title.value === 'edit') return true
  const { type, model_id } = form.values
  if (!type || !model_id) return false
  return true
})

function toggleCompat(cap: string, checked: boolean) {
  if (checked) {
    selectedCompat.value = [...selectedCompat.value, cap]
  } else {
    selectedCompat.value = selectedCompat.value.filter(c => c !== cap)
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
    const { data: result } = await postModels({ body: data as ModelsAddRequest, throwOnError: true })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['provider-models'] }),
})
const { mutateAsync: updateModel, isLoading: updateLoading } = useMutation({
  mutation: async ({ id, data }: { id: string; data: Record<string, unknown> }) => {
    const { data: result } = await putModelsById({
      path: { id },
      body: data as ModelsUpdateRequest,
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
      body: data as ModelsUpdateRequest,
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
  const model_id = form.values.model_id || (isEdit ? fallback!.model_id : '')
  const name = form.values.name ?? (isEdit ? fallback!.name : '')

  if (!type || !model_id) return

  const config: Record<string, unknown> = {}

  if (type === 'embedding') {
    const dim = form.values.dimensions ?? (isEdit ? fallback!.config?.dimensions : undefined)
    if (dim) config.dimensions = dim
  }

  if (type === 'chat' || type === 'image') {
    config.compatibilities = selectedCompat.value
  }

  if (type === 'chat') {
    const ctxWin = form.values.context_window ?? (isEdit ? fallback!.config?.context_window : undefined)
    if (ctxWin) config.context_window = ctxWin
  }

  const payload: Record<string, unknown> = {
    type,
    model_id,
    provider_id: id,
    config,
  }

  if (name) {
    payload.name = name
  }

  await run(
    () => {
      if (isEdit) {
        const modelUUID = fallback?.id
        if (modelUUID) {
          return updateModel({ id: modelUUID, data: payload as ModelsUpdateRequest })
        }
        return updateModelByLegacyModelID({ modelId: fallback!.model_id, data: payload as ModelsUpdateRequest })
      }
      return createModel(payload)
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
    const { type, model_id, name, config } = editInfo.value
    form.resetForm({
      values: {
        type: type || 'chat',
        model_id,
        name,
        dimensions: config?.dimensions,
        context_window: config?.context_window,
      },
    })
    selectedCompat.value = config?.compatibilities ?? []
    userEditedName.value = !!(name && name !== model_id)
  } else {
    form.resetForm({ values: { type: 'chat', model_id: '', name: '', dimensions: undefined, context_window: undefined } })
    selectedCompat.value = []
    userEditedName.value = false
  }
}, {
  immediate: true,
})
</script>
