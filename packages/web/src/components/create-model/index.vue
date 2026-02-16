<template>
  <section class="ml-auto">
    <Dialog v-model:open="open">
      <DialogTrigger as-child>
        <Button variant="default">
          {{ $t('models.addModel') }}
        </Button>
      </DialogTrigger>
      <DialogContent class="sm:max-w-106.25">
        <form @submit="addModel">
          <DialogHeader>
            <DialogTitle>
              {{ title === 'edit' ? $t('models.editModel') : $t('models.addModel') }}
            </DialogTitle>
            <DialogDescription class="mb-4">
              <Separator class="my-4" />
            </DialogDescription>
          </DialogHeader>
          <div class="flex flex-col gap-3">
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
                    <SelectTrigger class="w-full">
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
          </div>
          <DialogFooter class="mt-4">
            <DialogClose as-child>
              <Button variant="outline">
                {{ $t('common.cancel') }}
              </Button>
            </DialogClose>
            <Button
              type="submit"
              :disabled="!canSubmit"
            >
              <Spinner v-if="isLoading" />
              {{ title === 'edit' ? $t('common.save') : $t('models.addModel') }}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </section>
</template>

<script setup lang="ts">
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
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
  Separator,
  Label,
  Spinner,
} from '@memoh/ui'
import { useForm } from 'vee-validate'
import { inject, computed, watch, nextTick, type Ref, ref } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postModels, putModelsModelByModelId } from '@memoh/sdk'
import type { ModelsGetResponse } from '@memoh/sdk'

const availableInputModalities = ['text', 'image', 'audio', 'video', 'file'] as const
const selectedModalities = ref<string[]>(['text'])

const formSchema = toTypedSchema(z.object({
  type: z.string().min(1),
  model_id: z.string().min(1),
  name: z.string().optional(),
  dimensions: z.coerce.number().min(1).optional(),
}))

const form = useForm({
  validationSchema: formSchema,
})

const selectedType = computed(() => form.values.type || editInfo?.value?.type)

const open = inject<Ref<boolean>>('openModel', ref(false))
const title = inject<Ref<'edit' | 'title'>>('openModelTitle', ref('title'))
const editInfo = inject<Ref<ModelsGetResponse | null>>('openModelState', ref(null))

// 保存按钮：编辑模式直接可提交（表单已预填充，handleSubmit 内部会校验）
// 新建模式需要必填字段有值
const canSubmit = computed(() => {
  if (title.value === 'edit') return true
  const { type, model_id } = form.values
  return !!type && !!model_id
})

function toggleModality(mod: string, checked: boolean) {
  if (checked) {
    selectedModalities.value = [...selectedModalities.value, mod]
  } else {
    selectedModalities.value = selectedModalities.value.filter(m => m !== mod)
  }
}

const emptyValues = {
  type: '' as string,
  model_id: '' as string,
  name: '' as string,
  dimensions: undefined as number | undefined,
}

// Display Name 自动跟随 Model ID，除非用户主动修改过
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
const isLoading = computed(() => createLoading.value || updateLoading.value)

async function addModel(e: Event) {
  e.preventDefault()

  const isEdit = title.value === 'edit' && !!editInfo?.value
  const fallback = editInfo?.value

  // 从 form.values 读取，编辑模式用 editInfo 兜底
  // （Dialog 异步渲染可能导致 vee-validate 内部状态未同步）
  const type = form.values.type || (isEdit ? fallback!.type : '')
  const model_id = form.values.model_id || (isEdit ? fallback!.model_id : '')
  const name = form.values.name ?? (isEdit ? fallback!.name : '')
  const dimensions = form.values.dimensions ?? (isEdit ? fallback!.dimensions : undefined)

  if (!type || !model_id) return

  try {
    const payload: Record<string, unknown> = {
      type,
      model_id,
      llm_provider_id: id,
    }

    if (name) {
      payload.name = name
    }

    if (type === 'embedding' && dimensions) {
      payload.dimensions = dimensions
    }

    if (type === 'chat') {
      payload.input_modalities = selectedModalities.value.length > 0 ? selectedModalities.value : ['text']
    }

    if (isEdit) {
      await updateModel({ modelId: fallback!.model_id, data: payload as any })
    } else {
      await createModel(payload as any)
    }
    open.value = false
  } catch {
    return
  }
}

watch(open, async () => {
  if (!open.value) {
    title.value = 'title'
    editInfo.value = null
    return
  }

  // 等待 Dialog 内容和 FormField 组件挂载完成
  await nextTick()

  if (editInfo?.value) {
    const { type, model_id, name, dimensions, input_modalities } = editInfo.value
    form.resetForm({ values: { type, model_id, name, dimensions } })
    selectedModalities.value = input_modalities ?? ['text']
    userEditedName.value = !!(name && name !== model_id)
  } else {
    form.resetForm({ values: { ...emptyValues } })
    selectedModalities.value = ['text']
    userEditedName.value = false
  }
}, {
  immediate: true,
})
</script>
