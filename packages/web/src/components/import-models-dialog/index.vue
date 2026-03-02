<template>
  <FormDialogShell
    v-model:open="open"
    :title="$t('models.importModels')"
    :cancel-text="$t('common.cancel')"
    :submit-text="$t('common.import')"
    :submit-disabled="!clientType"
    :loading="isLoading"
    @submit="handleImport"
  >
    <template #trigger>
      <Button
        variant="outline"
        class="flex items-center gap-2"
      >
        <FontAwesomeIcon :icon="['fas', 'file-import']" />
        {{ $t('models.importModels') }}
      </Button>
    </template>
    <template #body>
      <div class="flex flex-col gap-3 mt-4">
        <Label class="mb-2">
          {{ $t('models.importClientType') }}
        </Label>
        <SearchableSelectPopover
          v-model="clientType"
          :options="clientTypeOptions"
          :placeholder="$t('models.clientTypePlaceholder')"
          class="w-full"
        />
        <p class="text-[0.8rem] text-muted-foreground">
          {{ $t('models.importClientTypeHint') }}
        </p>
      </div>
    </template>
  </FormDialogShell>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postProvidersByIdImportModels } from '@memoh/sdk'
import { toast } from 'vue-sonner'
import { Button, Label } from '@memoh/ui'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import { CLIENT_TYPE_LIST, CLIENT_TYPE_META } from '@/constants/client-types'
import { useDialogMutation } from '@/composables/useDialogMutation'

const props = defineProps<{
  providerId: string
}>()

const open = ref(false)
const { t } = useI18n()
const { run } = useDialogMutation()
const queryCache = useQueryCache()

const clientType = ref('openai-completions')

const clientTypeOptions = computed(() =>
  CLIENT_TYPE_LIST.map((ct) => ({
    value: ct.value,
    label: ct.label,
    description: ct.hint,
    keywords: [ct.label, ct.hint, CLIENT_TYPE_META[ct.value]?.value ?? ct.value],
  })),
)

const { mutateAsync: importModelsMutation, isLoading } = useMutation({
  mutation: async () => {
    const { data } = await postProvidersByIdImportModels({
      path: { id: props.providerId },
      body: { client_type: clientType.value },
      throwOnError: true,
    })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['provider-models'] })
  },
})

async function handleImport() {
  await run(
    () => importModelsMutation(),
    {
      fallbackMessage: t('models.importFailed'),
      onSuccess: (data) => {
        if (data) {
          toast.success(t('models.importSuccess', {
            created: data.created,
            skipped: data.skipped,
          }))
        }
        open.value = false
      },
    },
  )
}
</script>
