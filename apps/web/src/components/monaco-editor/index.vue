<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch } from 'vue'
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

const containerRef = ref<HTMLDivElement>()
const settings = useSettingsStore()

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
  minimap: { enabled: false },
  scrollBeyondLastLine: true,
  fontSize: 13,
  lineNumbers: 'on',
  renderLineHighlight: 'line',
  tabSize: 2,
  wordWrap: 'on',
  padding: { top: 8, bottom: 8 },
})

onMounted(async () => {
  if (!containerRef.value) return

  await createEditor(containerRef.value, props.modelValue, resolveLanguage())

  const editor = getEditorView()
  editor?.onDidChangeModelContent(() => {
    const value = editor.getValue() ?? ''
    emit('update:modelValue', value)
  })
})

onBeforeUnmount(() => {
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
</script>

<template>
  <div
    ref="containerRef"
    class="h-full w-full overflow-hidden"
  />
</template>
