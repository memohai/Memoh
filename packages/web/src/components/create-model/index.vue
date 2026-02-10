<template>
  <section class="ml-auto">
    <Dialog v-model:open="open">
      <DialogTrigger as-child>
        <Button variant="default">
          {{ $t("button.add", { msg: "Model" }) }}
        </Button>
      </DialogTrigger>
      <DialogContent class="sm:max-w-106.25">
        <form @submit="addModel">
          <DialogHeader>
            <DialogTitle>
              {{ title === 'edit' ? '编辑Model' : '添加Model' }}
            </DialogTitle>
            <DialogDescription class="mb-4">
              <Separator class="my-4" />
            </DialogDescription>
          </DialogHeader>
          <div class="flex flex-col gap-3">
            <!-- 1. Type（先选类型） -->
            <FormField
              v-slot="{ componentField }"
              name="type"
            >
              <FormItem>
                <Label class="mb-2">
                  Type
                </Label>
                <FormControl>
                  <Select v-bind="componentField">
                    <SelectTrigger class="w-full">
                      <SelectValue placeholder="选择模型类型" />
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

            <!-- 2. Model（原 Model ID） -->
            <FormField
              v-slot="{ componentField }"
              name="model_id"
            >
              <FormItem>
                <Label class="mb-2">
                  Model
                </Label>
                <FormControl>
                  <Input
                    type="text"
                    placeholder="e.g. gpt-4o"
                    v-bind="componentField"
                  />
                </FormControl>
              </FormItem>
            </FormField>

            <!-- 3. Display Name（可选） -->
            <FormField
              v-slot="{ componentField }"
              name="name"
            >
              <FormItem>
                <Label class="mb-2">
                  Display Name
                  <span class="text-muted-foreground text-xs ml-1">(optional)</span>
                </Label>
                <FormControl>
                  <Input
                    type="text"
                    placeholder="自定义显示名称"
                    v-bind="componentField"
                  />
                </FormControl>
              </FormItem>
            </FormField>

            <!-- 4. Dimensions（仅 embedding 时显示） -->
            <FormField
              v-if="selectedType === 'embedding'"
              v-slot="{ componentField }"
              name="dimensions"
            >
              <FormItem>
                <Label class="mb-2">
                  Dimensions
                </Label>
                <FormControl>
                  <Input
                    type="number"
                    placeholder="e.g. 1536"
                    v-bind="componentField"
                  />
                </FormControl>
              </FormItem>
            </FormField>

            <!-- 5. 多模态（仅 chat 时显示） -->
            <FormField
              v-if="selectedType === 'chat'"
              v-slot="{ componentField }"
              name="is_multimodal"
            >
              <FormItem class="flex items-center justify-between">
                <Label>
                  是否开启多模态
                </Label>
                <Switch
                  v-model="componentField.modelValue"
                  @update:model-value="componentField['onUpdate:modelValue']"
                />
              </FormItem>
            </FormField>
          </div>
          <DialogFooter class="mt-4">
            <DialogClose as-child>
              <Button variant="outline">
                Cancel
              </Button>
            </DialogClose>
            <Button
              type="submit"
              :disabled="!form.meta.value.valid"
            >
              <Spinner v-if="isLoading" />
              {{ title === 'edit' ? '保存' : $t("button.add", { msg: "Model" }) }}
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
  Switch,
  Separator,
  Label,
  Spinner,
} from '@memoh/ui'
import { useForm } from 'vee-validate'
import { inject, computed, watch, type Ref, ref } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { type ModelInfo } from '@memoh/shared'
import { useCreateModel } from '@/composables/api/useModels'

const formSchema = toTypedSchema(z.object({
  type: z.string().min(1, '请选择模型类型'),
  model_id: z.string().min(1),
  name: z.string().optional(),
  dimensions: z.coerce.number().min(1).optional(),
  is_multimodal: z.coerce.boolean().optional(),
}))

const form = useForm({
  validationSchema: formSchema,
})

const selectedType = computed(() => form.values.type)

const { id } = defineProps<{ id: string }>()

const { mutate: createModel, isLoading } = useCreateModel()

const addModel = form.handleSubmit(async (values) => {
  try {
    const payload: Record<string, unknown> = {
      type: values.type,
      model_id: values.model_id,
      llm_provider_id: id,
    }

    if (values.name) {
      payload.name = values.name
    }

    if (values.type === 'embedding' && values.dimensions) {
      payload.dimensions = values.dimensions
    }

    if (values.type === 'chat') {
      payload.is_multimodal = values.is_multimodal ?? false
    }

    await createModel(payload as any)
    open.value = false
  } catch {
    return
  }
})

const open = inject<Ref<boolean>>('openModel', ref(false))
const title = inject<Ref<'edit' | 'title'>>('openModelTitle', ref('title'))
const editInfo = inject<Ref<ModelInfo | null>>('openModelState', ref(null))

watch(open, () => {
  if (open.value && editInfo?.value) {
    form.setValues(editInfo.value)
  } else {
    form.resetForm()
  }

  if (!open.value) {
    title.value = 'title'
    editInfo.value = null
  }
}, {
  immediate: true,
})
</script>
