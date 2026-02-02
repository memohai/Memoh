<template>
  <section>
    <Dialog v-model:open="open">
      <DialogTrigger as-child>
        <Button
          class="w-full shadow-none! text-muted-foreground mb-4"
          variant="outline"
        >
          <svg-icon
            type="mdi"
            :path="mdiPlus"
            class="mr-1"
          /> 添加
        </Button>
      </DialogTrigger>
      <DialogContent class="sm:max-w-106.25">
        <form @submit="createProvider">
          <DialogHeader>
            <DialogTitle>添加提供商</DialogTitle>
            <DialogDescription>
              <Separator class="my-4" />
            </DialogDescription>
          </DialogHeader>
        
          
          <div class="flex-col gap-3 flex">
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
                    placeholder="请输入Name"
                    v-bind="componentField"
                  />
                </FormControl>
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="api_key"
            >
              <FormItem>
                <Label class="mb-2">
                  API 密钥
                </Label>
                <FormControl>
                  <Input
                    type="text"
                    placeholder="请输入Api Key"
                    v-bind="componentField"
                  />
                </FormControl>
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="base_url"
            >
              <FormItem>
                <Label class="mb-2">
                  URL
                </Label>
                <FormControl>
                  <Input
                    type="text"
                    placeholder="请输入URL"
                    v-bind="componentField"
                  />
                </FormControl>             
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="client_type"
            >
              <FormItem>
                <Label class="mb-2">
                  Type
                </Label>
                <FormControl>
                  <Select v-bind="componentField">
                    <SelectTrigger class="w-full">
                      <SelectValue :placeholder="$t('prompt.select', { msg: 'Type' })" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem
                          v-for="type in clientType"
                          :key="type"
                          :value="type"
                        >
                          {{ type }}
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </FormControl>
              </FormItem>
            </FormField>
          </div>
          <DialogFooter class="mt-8">
            <DialogClose as-child>
              <Button variant="outline">
                Cancel
              </Button>
            </DialogClose>
            <Button
              type="submit"
              :disabled="(form.meta.value.valid===false)||isLoading"
            >
              <Spinner
                v-if="isLoading"
                class="mr-1"
              />
              添加MCP
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </section>
</template>
<script setup lang="ts">
import { mdiPlus } from '@mdi/js'
import SvgIcon from '@jamescoyle/vue-icon'
import {
  Button,
  Dialog,
  DialogClose,
  DialogContent,  
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  Input,
  FormField,
  FormControl,
  FormItem,
  DialogDescription,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
  Separator,
  Label,
  Spinner
} from '@memoh/ui'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQueryCache } from '@pinia/colada'
import request from '@/utils/request'
import { type ProviderInfo } from '@memoh/shared'
import { clientType } from '@memoh/shared'


const open = defineModel<boolean>('open')

const cacheQuery=useQueryCache()
const {mutate:providerFetch,isLoading}=useMutation({
  mutation: (data: ProviderInfo) => request({
    url: '/providers',
    data,
    method:'post'
  }),
  onSettled: () => cacheQuery.invalidateQueries({
    key:['provider']
  })
})
const providerSchema = toTypedSchema(z.object({
  api_key: z.string().min(1),
  base_url: z.string().min(1),
  client_type: z.string().min(1),
  name: z.string().min(1),
  metadata: z.object({
    additionalProp1:z.object()
  })
}))

const form = useForm({
  validationSchema: providerSchema,
})

const createProvider=form.handleSubmit(async (value) => {
  try {
  
    await providerFetch(value)
    open.value=false
  } catch {
    return
  }
})

</script>