<script setup lang="ts">
// Feedback: alerts, toasts, empty states. The global <Toaster> already lives
// in App.vue, so sonner just fires toast() — no extra mount here.
import {
  Alert, AlertDescription, AlertTitle,
  Button,
  Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle,
  Progress,
} from '@memohai/ui'
import { toast } from 'vue-sonner'
import { AlertCircle, Inbox, Terminal } from 'lucide-vue-next'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import SectionShell from '../components/SectionShell.vue'
import Specimen from '../components/Specimen.vue'
import VariantMatrix from '../components/VariantMatrix.vue'
import { variantSpecs } from '../lib/variant-specs'

function runPromiseToast() {
  toast.promise(
    new Promise((resolve) => setTimeout(resolve, 1500)),
    { loading: 'Saving...', success: 'Saved!', error: 'Failed' },
  )
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
          label="toast(...) — vue-sonner"
          note="global Toaster mounted in App.vue"
        >
          <Button
            variant="outline"
            size="sm"
            @click="toast('Event has been created')"
          >
            Default
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast.success('Saved successfully')"
          >
            Success
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast.error('Something went wrong')"
          >
            Error
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast.warning('Heads up')"
          >
            Warning
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast.info('For your information')"
          >
            Info
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="toast('With action', { description: 'Description text', action: { label: 'Undo', onClick: () => {} } })"
          >
            With action
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="runPromiseToast"
          >
            Promise
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
