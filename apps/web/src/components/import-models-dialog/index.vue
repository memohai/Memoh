<template>
  <FormDialogShell
    v-model:open="open"
    :title="$t('models.importModels')"
    :cancel-text="$t('common.cancel')"
    :submit-text="$t('common.import')"
    :submit-disabled="false"
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
        <p class="text-sm text-muted-foreground">
          {{ $t('models.importConfirmHint') }}
        </p>
      </div>
    </template>
  </FormDialogShell>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postProvidersByIdImportModels } from '@memohai/sdk'
import { toast } from 'vue-sonner'
import { Button } from '@memohai/ui'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import { useDialogMutation } from '@/composables/useDialogMutation'

const props = defineProps<{
  providerId: string
}>()

const open = ref(false)
const { t } = useI18n()
const { run } = useDialogMutation()
const queryCache = useQueryCache()

const { mutateAsync: importModelsMutation, isLoading } = useMutation({
  mutation: async () => {
    const { data } = await postProvidersByIdImportModels({
      path: { id: props.providerId },
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
