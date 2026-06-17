<script setup lang="ts">
// Left anchor nav with scroll-spy. Observes each section element and
// highlights the one in view. Resilient to sections appearing/disappearing
// (e.g. the tokens legend toggle) — re-observes when the list changes.
import { onBeforeUnmount, ref, watch, nextTick } from 'vue'

const props = defineProps<{
  sections: { id: string; label: string }[]
}>()

const activeId = ref('')
let observer: IntersectionObserver | null = null

function setup() {
  observer?.disconnect()
  if (typeof IntersectionObserver === 'undefined') return
  observer = new IntersectionObserver(
    (entries) => {
      const visible = entries
        .filter((e) => e.isIntersecting)
        .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)[0]
      if (visible) activeId.value = visible.target.id
    },
    { rootMargin: '-72px 0px -65% 0px', threshold: 0 },
  )
  for (const s of props.sections) {
    const el = document.getElementById(s.id)
    if (el) observer.observe(el)
  }
}

function scrollTo(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

watch(
  () => props.sections.map((s) => s.id).join(','),
  () => nextTick(setup),
  { immediate: true },
)

onBeforeUnmount(() => observer?.disconnect())
</script>

<template>
  <nav class="flex flex-col gap-0.5">
    <button
      v-for="s in sections"
      :key="s.id"
      type="button"
      class="rounded-md px-2.5 py-1.5 text-left text-xs transition-colors hover:bg-accent"
      :class="activeId === s.id ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'"
      @click="scrollTo(s.id)"
    >
      {{ s.label }}
    </button>
  </nav>
</template>
