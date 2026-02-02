<template>
  <div class="p-4  **:[input]:mt-3 **:[input]:mb-4">
    <section class="flex justify-between items-center ">
      <h4 class="scroll-m-20   tracking-tight">
        {{ curProvider?.name }}
      </h4>
      <Switch />
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
          <template #default="{ close}">
            <PopoverTrigger as-child>
              <Button variant="destructive">
                删除
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
                  <Button @click="() => { deleteProvider();close() }">
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
          :disabled="isChange||!form.meta.value.valid"
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
          v-if="curProvider?.id!==undefined"
          :id="curProvider?.id as string"
        />
      </section>
      <section class="flex flex-col gap-4">
        <Item variant="outline">
          <ItemContent>
            <ItemTitle>Deep Seek R1</ItemTitle>
            <ItemDescription>
              <Badge variant="secondary">
                Chat
              </Badge>
            </ItemDescription>
          </ItemContent>
          <ItemActions>
            <Button
              variant="outline"
              size="sm"
            >
              编辑
            </Button>
          </ItemActions>
        </Item>
        <Item variant="outline">
          <ItemContent>
            <ItemTitle>Deep Seek R1</ItemTitle>
            <ItemDescription>
              <Badge variant="secondary">
                Chat
              </Badge>
            </ItemDescription>
          </ItemContent>
          <ItemActions>
            <Button
              variant="outline"
              size="sm"
            >
              编辑
            </Button>
          </ItemActions>
        </Item>
        <Item variant="outline">
          <ItemContent>
            <ItemTitle>Deep Seek R1</ItemTitle>
            <ItemDescription>
              <Badge variant="secondary">
                Chat
              </Badge>              
            </ItemDescription>
          </ItemContent>
          <ItemActions>
            <Button
              variant="outline"
              size="sm"
            >
              编辑
            </Button>
          </ItemActions>
        </Item>
      </section>
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
} from '@memoh/ui'
import CreateModel from '@/components/CreateModel/index.vue'
import { computed, inject,ref, toValue, watch } from 'vue'
import { type ProviderInfo } from '@memoh/shared'
import { useMutation,useQuery,useQueryCache } from '@pinia/colada'
import request from '@/utils/request'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'

const providerSchema=toTypedSchema(z.object({
  name: z.string().min(1),
  base_url: z.string().min(1),
  client_type: z.string().min(1),
  api_key: z.string().min(1),
  metadata: z.object({
    additionalProp1:z.object()
  })
}))

const form=useForm({
  validationSchema: providerSchema
})

const curProvider = inject('curProvider', ref<Partial<ProviderInfo & { id: string }>>())

const queryCache=useQueryCache()
const {mutate:deleteProvider,isLoading:deleteLoading}=useMutation({
  mutation:()=> request({
    url: `/providers/${curProvider.value?.id}`,
    method:'DELETE'
  }),
  onSettled: () => queryCache.invalidateQueries({
    key:['provider']
  })
})

const { mutate: changeProvider, isLoading: editLoading } = useMutation({
  mutation: (data:typeof form.values) => request({
    url: `/providers/${curProvider.value?.id}`,
    method: 'PUT',
    data
  }),
  onSettled: () => queryCache.invalidateQueries({
    key: ['provider']
  })
})

useQuery({
  key: ['model'],
  
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
    api_key:newVal?.api_key
  })
}, {
  immediate:true
})

const isChange=computed(() => {
  const rawCurProvider = toValue(curProvider)
  return JSON.stringify(form.values)===JSON.stringify({
    name: rawCurProvider?.name,
    base_url: rawCurProvider?.base_url,
    client_type: rawCurProvider?.client_type,
    api_key: rawCurProvider?.api_key,
    metadata: {
      additionalProp1:{}
    }
  })
})

</script>