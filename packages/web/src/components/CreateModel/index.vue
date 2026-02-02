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
            <DialogTitle> {{ $t("button.add", { msg: "Model" }) }}</DialogTitle>
            <DialogDescription class="mb-4">
              <Separator class="my-4" />
            </DialogDescription>
          </DialogHeader>
          <div class="flex flex-col gap-3">
            <FormField
              v-slot="{ componentField }"
              name="name"
            >
              <FormItem>
                <Label class="mb-2">
                  Name
                </Label>
                <FormControl>
                  <Input
                    type="text"
                    v-bind="componentField"
                  />
                </FormControl>
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="model_id"
            >
              <FormItem>
                <Label class="mb-2">
                  Model ID
                </Label>
                <FormControl>
                  <Input
                    type="text"
                    v-bind="componentField"
                  />
                </FormControl>
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="enable_as"
            >
              <FormItem>
                <Label class="mb-2">
                  Enable as
                </Label>
                <FormControl>
                  <Select v-bind="componentField">
                    <SelectTrigger class="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value="chat">
                          Chat
                        </SelectItem>
                        <SelectItem value="embedding">
                          embedding
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </FormControl>
              </FormItem>
            </FormField>
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
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value="chat">
                          Chat
                        </SelectItem>
                        <SelectItem value="embedding">
                          embedding
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </FormControl>
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="is_multimodal"
            >
              <FormItem>
                <Label class="mb-2">
                  是否开启多模态
                </Label>
                <Switch
                  id="airplane-mode"
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
              {{ $t("button.add", { msg: "Model" }) }}
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
  Spinner
} from '@memoh/ui'
import { useForm } from 'vee-validate'
import { inject, watch, type Ref, ref } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import request from '@/utils/request'
import { useMutation, useQueryCache } from '@pinia/colada'

const formSchema = toTypedSchema(z.object({
  'is_multimodal': z.coerce.boolean(),
  'model_id': z.string().min(1),
  'name': z.string().min(1),
  'type': z.string().min(1),
  'enable_as':z.string().min(1)
}))

const form = useForm({
  validationSchema: formSchema
})

const { id } = defineProps<{ id: string }>()

const queryCache = useQueryCache()
type ModelInfoType = Parameters<(Parameters<typeof form.handleSubmit>)[0]>[0]
const { mutate: createModel,isLoading } = useMutation({
  mutation: (modelInfo: ModelInfoType & {
    dimensions: number,
    enable_as: string,
    llm_provider_id: string
  }) => request({
    url: '/models',
    data: {
      ...modelInfo,
    },
    method: 'post'
  }),
  onSettled: () => { open.value = false; queryCache.invalidateQueries({ key: ['models'], exact: true }) }
})


const addModel = form.handleSubmit(async (modelInfo) => {  
  try {
    await createModel({
      ...modelInfo,
      dimensions: 0,     
      llm_provider_id: id
    })
    open.value=false
  } catch {
    return
  }

})

const open = inject<Ref<boolean>>('open', ref(false))
const editInfo = inject('editModelInfo', ref<null | (ModelInfoType & { id: string })>(null))
watch(open, () => {
  if (open.value && editInfo?.value) {
    form.setValues(editInfo.value)
  }
}, {
  immediate: true
})
</script>