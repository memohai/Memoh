<script setup lang="ts">
import { computed, onMounted, nextTick, ref, watch } from 'vue'
import { useSettingsStore } from '@/store/settings'

const props = withDefaults(defineProps<{
  content: string
  class?: string
}>(), {
  class: undefined,
})

const settings = useSettingsStore()
const themeStyle = ref('')

// Resolve the current themed defaults at runtime so the iframe's blank canvas
// inherits the app theme (foreground/background) and gets the correct
// `color-scheme` for native scrollbars / form controls.
// User-authored CSS inside the HTML still wins because it appears later in
// the document source order (equal specificity → later rule applies).
function refreshThemeStyle() {
  if (typeof document === 'undefined') return
  const cs = getComputedStyle(document.documentElement)
  const fg = cs.getPropertyValue('--color-foreground').trim()
  // Match the editor plane so the preview canvas is continuous with its tab.
  const bg = cs.getPropertyValue('--color-surface-editor').trim()
  const muted = cs.getPropertyValue('--color-muted-foreground').trim()
  const border = cs.getPropertyValue('--color-border').trim()
  const accent = cs.getPropertyValue('--color-accent').trim()
  const isDark = settings.theme === 'dark'

  themeStyle.value = `<style>
:root { color-scheme: ${isDark ? 'dark' : 'light'}; }
html, body {
  color: ${fg || 'inherit'};
  background-color: ${bg || 'transparent'};
  font-family: ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
  font-size: 14px;
  line-height: 1.6;
}
body { margin: 16px; }
a { color: ${fg || 'inherit'}; }
hr { border-color: ${border || 'currentColor'}; }
code, pre { background-color: ${accent || 'transparent'}; color: ${fg || 'inherit'}; }
blockquote { color: ${muted || 'inherit'}; border-left: 3px solid ${border || 'currentColor'}; padding-left: 12px; margin-left: 0; }
::selection { background-color: ${accent || 'rgba(127,127,127,0.3)'}; }
</style>`
}

onMounted(() => refreshThemeStyle())

watch(() => settings.theme, async () => {
  await nextTick()
  refreshThemeStyle()
})

const srcdoc = computed(() => themeStyle.value + (props.content ?? ''))
</script>

<template>
  <div :class="['flex h-full w-full min-h-0 bg-surface-editor', props.class]">
    <iframe
      :srcdoc="srcdoc"
      sandbox="allow-same-origin"
      class="h-full w-full border-0"
      title="HTML preview"
      referrerpolicy="no-referrer"
    />
  </div>
</template>
