<template>
  <div class="p-4  **:[input]:mt-3 **:[input]:mb-4">
    <section class="flex justify-between items-center ">
      <h4 class="scroll-m-20   tracking-tight">
        {{ curProvider?.name }}
      </h4>
    </section>
    <Separator class="mt-4 mb-6" />
    <form @submit="editProvider">
      <section>
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          Name
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="name"
        >
          <FormItem>
            <FormControl>
              <Input
                type="text"
                placeholder="请输入API密钥"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>
      <section>
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          API 密钥
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="api_key"
        >
          <FormItem>
            <FormControl>
              <Input
                type="text"
                placeholder="请输入API密钥"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>

      <section>
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          URL
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="base_url"
        >
          <FormItem>
            <FormControl>
              <Input
                type="text"
                placeholder="请输入URL"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>
      <section class="flex justify-end mt-4 gap-4">
        <Popover>
          <template #default="{ close }">
            <PopoverTrigger as-child>
              <Button variant="outline">
                <svg-icon
                  type="mdi"
                  :path="mdiTrashCanOutline"
                />
              </Button>
            </PopoverTrigger>
            <PopoverContent class="w-80">
              <div class="grid gap-4">
                <p class="leading-7 not-first:mt-6  ">
                  确认是否删除模型平台?
                </p>
                <section class="flex gap-4">
                  <Button
                    variant="outline"
                    class="ml-auto"
                  >
                    取消
                  </Button>
                  <Button @click="() => { deleteProvider(); close() }">
                    <Spinner v-if="deleteLoading" />
                    确定
                  </Button>
                </section>
              </div>
            </PopoverContent>
          </template>
        </Popover>
        <Button
          type="submit"
          :disabled="isChange || !form.meta.value.valid"
        >
          <Spinner v-if="editLoading" />
          确定修改
        </Button>
      </section>
    </form>
    <Separator class="mt-4 mb-6" />
    <section>
      <section class="flex justify-between items-center mb-4 ">
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          模型
        </h4>
        <CreateModel
          v-if="curProvider?.id !== undefined"
          :id="curProvider?.id as string"
        />
      </section>
      <section
        v-if="modelDataList?.length > 0"
        class="flex flex-col gap-4"
      >
        <Item
          v-for="modelData in modelDataList"
          :key="modelData.model_id"
          variant="outline"
        >
          <ItemContent>
            <ItemTitle>
              {{ modelData.name }}                     
            </ItemTitle>
            <ItemDescription class="gap-2 flex flex-wrap items-center mt-3 ">
              <Badge
                variant="outline"
              >
                {{ modelData.type }}
              </Badge>
            </ItemDescription>
          </ItemContent>
          <ItemActions>
            <Select
              :default-value="modelData.enable_as"
              @update:model-value="(value) => {
                modelData.value = value
                enableModel({
                  as: value as string === 'empty' ? '' : value as string,
                  model_id: modelData.model_id
                })
           
              }"
            >
              <SelectTrigger class="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="empty">
                    No Enable
                  </SelectItem>
                  <SelectItem value="chat">
                    Chat
                  </SelectItem>
                  <SelectItem value="embedding">
                    Embedding
                  </SelectItem>
                  <SelectItem value="memery">
                    Memery
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <Button
              variant="outline"
              class="cursor-pointer"
              @click="() => {
                openModel.state = true;
                openModel.title = 'edit';
                openModel.curState = deleteEnableAd(modelData)

              }"
            >
              <svg-icon
                type="mdi"
                :path="mdiCog"
              />
            </Button>
            <Popover>
              <template #default="{ close }">
                <PopoverTrigger as-child>
                  <Button variant="outline">
                    <svg-icon
                      type="mdi"
                      :path="mdiTrashCanOutline"
                    />
                  </Button>
                </PopoverTrigger>
                <PopoverContent class="w-80">
                  <div class="grid gap-4">
                    <p class="leading-7 not-first:mt-6  ">
                      确认是否删除模型?
                    </p>
                    <section class="flex gap-4">
                      <Button
                        variant="outline"
                        class="ml-auto"
                      >
                        取消
                      </Button>
                      <Button @click="() => { deleteModel(modelData.name); close() }">
                        <Spinner v-if="deleteModelLoading" />
                        确定
                      </Button>
                    </section>
                  </div>
                </PopoverContent>
              </template>
            </Popover>
          </ItemActions>
        </Item>
      </section>

      <Empty
        v-else
        class="h-full flex justify-center items-center"
      >
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <svg-icon
              type="mdi"
              :path="mdiListBoxOutline"
            />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>还没有添加模型</EmptyTitle>
        <EmptyDescription>请为当前Provider添加模型</EmptyDescription>
        <EmptyContent>
          <!-- <Button>Add data</Button> -->
        </EmptyContent>
      </Empty>
    </section>
  </div>
