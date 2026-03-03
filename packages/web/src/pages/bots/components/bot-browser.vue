<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { Button, Card, CardContent, CardHeader, CardTitle, Input, Label, Spinner, Textarea, Badge } from '@memoh/ui'
import { client } from '@memoh/sdk/client'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  botId: string
}>()

type BrowserSession = {
  session_id: string
  status: string
  current_url?: string
  action_count?: number
  expires_at?: string
  remote_session_id?: string
}

const { t } = useI18n()
const loading = ref(false)
const creating = ref(false)
const actionLoading = ref(false)
const closing = ref(false)
const sessions = ref<BrowserSession[]>([])
const selectedSessionId = ref('')
const actionName = ref<'goto' | 'click' | 'type' | 'screenshot' | 'extract_text'>('goto')
const actionUrl = ref('')
const actionTarget = ref('')
const actionValue = ref('')
const actionResult = ref('')

const activeSessions = computed(() => sessions.value.filter(item => item.status === 'active'))

function resolveErrorMessage(error: unknown, fallback: string): string {
  return resolveApiErrorMessage(error, fallback)
}

async function loadSessions(showToast = false) {
  if (!props.botId) return
  loading.value = true
  try {
    const { data } = await client.get<{ items: BrowserSession[] }>({
      url: `/bots/${props.botId}/browser/sessions`,
      throwOnError: true,
    })
    sessions.value = data?.items ?? []
    if (!selectedSessionId.value && activeSessions.value.length > 0) {
      selectedSessionId.value = activeSessions.value[0].session_id
    }
  } catch (error) {
    if (showToast) {
      toast.error(resolveErrorMessage(error, t('bots.browser.loadFailed')))
    }
  } finally {
    loading.value = false
  }
}

async function handleCreateSession() {
  creating.value = true
  try {
    const { data } = await client.post<BrowserSession>({
      url: `/bots/${props.botId}/browser/sessions`,
      body: {},
      throwOnError: true,
    })
    if (data?.session_id) {
      selectedSessionId.value = data.session_id
    }
    await loadSessions(false)
    toast.success(t('bots.browser.createSuccess'))
  } catch (error) {
    toast.error(resolveErrorMessage(error, t('bots.browser.createFailed')))
  } finally {
    creating.value = false
  }
}

async function handleCloseSession(sessionId: string) {
  if (!sessionId) return
  closing.value = true
  try {
    await client.delete({
      url: `/bots/${props.botId}/browser/sessions/${sessionId}`,
      throwOnError: true,
    })
    if (selectedSessionId.value === sessionId) {
      selectedSessionId.value = ''
    }
    await loadSessions(false)
    toast.success(t('bots.browser.closeSuccess'))
  } catch (error) {
    toast.error(resolveErrorMessage(error, t('bots.browser.closeFailed')))
  } finally {
    closing.value = false
  }
}

async function handleExecuteAction() {
  if (!selectedSessionId.value) {
    toast.error(t('bots.browser.selectSession'))
    return
  }
  actionLoading.value = true
  try {
    const { data } = await client.post<Record<string, unknown>>({
      url: `/bots/${props.botId}/browser/sessions/${selectedSessionId.value}/actions`,
      body: {
        name: actionName.value,
        url: actionUrl.value.trim() || undefined,
        target: actionTarget.value.trim() || undefined,
        value: actionValue.value || undefined,
      },
      throwOnError: true,
    })
    actionResult.value = JSON.stringify(data ?? {}, null, 2)
    await loadSessions(false)
  } catch (error) {
    toast.error(resolveErrorMessage(error, t('bots.browser.actionFailed')))
  } finally {
    actionLoading.value = false
  }
}

onMounted(() => {
  void loadSessions(true)
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between">
      <div>
        <h3 class="text-lg font-medium">
          {{ $t('bots.browser.title') }}
        </h3>
        <p class="text-sm text-muted-foreground">
          {{ $t('bots.browser.subtitle') }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          :disabled="loading || creating"
          @click="loadSessions(true)"
        >
          <Spinner
            v-if="loading"
            class="mr-1.5"
          />
          {{ $t('common.refresh') }}
        </Button>
        <Button
          size="sm"
          :disabled="creating"
          @click="handleCreateSession"
        >
          <Spinner
            v-if="creating"
            class="mr-1.5"
          />
          {{ $t('bots.browser.createSession') }}
        </Button>
      </div>
    </div>

    <Card>
      <CardHeader>
        <CardTitle class="text-base">
          {{ $t('bots.browser.sessions') }}
        </CardTitle>
      </CardHeader>
      <CardContent class="space-y-2">
        <div
          v-if="sessions.length === 0"
          class="text-sm text-muted-foreground"
        >
          {{ $t('bots.browser.empty') }}
        </div>
        <div
          v-for="item in sessions"
          :key="item.session_id"
          class="flex items-center justify-between gap-3 rounded-md border p-2"
        >
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="font-mono text-xs hover:underline"
                @click="selectedSessionId = item.session_id"
              >
                {{ item.session_id }}
              </button>
              <Badge variant="outline">
                {{ item.status }}
              </Badge>
            </div>
            <p class="truncate text-xs text-muted-foreground">
              {{ item.current_url || '-' }}
            </p>
          </div>
          <Button
            variant="destructive"
            size="sm"
            :disabled="closing || item.status !== 'active'"
            @click="handleCloseSession(item.session_id)"
          >
            {{ $t('bots.browser.closeSession') }}
          </Button>
        </div>
      </CardContent>
    </Card>

    <Card>
      <CardHeader>
        <CardTitle class="text-base">
          {{ $t('bots.browser.actions') }}
        </CardTitle>
      </CardHeader>
      <CardContent class="space-y-3">
        <div class="grid gap-3 md:grid-cols-2">
          <div class="space-y-1">
            <Label>{{ $t('bots.browser.selectedSession') }}</Label>
            <Input
              v-model="selectedSessionId"
              :placeholder="$t('bots.browser.selectSessionPlaceholder')"
            />
          </div>
          <div class="space-y-1">
            <Label>{{ $t('bots.browser.actionName') }}</Label>
            <select
              v-model="actionName"
              class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            >
              <option value="goto">
                goto
              </option>
              <option value="click">
                click
              </option>
              <option value="type">
                type
              </option>
              <option value="screenshot">
                screenshot
              </option>
              <option value="extract_text">
                extract_text
              </option>
            </select>
          </div>
          <div class="space-y-1 md:col-span-2">
            <Label>URL</Label>
            <Input
              v-model="actionUrl"
              placeholder="https://example.com"
            />
          </div>
          <div class="space-y-1">
            <Label>Target</Label>
            <Input
              v-model="actionTarget"
              placeholder="#selector"
            />
          </div>
          <div class="space-y-1">
            <Label>Value</Label>
            <Input
              v-model="actionValue"
              placeholder="input text"
            />
          </div>
        </div>
        <Button
          :disabled="actionLoading || activeSessions.length === 0"
          @click="handleExecuteAction"
        >
          <Spinner
            v-if="actionLoading"
            class="mr-1.5"
          />
          {{ $t('bots.browser.execute') }}
        </Button>
        <div class="space-y-1">
          <Label>{{ $t('bots.browser.result') }}</Label>
          <Textarea
            v-model="actionResult"
            class="min-h-[220px] font-mono text-xs"
            readonly
          />
        </div>
      </CardContent>
    </Card>
  </div>
</template>
