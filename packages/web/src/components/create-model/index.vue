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
                  {{ $t('models.type') }}
                </Label>
                <FormControl>
                  <Select v-bind="componentField">
                    <SelectTrigger class="w-full">
                      <SelectValue :placeholder="$t('models.typePlaceholder')" />
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
              v-slot="{ componentField }"
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
                    v-bind="componentField"
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

            <!-- Multimodal (chat only) -->
            <FormField
              v-if="selectedType === 'chat'"
              v-slot="{ componentField }"
              name="is_multimodal"
            >
              <FormItem class="flex items-center justify-between">
                <Label>
                  {{ $t('models.multimodal') }}
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
                {{ $t('common.cancel') }}
              </Button>
            </DialogClose>
            <Button
              type="submit"
              :disabled="!form.meta.value.valid"
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
  type: z.string().min(1),
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