</template>

<script setup lang="ts">
import {
  Switch, Separator, Spinner, Input, Button,
  FormControl,
  FormField,
  FormItem,
  Item,
  ItemContent,
  ItemDescription,
  ItemActions,
  ItemTitle,
  Badge,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem
} from '@memoh/ui'
import CreateModel from '@/components/CreateModel/index.vue'
import { computed, inject, provide, reactive, ref, toRef, toValue, watch } from 'vue'
import { type ProviderInfo } from '@memoh/shared'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import request from '@/utils/request'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import SvgIcon from '@jamescoyle/vue-icon'
import { mdiListBoxOutline, mdiCog, mdiTrashCanOutline } from '@mdi/js'
import { type ModelInfo } from '@memoh/shared'

const openModel = reactive<{
  state: boolean,
  title: 'title' | 'edit',
  curState: ModelInfo | null
}>({
  state: false,
  title: 'title',
  curState: null
})

provide('openModel', toRef(openModel, 'state'))
provide('openModelTitle', toRef(openModel, 'title'))
provide('openModelState', toRef(openModel, 'curState'))

const deleteEnableAd = (value:ModelInfo) => {
  const copyModelData = { ...value }
  if ('enable_as' in copyModelData) {
    delete copyModelData['enable_as']
  }
  return copyModelData
}

const providerSchema = toTypedSchema(z.object({
  name: z.string().min(1),
  base_url: z.string().min(1),
  client_type: z.string().min(1),
  api_key: z.string().min(1),
  metadata: z.object({
    additionalProp1: z.object()
  })
}))

const form = useForm({
  validationSchema: providerSchema
})

const curProvider = inject('curProvider', ref<Partial<ProviderInfo & { id: string }>>())

const queryCache = useQueryCache()
const { mutate: deleteProvider, isLoading: deleteLoading } = useMutation({
  mutation: () => request({
    url: `/providers/${curProvider.value?.id}`,
    method: 'DELETE'
  }),
  onSettled: () => queryCache.invalidateQueries({
    key: ['provider']
  })
})

const { mutate: changeProvider, isLoading: editLoading } = useMutation({
  mutation: (data: typeof form.values) => request({
    url: `/providers/${curProvider.value?.id}`,
    method: 'PUT',
    data
  }),
  onSettled: () => queryCache.invalidateQueries({
    key: ['provider']
  })
})

const { mutate: deleteModel, isLoading: deleteModelLoading } = useMutation({
  mutation: (id) => request({
    url: `/models/model/${id}`,
    method: 'DELETE'
  }),
  onSettled: () => queryCache.invalidateQueries({
    key: ['model']
  })
})

const { mutate: enableModel } = useMutation({
  mutation: (data: { as: string, model_id: string }) => (request({
    url: '/models/enable',
    data,
    method: 'post'
  })),
  onSettled: () => queryCache.invalidateQueries({
    key: ['model']
  })
})

const { mutate: updateMultimodal } = useMutation({
  mutation: (data: ModelInfo) => request({
    url: `models/model/${data?.model_id}`,
    data,
    method: 'PUT'
  }),
  onSettled: () => {
    queryCache.invalidateQueries({
      key: ['model']
    })
  }
})


const { data: modelDataList } = useQuery({
  key: ['model'],
  query: () => request({
    url: `/providers/${curProvider.value?.id}/models`,
  }).then(fetchData => fetchData.data.map((model: ModelInfo) => ({
    ...model,
    enable_as: model.enable_as ?? 'empty'
  })))
})

const editProvider = form.handleSubmit(async (value) => {
  try {
    await changeProvider(value)
  } catch {
    return
  }
})


watch(curProvider, (newVal) => {
  form.setValues({
    name: newVal?.name,
    base_url: newVal?.base_url,
    client_type: newVal?.client_type,
    api_key: newVal?.api_key
  })
  queryCache.invalidateQueries({
    key: ['model']
  })
}, {
  immediate: true
})

const isChange = computed(() => {
  const rawCurProvider = toValue(curProvider)
  return JSON.stringify(form.values) === JSON.stringify({
    name: rawCurProvider?.name,
    base_url: rawCurProvider?.base_url,
    client_type: rawCurProvider?.client_type,
    api_key: rawCurProvider?.api_key,
    metadata: {
      additionalProp1: {}
    }
  })
})

</script>