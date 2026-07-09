<script setup lang="ts">
// Feedback: alerts, toasts, empty states. The global <Toaster> already lives
// in App.vue, so this just fires toast() — no extra mount here.
import {
  Alert, AlertDescription, AlertTitle,
  Button,
  Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle,
  Progress,
} from '@felinic/ui'
import { toast } from '@felinic/ui'
import { AlertCircle, Inbox, Terminal } from 'lucide-vue-next'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import SectionShell from '../components/SectionShell.vue'
import Specimen from '../components/Specimen.vue'
import VariantMatrix from '../components/VariantMatrix.vue'
import { variantSpecs } from '../lib/variant-specs'

// Fire a burst so the stack reads as a quiet, fully-readable column — newest on
// top, every card's content visible (no depth stacking that hides prior messages).
function runStackDemo() {
  const fns = [
    () => toast.success('Bot saved'),
    () => toast.info('Model synced'),
    () => toast.warning('Channel reconnecting…'),
    () => toast('Workspace synced'),
  ]
  fns.forEach((fn, i) => setTimeout(fn, i * 220))
}

// Live bar — mimics the container image pull / create SSE progress the server
// streams, looping 0 → 100 so the transition reads at a glance.
const liveProgress = ref(8)
let progressTimer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  progressTimer = setInterval(() => {
    // Land exactly on 100 (clamp the last step), then loop back to 0 — reka's
    // ProgressRoot rejects any value above max (100).
    liveProgress.value = liveProgress.value >= 100 ? 0 : Math.min(100, liveProgress.value + 12)
  }, 900)
})
onBeforeUnmount(() => clearInterval(progressTimer))
</script>

<template>
  <SectionShell
    id="feedback"
    label="Feedback"
    description="Alerts, toast notifications, and empty states."
  >
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <div class="lg:col-span-2">
        <Specimen label="<Alert :variant>">
          <VariantMatrix
            :variants="variantSpecs.alert.variants"
            class="w-full"
          >
            <template #default="{ variant }">
              <Alert
                :variant="variant"
                class="max-w-md"
              >
                <AlertCircle v-if="variant === 'destructive'" />
                <Terminal v-else />
                <AlertTitle>{{ variant }} alert</AlertTitle>
                <AlertDescription>This is an alert with the {{ variant }} variant.</AlertDescription>
              </Alert>
            </template>
          </VariantMatrix>
        </Specimen>
      </div>

      <div class="lg:col-span-2">
        <Specimen
          label="toast(...) — variants"
          note="real global Toaster (App.vue, top-right) · semantic color on the icon only · skin in style.css"
        >
          <Button
            variant="outline"
            size="sm"
            @click="toast('Workspace synced')"
          >
            Default
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast.success('Bot saved', { description: 'Your changes are live.' })"
          >
            Success
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast.warning('Reconnecting…', { description: 'The channel dropped and is retrying.' })"
          >
            Warning
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast.error('Save failed', { description: 'Check your network and try again.' })"
          >
            Error
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast.info('New model available', { description: 'gpt-5.5 is ready to use.' })"
          >
            Info
          </Button>
        </Specimen>
      </div>

      <div class="lg:col-span-2">
        <Specimen
          label="toast(...) — long content auto-shaping"
          note="a long / unbreakable, titleless blob (raw backend error, path, URL, connection string) is auto-shaped into a variant heading + gray description (like 'Upload failed') instead of a bold wall of title text. Short titles stay single-line."
        >
          <Button
            variant="outline"
            size="sm"
            @click="toast.error('not found: readdir: open /Users/qqqqqf/.memoh/workspaces/1/documents/项目: no such file or directory')"
          >
            Raw backend error
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast('https://example.com/api/v1/workspaces/1/documents/very/deeply/nested/path/that/never/breaks/file.tar.gz?token=abcdefghijklmnopqrstuvwxyz0123456789')"
          >
            Unbreakable URL
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast.error('Upload failed', { description: 'Could not write /Users/qqqqqf/.memoh/workspaces/1/documents/项目/long-unbreakable-filename-without-spaces-1234567890.bin — the destination directory does not exist.' })"
          >
            Long title + desc
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast.warning('connection_string=postgresql://memoh_user:supersecretpassword@db.internal.memoh.example.com:5432/memoh_production?sslmode=require', { action: { label: 'Copy', onClick: () => {} } })"
          >
            Long blob + action
          </Button>
        </Specimen>
      </div>

      <div class="lg:col-span-2">
        <Specimen
          label="toast(...) — rich behaviors"
          note="action · single-line (Retry) · long (10s) · Stack fires 4 (full column) · dismiss all"
        >
          <Button
            variant="outline"
            size="sm"
            @click="toast('Message archived', { action: { label: 'Undo', onClick: () => {} } })"
          >
            With action
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast('Workspace sync paused', { action: { label: 'Retry', onClick: () => {} } })"
          >
            Single-line action
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast.warning('MCP pencil: Server disconnected.', { description: 'For troubleshooting guidance, please visit our debugging documentation.', action: { label: 'Open developer settings', onClick: () => {} } })"
          >
            Long text + Action
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast('Heads up', { duration: 10000 })"
          >
            Long (10s)
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="runStackDemo"
          >
            Stack ×4
          </Button>
          <Button
            variant="ghost"
            size="sm"
            @click="toast.dismiss()"
          >
            Dismiss all
          </Button>
        </Specimen>
      </div>

      <div class="lg:col-span-2">
        <Specimen
          label="<Progress>"
          note="neutral rail + selection-blue fill — container pull / upload progress"
        >
          <div class="flex w-full max-w-md flex-col gap-4">
            <Progress :model-value="30" />
            <Progress :model-value="66" />
            <Progress :model-value="100" />
            <Progress :model-value="liveProgress" />
          </div>
        </Specimen>
      </div>

      <div class="lg:col-span-2">
        <Specimen label="<Empty>">
          <Empty class="w-full max-w-sm">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Inbox />
              </EmptyMedia>
              <EmptyTitle>No messages</EmptyTitle>
              <EmptyDescription>You're all caught up. New messages will appear here.</EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button
                variant="primary"
                size="sm"
              >
                Refresh
              </Button>
            </EmptyContent>
          </Empty>
        </Specimen>
      </div>
    </div>
  </SectionShell>
</template>
