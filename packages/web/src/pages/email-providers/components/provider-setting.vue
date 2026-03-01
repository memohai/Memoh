<template>
  <div class="p-4">
    <section class="flex justify-between items-center">
      <div class="flex items-center gap-2">
        <FontAwesomeIcon
          :icon="['fas', 'envelope']"
          class="size-5"
        />
        <div>
          <h2 class="text-base font-semibold">
            {{ curProvider?.name }}
          </h2>
          <p class="text-xs text-muted-foreground">
            {{ curProvider?.provider }}
          </p>
        </div>
      </div>
    </section>
    <Separator class="mt-4 mb-6" />

    <form @submit="handleSave">
      <div class="space-y-4">
        <section>
          <FormField
            v-slot="{ componentField }"
            name="name"
          >
            <FormItem>
              <Label :for="componentField.id || 'email-provider-name'">
                {{ $t('common.name') }}
              </Label>
              <FormControl>
                <Input
                  :id="componentField.id || 'email-provider-name'"
                  type="text"
                  :placeholder="$t('common.namePlaceholder')"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>
        </section>

        <Separator class="my-4" />

        <!-- Dynamic config fields from meta schema -->
        <div
          v-for="field in orderedFields"
          :key="field.key"
          class="space-y-2"
        >
          <Label :for="field.type === 'bool' || field.type === 'enum' ? undefined : `email-field-${field.key}`">
            {{ $te(`emailProvider.fields.${field.key}`) ? $t(`emailProvider.fields.${field.key}`) : (field.title || field.key) }}
            <span
              v-if="!field.required"
              class="text-xs text-muted-foreground ml-1"
            >({{ $t('common.optional') }})</span>
          </Label>
          <p
            v-if="field.description"
            class="text-xs text-muted-foreground"
          >
            {{ field.description }}
          </p>

          <div
            v-if="field.type === 'secret'"
            class="relative"
          >
            <Input
              :id="`email-field-${field.key}`"
              v-model="configData[field.key]"
              :type="visibleSecrets[field.key] ? 'text' : 'password'"
              :placeholder="field.example ? String(field.example) : ''"
            />
            <button
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
              @click="visibleSecrets[field.key] = !visibleSecrets[field.key]"
            >
              <FontAwesomeIcon
                :icon="['fas', visibleSecrets[field.key] ? 'eye-slash' : 'eye']"
                class="size-3.5"
              />
            </button>
          </div>

          <Switch
            v-else-if="field.type === 'bool'"
            :model-value="!!configData[field.key]"
            @update:model-value="(val) => configData[field.key] = !!val"
          />

          <Input
            v-else-if="field.type === 'number'"
            :id="`email-field-${field.key}`"
            v-model.number="configData[field.key]"
            type="number"
            :placeholder="field.example ? String(field.example) : ''"
          />

          <Select
            v-else-if="field.type === 'enum' && field.enum"
            :model-value="String(configData[field.key] || '')"
            @update:model-value="(val) => configData[field.key] = val"
          >
            <SelectTrigger>
              <SelectValue :placeholder="field.title || field.key" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem
                v-for="opt in field.enum"
                :key="opt"
                :value="opt"
              >
                {{ opt }}
              </SelectItem>
            </SelectContent>
          </Select>

          <Input
            v-else
            :id="`email-field-${field.key}`"
            v-model="configData[field.key]"
            type="text"
            :placeholder="field.example ? String(field.example) : ''"
          />
        </div>
      </div>

      <section class="flex justify-end mt-6 gap-4">
        <ConfirmPopover
          :message="$t('emailProvider.deleteConfirm')"
          :loading="deleteLoading"
          @confirm="handleDelete"
        >
          <template #trigger>
            <Button
              type="button"
              variant="outline"
            >
              <FontAwesomeIcon :icon="['far', 'trash-can']" />
            </Button>
          </template>
        </ConfirmPopover>
        <LoadingButton
          type="submit"
          :loading="editLoading"
        >
          {{ $t('provider.saveChanges') }}
        </LoadingButton>
      </section>
    </form>
  </div>
</template>

<script setup lang="ts">
import {
  Input,
  Button,
  FormControl,
  FormField,
  FormItem,
  Separator,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  Switch,
  Label,
} from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import LoadingButton from '@/components/loading-button/index.vue'
import { computed, inject, reactive, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { putEmailProvidersById, deleteEmailProvidersById, getEmailProvidersMeta } from '@memoh/sdk'
import type { EmailProviderResponse, EmailFieldSchema } from '@memoh/sdk'

const { t } = useI18n()
const curProvider = inject('curEmailProvider', ref<EmailProviderResponse>())
const curProviderId = computed(() => curProvider.value?.id)

const { data: metaList } = useQuery({
  key: () => ['email-providers-meta'],
  query: async () => {
    const { data } = await getEmailProvidersMeta({ throwOnError: true })
    return data
  },
})

const currentMeta = computed(() => {
  if (!metaList.value || !curProvider.value?.provider) return null
  return (metaList.value as any[]).find((m: any) => m.provider === curProvider.value?.provider)
})

const orderedFields = computed<EmailFieldSchema[]>(() => {
  const fields = currentMeta.value?.config_schema?.fields
  if (!Array.isArray(fields)) return []
  return [...fields].sort((a, b) => (a.order ?? 0) - (b.order ?? 0))
})

const queryCache = useQueryCache()

const schema = toTypedSchema(z.object({
  name: z.string().min(1),
}))

const form = useForm({ validationSchema: schema })

const configData = reactive<Record<string, unknown>>({})
const visibleSecrets = reactive<Record<string, boolean>>({})

let loadedProviderId = ''
watch(() => curProvider.value?.id, (id) => {
  if (!id || id === loadedProviderId) return
  loadedProviderId = id
  const p = curProvider.value
  if (p) {
    form.setValues({ name: p.name ?? '' })
    const cfg = p.config ?? {}
    Object.keys(configData).forEach((k) => delete configData[k])
    Object.assign(configData, { ...cfg })
  }
}, { immediate: true })

const { mutateAsync: submitUpdate, isLoading: editLoading } = useMutation({
  mutation: async (data: { name: string; config: Record<string, unknown> }) => {
    if (!curProviderId.value) return
    const { data: result } = await putEmailProvidersById({
      path: { id: curProviderId.value },
      body: { name: data.name, config: data.config },
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['email-providers'] }),
})

const { mutateAsync: doDelete, isLoading: deleteLoading } = useMutation({
  mutation: async () => {
    if (!curProviderId.value) return
    await deleteEmailProvidersById({ path: { id: curProviderId.value }, throwOnError: true })
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['email-providers'] }),
})

const handleSave = form.handleSubmit(async (values) => {
  try {
    await submitUpdate({ name: values.name, config: { ...configData } })
    toast.success(t('provider.saveChanges'))
  } catch (e: any) {
    toast.error(e?.message || t('common.saveFailed'))
  }
})

async function handleDelete() {
  try {
    await doDelete()
    toast.success(t('common.deleteSuccess'))
  } catch (e: any) {
    toast.error(e?.message || t('common.saveFailed'))
  }
}
</script>
