<template>
  <section class="flex">
    <Dialog v-model:open="open">
      <DialogTrigger as-child>
        <Button
          variant="default"
          class="ml-auto my-4"
        >
          {{ $t('platform.addTitle') }}
        </Button>
      </DialogTrigger>
      <DialogContent class="sm:max-w-106.25">
        <form @submit="addPlatform">
          <DialogHeader>
            <DialogTitle>{{ $t('platform.addTitle') }}</DialogTitle>
            <DialogDescription class="mb-4">
              {{ $t('platform.addDescription') }}
            </DialogDescription>
          </DialogHeader>

          <div class="flex flex-col gap-3">
            <!-- Name -->
            <FormField
              v-slot="{ componentField }"
              name="name"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  {{ $t('platform.name') }}
                </FormLabel>
                <FormControl>
                  <Input
                    type="text"
                    :placeholder="$t('platform.namePlaceholder')"
                    v-bind="componentField"
                    autocomplete="name"
                  />
                </FormControl>
                <blockquote class="h-5">
                  <FormMessage />
                </blockquote>
              </FormItem>
            </FormField>

            <!-- Config (key:value tags) -->
            <FormField
              v-slot="{ componentField }"
              name="config"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  {{ $t('platform.config') }}
                </FormLabel>
                <FormControl>
                  <TagsInput
                    :add-on-blur="true"
                    :model-value="configTags.tagList.value"
                    :convert-value="configTags.convertValue"
                    @update:model-value="(tags) => configTags.handleUpdate(tags.map(String), componentField['onUpdate:modelValue'])"
                  >
                    <TagsInputItem
                      v-for="(value, index) in configTags.tagList.value"
                      :key="index"
                      :value="value"
                    >
                      <TagsInputItemText />
                      <TagsInputItemDelete />
                    </TagsInputItem>
                    <TagsInputInput
                      :placeholder="$t('platform.configPlaceholder')"
                      class="w-full py-1"
                    />
                  </TagsInput>
                </FormControl>
                <blockquote class="h-5">
                  <FormMessage />
                </blockquote>
              </FormItem>
            </FormField>

            <!-- Active -->
            <FormField
              v-slot="{ componentField }"
              name="active"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  {{ $t('platform.active') }}
                </FormLabel>
                <FormControl>
                  <Switch
                    :model-value="componentField.modelValue"
                    @update:model-value="componentField['onUpdate:modelValue']"
                  />
                </FormControl>
                <blockquote class="h-5">
                  <FormMessage />
                </blockquote>
              </FormItem>
            </FormField>
          </div>

          <DialogFooter class="mt-4">
            <DialogClose as-child>
              <Button variant="outline">
                {{ $t('common.cancel') }}
              </Button>
            </DialogClose>
            <Button type="submit">
              {{ $t('platform.addTitle') }}
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
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  Input,
  FormField,
  FormControl,
  FormItem,
  FormLabel,
  FormMessage,
  TagsInput,
  TagsInputInput,
  TagsInputItem,
  TagsInputItemDelete,
  TagsInputItemText,
  Switch,
} from '@memoh/ui'
import z from 'zod'
import { toTypedSchema } from '@vee-validate/zod'
import { useForm } from 'vee-validate'
import { ref, inject } from 'vue'
import { useKeyValueTags } from '@/composables/useKeyValueTags'
import { useCreatePlatform } from '@/composables/api/usePlatform'

const configTags = useKeyValueTags()

const validationSchema = toTypedSchema(z.object({
  name: z.string().min(1),
  config: z.looseObject({}),
  active: z.coerce.boolean(),
}))

const form = useForm({ validationSchema })

const { mutate: addFetchPlatform } = useCreatePlatform()

const addPlatform = form.handleSubmit(async (value) => {
  try {
    await addFetchPlatform(value)
    open.value = false
  } catch {
    return
  }
})

const open = inject('open', ref<boolean>(false))
</script>
