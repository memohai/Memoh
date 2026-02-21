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
          /> {{ $t('searchProvider.add') }}
        </Button>
      </DialogTrigger>
      <DialogContent class="sm:max-w-106.25">
        <form @submit="handleCreate">
          <DialogHeader>
            <DialogTitle>{{ $t('searchProvider.add') }}</DialogTitle>
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
                <Label
                  class="mb-2"
                  :for="componentField.id || 'search-provider-create-name'"
                >
                  {{ $t('common.name') }}
                </Label>
                <FormControl>
                  <Input
                    :id="componentField.id || 'search-provider-create-name'"
                    type="text"
                    :placeholder="$t('common.namePlaceholder')"
                    v-bind="componentField"
                    :aria-label="$t('common.name')"
                  />
                </FormControl>
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="provider"
            >
              <FormItem>
                <Label
                  class="mb-2"
                  :for="componentField.id || 'search-provider-create-type'"
                >
                  {{ $t('searchProvider.provider') }}
                </Label>
                <FormControl>
                  <Select v-bind="componentField">
                    <SelectTrigger
                      :id="componentField.id || 'search-provider-create-type'"
                      class="w-full"
                      :aria-label="$t('searchProvider.provider')"
                    >
                      <SelectValue :placeholder="$t('common.typePlaceholder')" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem
                          v-for="type in PROVIDER_TYPES"
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
              :disabled="(form.meta.value.valid === false) || isLoading"
            >
              <Spinner
                v-if="isLoading"
                class="mr-1"
              />
              {{ $t('searchProvider.add') }}
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
import { postSearchProviders } from '@memoh/sdk'

const PROVIDER_TYPES = ['brave'] as const

const open = defineModel<boolean>('open')

const queryCache = useQueryCache()
const { mutate: providerFetch, isLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    const { data: result } = await postSearchProviders({ body: data as any, throwOnError: true })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['search-providers'] }),
})

const providerSchema = toTypedSchema(z.object({
  name: z.string().min(1),
  provider: z.string().min(1),
}))

const form = useForm({
  validationSchema: providerSchema,
})

const handleCreate = form.handleSubmit(async (value) => {
  try {
    await providerFetch({ ...value, config: {} })
    open.value = false
  } catch {
    return
  }
})
</script>
