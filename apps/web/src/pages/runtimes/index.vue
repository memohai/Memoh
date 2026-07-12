<template>
  <PageShell
    :title="t('runtimes.title')"
    :description="t('runtimes.description')"
  >
    <template #actions>
      <Button
        size="sm"
        @click="connectDialogOpen = true"
      >
        <Plus class="size-4" />
        {{ t('runtimes.connect') }}
      </Button>
    </template>

    <div class="space-y-8">
      <SettingsSection :title="t('runtimes.computers')">
        <InlineLoadingRow
          v-if="runtimesLoading && runtimes === undefined"
          surface="card-row"
        >
          {{ t('runtimes.loadingComputers') }}
        </InlineLoadingRow>

        <SettingsRow
          v-else-if="runtimesError && runtimes === undefined"
          :label="t('runtimes.loadFailed')"
          :description="t('runtimes.loadFailedDescription')"
        >
          <Button
            variant="outline"
            size="sm"
            @click="refetchRuntimes()"
          >
            {{ t('runtimes.retry') }}
          </Button>
        </SettingsRow>

        <div
          v-else-if="runtimeItems.length === 0"
          class="px-6 py-8 text-center"
        >
          <p class="text-sm font-medium text-foreground">
            {{ t('runtimes.emptyTitle') }}
          </p>
          <p class="mx-auto mt-1 max-w-md text-xs leading-relaxed text-muted-foreground">
            {{ t('runtimes.emptyDescription') }}
          </p>
          <Button
            class="mt-4"
            variant="outline"
            size="sm"
            @click="connectDialogOpen = true"
          >
            {{ t('runtimes.connect') }}
          </Button>
        </div>

        <SettingsRow
          v-for="runtime in runtimeItems"
          v-else
          :key="runtime.id"
          align="start"
          stack="sm"
        >
          <template #leading>
            <Laptop class="size-4 text-muted-foreground" />
          </template>
          <template #content>
            <p class="truncate text-sm font-medium text-foreground">
              {{ runtime.name }}
            </p>
            <p class="mt-0.5 truncate text-xs text-muted-foreground">
              {{ runtimeSummary(runtime) }}
            </p>
            <div
              v-if="!runtime.online && runtime.key"
              class="mt-3 flex min-w-0 items-start gap-2 rounded-md border border-border bg-muted-soft p-2"
            >
              <code class="min-w-0 flex-1 break-all font-mono text-xs leading-relaxed text-foreground">{{ runtimeCommand(runtime) }}</code>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                class="shrink-0"
                :aria-label="t('runtimes.copyCommand')"
                @click="copyCommand(runtimeCommand(runtime))"
              >
                <Copy class="size-4" />
              </Button>
            </div>
          </template>
          <div class="flex items-center gap-2">
            <span class="flex items-center gap-1.5 text-xs text-muted-foreground">
              <span
                class="size-1.5 rounded-full"
                :class="runtime.online ? 'bg-success' : 'bg-accent-gray-border'"
              />
              {{ runtime.online ? t('runtimes.status.online') : t('runtimes.status.offline') }}
            </span>
            <ConfirmPopover
              :title="t('runtimes.revokeTitle')"
              :message="t('runtimes.revokeDescription', { name: runtime.name })"
              :confirm-text="t('runtimes.revoke')"
              variant="destructive"
              @confirm="revokeRuntime(runtime)"
            >
              <template #trigger>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  :aria-label="t('runtimes.revoke')"
                >
                  <Trash2 class="size-4" />
                </Button>
              </template>
            </ConfirmPopover>
          </div>
        </SettingsRow>
      </SettingsSection>
    </div>
  </PageShell>

  <Dialog v-model:open="connectDialogOpen">
    <DialogContent>
      <form
        v-if="!createdCredential"
        @submit.prevent="createRuntimeCredential"
      >
        <DialogHeader>
          <DialogTitle>{{ t('runtimes.connectDialog.title') }}</DialogTitle>
          <DialogDescription>
            {{ t('runtimes.connectDialog.description') }}
          </DialogDescription>
        </DialogHeader>

        <FormStack class="mt-4">
          <FormField
            v-slot="{ componentField }"
            name="name"
          >
            <FieldStack
              :label="t('runtimes.connectDialog.name')"
              :help="t('runtimes.connectDialog.nameHelp')"
            >
              <FormControl>
                <Input
                  v-bind="componentField"
                  autofocus
                  autocomplete="off"
                  :placeholder="t('runtimes.connectDialog.namePlaceholder')"
                />
              </FormControl>
            </FieldStack>
          </FormField>
        </FormStack>

        <DialogFooter class="mt-4">
          <Button
            type="button"
            variant="outline"
            :disabled="creatingRuntime"
            @click="connectDialogOpen = false"
          >
            {{ t('common.cancel') }}
          </Button>
          <Button
            type="submit"
            :loading="creatingRuntime"
          >
            {{ t('runtimes.connectDialog.create') }}
          </Button>
        </DialogFooter>
      </form>

      <template v-else>
        <DialogHeader>
          <DialogTitle>{{ t('runtimes.connectDialog.runTitle') }}</DialogTitle>
          <DialogDescription>
            {{ t('runtimes.connectDialog.runDescription') }}
          </DialogDescription>
        </DialogHeader>

        <div class="mt-4 space-y-3">
          <div class="relative min-w-0 overflow-hidden rounded-md border border-border bg-muted-soft">
            <pre
              class="max-h-40 min-w-0 overflow-auto whitespace-pre-wrap break-all p-3 pr-12 font-mono text-xs leading-relaxed text-foreground"
              :aria-label="t('runtimes.connectDialog.command')"
            ><code>{{ connectCommand }}</code></pre>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              class="absolute right-2 top-2"
              :aria-label="t('common.copy')"
              @click="copyCommand(connectCommand)"
            >
              <Copy class="size-4" />
            </Button>
          </div>
          <p class="text-xs leading-relaxed text-muted-foreground">
            {{ t('runtimes.connectDialog.keyOnce') }}
          </p>
        </div>

        <DialogFooter class="mt-4">
          <Button @click="connectDialogOpen = false">
            {{ t('runtimes.connectDialog.done') }}
          </Button>
        </DialogFooter>
      </template>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useMutation, useQuery } from '@pinia/colada'
