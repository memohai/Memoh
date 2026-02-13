<template>
  <section>
    <Dialog v-model:open="open">
      <DialogTrigger as-child>
        <Button
          class="w-full shadow-none! text-muted-foreground mb-4"
          variant="outline"
        >
          <FontAwesomeIcon
            :icon="['fas', 'plus']"
            class="mr-1"
          /> {{ $t('provider.addBtn') }}
        </Button>
      </DialogTrigger>
      <DialogContent class="sm:max-w-106.25">
        <form @submit="createProvider">
          <DialogHeader>
            <DialogTitle>{{ $t('provider.add') }}</DialogTitle>
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
                  {{ $t('common.name') }}
                </Label>
                <FormControl>
                  <Input
                    type="text"
                    :placeholder="$t('common.namePlaceholder')"
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
                  {{ $t('provider.apiKey') }}
                </Label>
                <FormControl>
                  <Input
                    type="text"
                    :placeholder="$t('provider.apiKeyPlaceholder')"
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
                  {{ $t('provider.url') }}
                </Label>
                <FormControl>
                  <Input
                    type="text"
                    :placeholder="$t('provider.urlPlaceholder')"
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
                  {{ $t('common.type') }}
                </Label>
                <FormControl>
                  <Select v-bind="componentField">
                    <SelectTrigger class="w-full">
                      <SelectValue :placeholder="$t('common.typePlaceholder')" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem
                          v-for="type in CLIENT_TYPES"
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
                {{ $t('common.cancel') }}
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
              {{ $t('provider.add') }}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </section>
</template>
<script setup lang="ts">
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
  Spinner,
} from '@memoh/ui'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postProviders } from '@memoh/sdk'
import type { ProvidersClientType } from '@memoh/sdk'

const CLIENT_TYPES: ProvidersClientType[] = [
  'openai', 'openai-compat', 'anthropic', 'google',
  'azure', 'bedrock', 'mistral', 'xai', 'ollama', 'dashscope',
]

const open = defineModel<boolean>('open')

const queryCache = useQueryCache()
const { mutate: providerFetch, isLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    const { data: result } = await postProviders({ body: data as any, throwOnError: true })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['providers'] }),
})

const providerSchema = toTypedSchema(z.object({
  api_key: z.string().min(1),
  base_url: z.string().min(1),
  client_type: z.string().min(1),
  name: z.string().min(1),
  metadata: z.object({
    additionalProp1: z.object({}),
  }),
}))

const form = useForm({
  validationSchema: providerSchema,
})

const createProvider = form.handleSubmit(async (value) => {
  try {
    await providerFetch(value)
    open.value = false
  } catch {
    return
  }
})
</script>
