<template>
  <section class="flex">
    <Dialog v-model:open="open">
      <DialogTrigger as-child>
        <Button
          variant="default"
          class="ml-auto my-4"
        >
          {{ $t('mcp.addTitle') }}
        </Button>
      </DialogTrigger>
      <DialogContent class="sm:max-w-106.25">
        <form @submit="createMCP">
          <DialogHeader>
            <DialogTitle>{{ $t('mcp.addTitle') }}</DialogTitle>
            <DialogDescription class="mb-4">
              {{ $t('mcp.addDescription') }}
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
                  {{ $t('mcp.name') }}
                </FormLabel>
                <FormControl>
                  <Input
                    type="text"
                    :placeholder="$t('mcp.namePlaceholder')"
                    v-bind="componentField"
                    autocomplete="name"
                  />
                </FormControl>
                <blockquote class="h-5">
                  <FormMessage />
                </blockquote>
              </FormItem>
            </FormField>

            <!-- Type -->
            <FormField
              v-slot="{ componentField }"
              name="config.type"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  {{ $t('mcp.type') }}
                </FormLabel>
                <FormControl>
                  <Select v-bind="componentField">
                    <SelectTrigger class="w-full">
                      <SelectValue :placeholder="$t('mcp.typePlaceholder')" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value="stdio">
                          Stdio
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

            <!-- Cwd -->
            <FormField
              v-slot="{ componentField }"
              name="config.cwd"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  {{ $t('mcp.cwd') }}
                </FormLabel>
                <FormControl>
                  <Input
                    type="text"
                    :placeholder="$t('mcp.cwdPlaceholder')"
                    v-bind="componentField"
                    autocomplete="cwd"
                  />
                </FormControl>
                <blockquote class="h-5">
                  <FormMessage />
                </blockquote>
              </FormItem>
            </FormField>

            <!-- Command -->
            <FormField
              v-slot="{ componentField }"
              name="config.command"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  {{ $t('mcp.command') }}
                </FormLabel>
                <FormControl>
                  <Input
                    :placeholder="$t('mcp.commandPlaceholder')"
                    v-bind="componentField"
                  />
                </FormControl>
                <blockquote class="h-5">
                  <FormMessage />
                </blockquote>
              </FormItem>
            </FormField>

            <!-- Arguments -->
            <FormField
              v-slot="{ componentField }"
              name="config.args"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  {{ $t('mcp.arguments') }}
                </FormLabel>
                <FormControl>
                  <TagsInput
                    v-model="componentField.modelValue"
                    :add-on-blur="true"
                    :duplicate="true"
                    @update:model-value="componentField['onUpdate:modelValue']"
                  >
                    <TagsInputItem
                      v-for="item in componentField.modelValue"
                      :key="item"
                      :value="item"
                    >
                      <TagsInputItemText />
                      <TagsInputItemDelete />
                    </TagsInputItem>
                    <TagsInputInput
                      :placeholder="$t('mcp.argumentsPlaceholder')"
                      class="w-full py-1"
                    />
                  </TagsInput>
                </FormControl>
                <blockquote class="h-5">
                  <FormMessage />
                </blockquote>
              </FormItem>
            </FormField>

            <!-- Env (key:value tags) -->
            <FormField
              v-slot="{ componentField }"
              name="config.env"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  {{ $t('mcp.env') }}
                </FormLabel>
                <FormControl>
                  <TagsInput
                    :add-on-blur="true"
                    :model-value="envTags.tagList.value"
                    :convert-value="envTags.convertValue"
                    @update:model-value="(tags) => envTags.handleUpdate(tags.map(String), componentField['onUpdate:modelValue'])"
                  >
                    <TagsInputItem
                      v-for="(value, index) in envTags.tagList.value"
                      :key="index"
                      :value="value"
                    >
                      <TagsInputItemText />
                      <TagsInputItemDelete />
                    </TagsInputItem>
                    <TagsInputInput
                      :placeholder="$t('mcp.envPlaceholder')"
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
                <FormControl>
                  <section class="flex gap-4">
                    <Label>{{ $t('mcp.active') }}</Label>
                    <Switch
                      :model-value="componentField.modelValue"
                      @update:model-value="componentField['onUpdate:modelValue']"
                    />
                  </section>
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
              {{ $t('mcp.addTitle') }}
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
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
  FormItem,
  FormLabel,
  FormMessage,
  TagsInput,
  TagsInputInput,
  TagsInputItem,
  TagsInputItemDelete,
  TagsInputItemText,
  Switch,
  Label,
} from '@memoh/ui'
import z from 'zod'
import { toTypedSchema } from '@vee-validate/zod'
import { useForm } from 'vee-validate'
import { ref, inject, watch } from 'vue'
import { type MCPListItem as MCPType } from '@memoh/shared'
import { useKeyValueTags } from '@/composables/useKeyValueTags'
import { useCreateOrUpdateMcp } from '@/composables/api/useMcp'

// ---- Env key:value 转换 ----
const envTags = useKeyValueTags()

// ---- 表单 ----
const validateSchema = toTypedSchema(z.object({
  name: z.string().min(1),
  config: z.object({
    type: z.string().min(1),
    command: z.string().min(1),
    args: z.array(z.coerce.string().check(z.minLength(1))).min(1),
    env: z.looseObject({}),
    cwd: z.string().min(1),
  }),
  active: z.coerce.boolean(),
}))

const form = useForm({
  validationSchema: validateSchema,
})

// ---- API ----
const { mutate: fetchMCP } = useCreateOrUpdateMcp()

// ---- Dialog & 编辑状态 ----
const open = inject('open', ref(false))
const mcpEditData = inject('mcpEditData', ref<{
  name: string
  config: MCPType['config']
  active: boolean
  id: string
} | null>(null))

watch(open, () => {
  if (open.value && mcpEditData.value) {
    form.setValues(mcpEditData.value)
    envTags.initFromObject(mcpEditData.value.config?.env as Record<string, string>)
  }
  if (!open.value) {
    mcpEditData.value = null
  }
}, { immediate: true })

const createMCP = form.handleSubmit(async (value) => {
  try {
    await fetchMCP({ ...value, id: mcpEditData.value?.id })
    open.value = false
  } catch {
    return
  }
})
</script>