import {
  deleteUsersMeRuntimesById,
  getUsersMeRuntimes,
  postUsersMeRuntimes,
  type UserruntimeRuntime,
} from '@memohai/sdk'
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  FormControl,
  FormField,
  Input,
  toast,
} from '@felinic/ui'
import { Copy, Laptop, Plus, Trash2 } from 'lucide-vue-next'
import PageShell from '@/components/page-shell/index.vue'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import InlineLoadingRow from '@/components/inline-loading-row/index.vue'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import FieldStack from '@/components/settings/field-stack.vue'
import FormStack from '@/components/settings/form-stack.vue'
import { sdkApiBaseUrl } from '@/lib/api-client'
import { useClipboard } from '@/composables/useClipboard'
import { resolveApiErrorMessage } from '@/utils/api-error'

const { t } = useI18n()
const { copyText } = useClipboard()

const {
  data: runtimes,
  error: runtimesError,
  isLoading: runtimesLoading,
  refetch: refetchRuntimes,
} = useQuery({
  key: () => ['remote-runtimes'],
  query: async () => {
    const { data } = await getUsersMeRuntimes({ throwOnError: true })
    return data
  },
})

const runtimeItems = computed(() => runtimes.value ?? [])

const connectDialogOpen = ref(false)
const createdCredential = ref<UserruntimeRuntime | null>(null)

const connectSchema = toTypedSchema(z.object({
  name: z.string().trim().min(1, t('runtimes.connectDialog.nameRequired')),
}))
const connectForm = useForm({ validationSchema: connectSchema, initialValues: { name: '' } })

const { mutateAsync: createRuntime, isLoading: creatingRuntime } = useMutation({
  mutation: async (name: string) => {
    const { data } = await postUsersMeRuntimes({
      body: { name },
      throwOnError: true,
    })
    return data
  },
})

const { mutateAsync: revokeRuntimeCredential } = useMutation({
  mutation: async (runtimeID: string) => {
    await deleteUsersMeRuntimesById({ path: { id: runtimeID }, throwOnError: true })
  },
})

function commandForKey(value: string | undefined): string {
  const key = value?.trim()
  if (!key) return ''
  const server = sdkApiBaseUrl()
  const localFlag = isInsecureLocalhost(server) ? ' --insecure-localhost' : ''
  return `npx --yes @memohai/runtime --server ${server} --key ${key}${localFlag}`
}

const connectCommand = computed(() => commandForKey(createdCredential.value?.key))

function runtimeCommand(runtime: UserruntimeRuntime): string {
  return commandForKey(runtime.key)
}

const createRuntimeCredential = connectForm.handleSubmit(async (values) => {
  try {
    createdCredential.value = await createRuntime(values.name.trim())
    await refetchRuntimes()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('runtimes.connectDialog.createFailed')))
  }
})

async function copyCommand(command: string): Promise<void> {
  const copied = await copyText(command)
  if (copied) {
    toast.success(t('common.copied'))
  } else {
    toast.error(t('common.copyFailed'))
  }
}

async function revokeRuntime(runtime: UserruntimeRuntime): Promise<void> {
  if (!runtime.id) return
  try {
    await revokeRuntimeCredential(runtime.id)
    await refetchRuntimes()
    toast.success(t('runtimes.revokeSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('runtimes.revokeFailed')))
  }
}

function runtimeSummary(runtime: UserruntimeRuntime): string {
  if (!runtime.online) return t('runtimes.waitingForConnection')
  const machine = [runtime.hostname, runtime.os, runtime.arch].filter(Boolean).join(' · ')
  return machine || t('runtimes.connected')
}

function isInsecureLocalhost(server: string): boolean {
  const url = new URL(server)
  const hostname = url.hostname.replace(/^\[|\]$/g, '')
  return url.protocol === 'http:' && ['localhost', '127.0.0.1', '::1'].includes(hostname)
}

watch(connectDialogOpen, (open) => {
  if (open) return
  createdCredential.value = null
  connectForm.resetForm({ values: { name: '' } })
})

let pollTimer: number | undefined
function refreshVisibleRuntimes(): void {
  if (document.visibilityState === 'visible') void refetchRuntimes()
}

onMounted(() => {
  pollTimer = window.setInterval(refreshVisibleRuntimes, 5000)
  document.addEventListener('visibilitychange', refreshVisibleRuntimes)
})

onBeforeUnmount(() => {
  if (pollTimer !== undefined) window.clearInterval(pollTimer)
  document.removeEventListener('visibilitychange', refreshVisibleRuntimes)
})
</script>
