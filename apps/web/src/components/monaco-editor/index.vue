<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount, watch } from 'vue'
import { useMonaco } from 'stream-monaco'
import { useSettingsStore } from '@/store/settings'
import { getLanguageByFilename } from '@/components/file-manager/utils'

const props = withDefaults(defineProps<{
  modelValue: string
  language?: string
  readonly?: boolean
  filename?: string
}>(), {
  language: undefined,
  readonly: false,
  filename: undefined,
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const editorRef = ref<HTMLDivElement>()
const settings = useSettingsStore()
let observer: MutationObserver | null = null
const editorFontSize = computed(() => settings.codeFontSizePx)
// Keep Monaco's built-in platform default font unless the user explicitly
// customizes the code font; `undefined` resets to the editor default.
const editorFontFamily = computed(() => settings.codeFontFamily ? settings.codeFontStack : undefined)

function resolveLanguage(): string {
  if (props.language) return props.language
  if (props.filename) return getLanguageByFilename(props.filename)
  return 'plaintext'
}

function resolveTheme(): string {
  return settings.theme === 'dark' ? 'vitesse-dark' : 'vitesse-light'
}

const {
  createEditor,
  cleanupEditor,
  updateCode,
  setTheme,
  setLanguage,
  getEditorView,
} = useMonaco({
  theme: resolveTheme(),
  themes: ['vitesse-dark', 'vitesse-light'],
  readOnly: props.readonly,
  automaticLayout: true,
  autoScrollInitial: false,
  autoScrollOnUpdate: false,
  minimap: { enabled: false },
  scrollBeyondLastLine: true,
  fontSize: editorFontSize.value,
  fontFamily: editorFontFamily.value,
  lineNumbers: 'on',
  renderLineHighlight: 'line',
  tabSize: 2,
  wordWrap: 'on',
  padding: { top: 8, bottom: 8 },
})

function clearInlineHeightStyles(el: HTMLElement) {
  let changed = false
  for (const prop of ['height', 'max-height', 'min-height', 'overflow'] as const) {
    if (el.style.getPropertyValue(prop)) {
      el.style.removeProperty(prop)
      changed = true
    }
  }
  return changed
}

onMounted(async () => {
  if (!editorRef.value) return

  await createEditor(editorRef.value, props.modelValue, resolveLanguage())

  clearInlineHeightStyles(editorRef.value)

  observer = new MutationObserver(() => {
    if (editorRef.value) clearInlineHeightStyles(editorRef.value)
  })
  observer.observe(editorRef.value, { attributes: true, attributeFilter: ['style'] })

  const editor = getEditorView()
  if (editor) {
    editor.updateOptions({
      fontSize: editorFontSize.value,
      fontFamily: editorFontFamily.value,
    })
    editor.setPosition({ lineNumber: 1, column: 1 })
    editor.revealLine(1)
  }
  editor?.onDidChangeModelContent(() => {
    const value = editor.getValue() ?? ''
    emit('update:modelValue', value)
  })
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
  cleanupEditor()
})

watch(() => props.modelValue, (newVal) => {
  const editor = getEditorView()
  if (!editor) return
  if (editor.getValue() !== newVal) {
    updateCode(newVal, resolveLanguage())
  }
})

watch(() => props.readonly, (val) => {
  getEditorView()?.updateOptions({ readOnly: val })
})

watch([() => props.language, () => props.filename], () => {
  setLanguage(resolveLanguage())
})

watch(() => settings.theme, () => {
  setTheme(resolveTheme())
})

watch(editorFontSize, (fontSize) => {
  getEditorView()?.updateOptions({ fontSize })
})

watch(editorFontFamily, (fontFamily) => {
  getEditorView()?.updateOptions({ fontFamily })
})
</script>

<template>
  <div
    ref="editorRef"
    class="h-full w-full overflow-hidden"
  />
</template>
