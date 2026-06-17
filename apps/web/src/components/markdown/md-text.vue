<script setup lang="ts">
import { computed, inject, type Ref } from 'vue'
import { splitScriptRuns } from '@/utils/script-runs'

// Custom markstream `text` node: split mixed CJK/Latin into per-script spans so
// each takes its own weight (.chat-cjk / .chat-latin in style.css). We render the
// SAME split WHILE STREAMING too, so the weight is correct from the first frame
// and never "pops" between streaming and settled. The only thing given up vs.
// markstream's own TextNode is its per-delta opacity fade; the typewriter cadence
// still comes through. markstream routes code / math / inline-code / emoji through
// their own node types, so this only ever sees plain prose text.
defineOptions({ inheritAttrs: false })

const props = defineProps<{
  node: { type: 'text', content: string, raw: string, center?: boolean }
}>()

// markstream mutates `node.content` in place (same object reference) on every
// stream tick, so reading it alone never re-triggers reactivity. It also bumps an
// injected version ref each tick to drive its own TextNode updates; depend on that
// same signal so our split re-runs as the content grows — otherwise the whole
// block only appears once at settle (no streaming). Key is markstream-internal.
const streamVersion = inject<Ref<number> | undefined>('markstreamStreamVersion', undefined)

const runs = computed(() => {
  void streamVersion?.value
  return splitScriptRuns(props.node.content)
})
</script>

<template>
  <span
    class="text-node"
    :class="node.center ? 'text-node-center' : ''"
  ><span
    v-for="(run, i) in runs"
    :key="i"
    :class="run.script === 'cjk' ? 'chat-cjk' : 'chat-latin'"
  >{{ run.text }}</span></span>
</template>
