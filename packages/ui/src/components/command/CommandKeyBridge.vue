<script setup lang="ts">
// Headless adapter that lets a text input OUTSIDE the Command tree drive the
// listbox keyboard interaction — the same mechanism reka's ListboxFilter uses
// for an input inside it. While mounted, the highlight is virtual (reka's
// focusable=false mode), so arrow navigation never steals focus from the
// host's own input; the host forwards ArrowUp/ArrowDown to `navigate` and a
// plain Enter to `select` (calling preventDefault itself, mirroring
// ListboxFilter's withModifiers(['prevent']) bindings).
//
// Built for composer-style surfaces — e.g. a chat input with a slash picker
// or command-result list floating above it — where moving focus out of the
// textarea would break typing flow.
import { injectListboxRootContext } from 'reka-ui'
import { computed, onBeforeUnmount, onMounted } from 'vue'

const rootContext = injectListboxRootContext()

onMounted(() => {
  rootContext.focusable.value = false
})
onBeforeUnmount(() => {
  rootContext.focusable.value = true
})

const hasHighlight = computed(() => !!rootContext.highlightedElement.value)

function navigate(event: KeyboardEvent) {
  rootContext.onKeydownNavigation(event)
}

function select(event: KeyboardEvent) {
  rootContext.onKeydownEnter(event)
}

defineExpose({ hasHighlight, navigate, select })
</script>

<template>
  <slot />
</template>
