<template>
  <main class="min-h-screen bg-background text-foreground p-6">
    <section class="mx-auto flex min-h-[calc(100vh-48px)] w-full max-w-md flex-col justify-center gap-6">
      <header class="space-y-1.5">
        <h1 class="text-xl font-semibold leading-7">
          Memoh Settings
        </h1>
        <p class="text-sm text-muted-foreground">
          Choose which Memoh server this app connects to.
        </p>
      </header>

      <form
        class="space-y-5"
        @submit.prevent="save"
      >
        <div class="space-y-2">
          <Label for="remote-server">
            Server address
          </Label>
          <Input
            id="remote-server"
            v-model="serverUrl"
            type="text"
            inputmode="url"
            autocomplete="url"
            placeholder="https://memoh.example.com"
            :disabled="loading || saving"
          />
          <p class="text-xs text-muted-foreground">
            Current server: <span class="font-mono">{{ currentServerUrl || 'Not configured' }}</span>
          </p>
        </div>

        <p
          v-if="error"
          class="text-sm text-destructive"
        >
          {{ error }}
        </p>
        <p
          v-else-if="saved"
          class="text-sm text-muted-foreground"
        >
          Saved.
        </p>

        <div class="flex flex-wrap items-center gap-2">
          <Button
            type="submit"
            :disabled="!canSave"
          >
            <Spinner v-if="saving" />
            <Save
              v-else
              class="size-4"
            />
            Save
          </Button>
          <Button
            type="button"
            variant="outline"
            :disabled="loading || saving || serverUrl === currentServerUrl"
            @click="reset"
          >
            <RotateCcw class="size-4" />
            Reset
          </Button>
        </div>
      </form>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RotateCcw, Save } from 'lucide-vue-next'
import { Button, Input, Label, Spinner } from '@memohai/ui'

const loading = ref(true)
const saving = ref(false)
const saved = ref(false)
const error = ref('')
const mode = ref<'local' | 'remote'>('remote')
const serverUrl = ref('')
const currentServerUrl = ref('')

const canSave = computed(() =>
  mode.value === 'remote'
  && !loading.value
  && !saving.value
  && serverUrl.value.trim().length > 0
  && serverUrl.value !== currentServerUrl.value,
)

onMounted(async () => {
  try {
    const status = await window.api.desktop.getServerStatus()
    mode.value = status.mode
    serverUrl.value = status.baseUrl
    currentServerUrl.value = status.baseUrl
    if (status.mode !== 'remote') {
      error.value = 'Server connection can only be changed in Memoh.'
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
})

function reset() {
  serverUrl.value = currentServerUrl.value
  error.value = ''
  saved.value = false
}

async function save() {
  if (!canSave.value) return
  error.value = ''
  saved.value = false
  saving.value = true
  try {
    const status = await window.api.desktop.saveRemoteBaseUrl(serverUrl.value)
    serverUrl.value = status.baseUrl
    currentServerUrl.value = status.baseUrl
    saved.value = true
    if (status.changed) {
      await window.api.window.closeSelf()
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    saving.value = false
  }
}
</script>
