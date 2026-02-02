<template>
  <section>
    <section class="max-w-187 m-auto">
      <Card>
        <form @submit="changeSetting">
          <CardHeader>
            <CardTitle class="text-2xl font-semibold tracking-tight">
              Settings
            </CardTitle>
            <CardDescription>
              Model Settings
            </CardDescription>
          </CardHeader>
          <CardContent class="mt-4">
            <FormField
              v-slot="{ componentField }"
              name="defaultChatModel"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  Chat Model
                </FormLabel>
                <FormControl>
                  <Select v-bind="componentField">
                    <SelectTrigger class="w-full">
                      <SelectValue :placeholder="$t('prompt.select',{msg:'Client Type'})" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem
                          v-for="(modelItem, index) in modelType.chat"
                          :key="modelItem.id"
                          :value="(modelItem.model.apiKey + index)"
                        >
                          {{ modelItem.model.name }}
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </FormControl>
                <blockquote class="h-5">
                  <FormMessage />
                </blockquote>
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="defaultEmbeddingModel"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  Embedding Model
                </FormLabel>
                <FormControl>
                  <Select v-bind="componentField">
                    <SelectTrigger class="w-full">
                      <SelectValue :placeholder="$t('prompt.select',{msg:'Embedding Type'})" />
                    </SelectTrigger>
                    <SelectContent v-if="modelType.embedding.length > 0">
                      <SelectGroup>
                        <SelectItem
                          v-for="(modelItem, index) in modelType.embedding"
                          :key="modelItem.id"
                          :value="(modelItem.model.apiKey + index)"
                        >
                          {{ modelItem.model.name }}
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>

                  <blockquote class="h-5">
                    <FormMessage />
                  </blockquote>
                </formcontrol>
              </FormItem>
            </FormField>

            <FormField
              v-slot="{ componentField }"
              name="defaultSummaryModel"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  <!-- defaultSummaryModel -->
                  Summary Model
                </FormLabel>
                <FormControl>
                  <Select v-bind="componentField">
                    <SelectTrigger class="w-full">
                      <SelectValue :placeholder="$t('prompt.select', { msg: 'Summary Type' })" />
                    </SelectTrigger>
                    <SelectContent v-if="modelType.embedding.length > 0">
                      <SelectGroup>
                        <SelectItem
                          v-for="(modelItem, index) in modelType.summary"
                          :key="modelItem.id"
                          :value="(modelItem.model.apiKey + index)"
                        >
                          {{ modelItem.model.name }}
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </FormControl>
                <blockquote class="h-5">
                  <FormMessage />
                </blockquote>
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="language"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  Language
                </FormLabel>
                <FormControl>
                  <Select v-bind="componentField">
                    <SelectTrigger class="w-full">
                      <SelectValue                    
                        :placeholder="$t('prompt.select', { msg: 'Language' })"
                      />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value="ch">
                          中文
                        </SelectItem>
                        <SelectItem value="en">
                          English
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </FormControl>
                <blockquote class="h-5">
                  <FormMessage />
                </blockquote>
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="maxContextLoadTime"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  <!-- defaultSummaryModel -->
                  Timeout
                </FormLabel>
                <FormControl>
                  <Input
                    :placeholder="$t('prompt.enter',{msg:'Timeout'})"
                    v-bind="componentField"
                  />
                </FormControl>
                <blockquote class="h-5">
                  <FormMessage />
                </blockquote>
              </FormItem>
            </FormField>
          </CardContent>
          <CardFooter class="flex">
            <Button
              class="ml-auto"
              type="submit"
              :disabled="diabeld"
            >
              Change
            </Button>
          </CardFooter>
        </form>
      </Card>
    </section>
  </section>
</template>

<script setup lang="ts">
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import request from '@/utils/request'
import { watch, reactive, computed } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm, useFormValues } from 'vee-validate'
import {
  Input,
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
  CardContent,
  FormField,
  FormItem,
  FormLabel,
  FormControl,
  Button,
  FormMessage,
  Select,
  SelectTrigger,
  SelectContent,
  SelectValue,
  SelectGroup,
  SelectItem,
  CardFooter
} from '@memoh/ui'

type ModelList = {
  id: ModelTable['id'],
  model: Omit<ModelTable, 'id' | 'defaultChatModel' | 'defaultEmbeddingModel' | 'defaultSummaryModel'>
};

const modelType = reactive<{
  chat: ModelList[],
  embedding: ModelList[],
  summary: ModelList[]
}>({
  chat: [],
  embedding: [],
  summary: []
})

const { data: settingData } = useQuery({
  key: ['Setting'],
  query: async () => {
    const modelData = await request({
      url: '/model/',
      method: 'get'
    })
    for (const modelItems of modelData.data.items) {
      let type = modelItems.model.type as keyof typeof modelType
      modelType[type].push(modelItems)
    }
    return await request({
      url: '/settings/',
      method: 'get'
    })
  }
})

const formSchema = toTypedSchema(z.object({
  defaultChatModel: z.any(),
  defaultEmbeddingModel: z.any(),
  defaultSummaryModel: z.any(),
  maxContextLoadTime: z.coerce.number().min(1500),
  language: z.literal(['ch', 'en'])
}))

const form = useForm({
  validationSchema: formSchema
})

const currentSetting = useFormValues()

const diabeld = computed(() => {
  return Object.keys(currentSetting.value).every((property) => {
    const curKey = currentSetting.value[property]
    const cacheKey = settingData.value?.data?.data?.[property]
    if (curKey === cacheKey || Number(curKey) === Number(cacheKey)) {
      return true
    }
  })
})
watch(settingData, () => {
  form.setValues({
    ...(settingData.value?.data.data ?? {})
  })
}, {
  immediate: true
})


const cacheQuery=useQueryCache()
const { mutate: fetchSetting } = useMutation({
  mutation: (data:typeof currentSetting.value) => request({
    url: '/settings/',
    data
  }),
  onSettled: () => {
    cacheQuery.invalidateQueries({
      key:['Setting']
    })
  }
})
const changeSetting = form.handleSubmit(async (value) => {
 
  try {
    await fetchSetting(value)    
  } catch {
      return 
    }  
})

</script>