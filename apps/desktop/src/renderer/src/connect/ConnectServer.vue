<template>
  <main class="w-screen h-screen flex bg-background text-foreground p-4">
    <section class="m-auto w-full max-w-sm flex flex-col gap-8">
      <header class="space-y-2 text-center">
        <h1 class="text-3xl font-semibold tracking-wide">
          Memoh
        </h1>
        <p class="text-sm text-muted-foreground">
          Connect to your Memoh server.
        </p>
      </header>

      <form
        class="flex flex-col gap-4"
        @submit.prevent="save"
      >
        <div class="flex flex-col gap-2">
          <Label for="server-url">
            Server URL
          </Label>
          <Input
            id="server-url"
            v-model="baseUrl"
            type="text"
            inputmode="url"
            placeholder="https://memoh.example.com"
            autocomplete="url"
          />
        </div>

        <p
          v-if="error"
          class="text-sm text-destructive"
        >
          {{ error }}
        </p>

        <Button
          type="submit"
          class="w-full"
          :disabled="saving || !baseUrl.trim()"
        >
          <Spinner v-if="saving" />
          Connect
        </Button>
      </form>
    </section>
  </main>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Button, Input, Label, Spinner } from '@memohai/ui'

const baseUrl = ref('')
const error = ref('')
const saving = ref(false)

onMounted(async () => {
  const status = await window.api.desktop.getServerStatus()
  baseUrl.value = status.baseUrl
})

async function save() {
  if (saving.value) return
  error.value = ''
  saving.value = true
  try {
    await window.api.desktop.saveRemoteBaseUrl(baseUrl.value)
    localStorage.removeItem('token')
    window.location.reload()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    saving.value = false
  }
}
</script>
