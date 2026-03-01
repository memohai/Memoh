<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, shallowRef } from 'vue'
import * as monaco from 'monaco-editor'
import { useSettingsStore } from '@/store/settings'
import { getLanguageByFilename } from '@/components/file-manager/utils'

self.MonacoEnvironment = {
  getWorker() {
    return new Worker(
      new URL('monaco-editor/esm/vs/editor/editor.worker.js', import.meta.url),
      { type: 'module' },
    )
  },
}

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
const editorInstance = shallowRef<monaco.editor.IStandaloneCodeEditor>()
const settings = useSettingsStore()

function resolveLanguage(): string {
  if (props.language) return props.language
  if (props.filename) return getLanguageByFilename(props.filename)
  return 'plaintext'
}

function resolveTheme(): string {
  return settings.theme === 'dark' ? 'vs-dark' : 'vs'
}

onMounted(() => {
  if (!containerRef.value) return

  editorInstance.value = monaco.editor.create(containerRef.value, {
    value: props.modelValue,
    language: resolveLanguage(),
    theme: resolveTheme(),
    readOnly: props.readonly,
    automaticLayout: true,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    fontSize: 13,
    lineNumbers: 'on',
    renderLineHighlight: 'line',
    tabSize: 2,
    wordWrap: 'on',
    padding: { top: 8, bottom: 8 },
  })

  editorInstance.value.onDidChangeModelContent(() => {
    const value = editorInstance.value?.getValue() ?? ''
    emit('update:modelValue', value)
  })
})

onBeforeUnmount(() => {
  editorInstance.value?.dispose()
})

watch(() => props.modelValue, (newVal) => {
  const editor = editorInstance.value
  if (!editor) return
  if (editor.getValue() !== newVal) {
    editor.setValue(newVal)
  }
})

watch(() => props.readonly, (val) => {
  editorInstance.value?.updateOptions({ readOnly: val })
})

watch([() => props.language, () => props.filename], () => {
  const model = editorInstance.value?.getModel()
  if (model) {
    monaco.editor.setModelLanguage(model, resolveLanguage())
  }
})

watch(() => settings.theme, () => {
  monaco.editor.setTheme(resolveTheme())
})
</script>

<template>
  <div
    ref="containerRef"
    class="h-full w-full overflow-hidden"
  />
</template>
