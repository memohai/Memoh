import { ref } from 'vue'

/**
 * TagsInput key:value two-way conversion for platform config and MCP env/headers.
 * Input: string[] of "key:value"; output: Record<string, string> via callback.
 */
export function useKeyValueTags() {
  const tagList = ref<string[]>([])

  function convertValue(tagStr: string): string {
    return /^\w+:\w+$/.test(tagStr) ? tagStr : ''
  }

  function handleUpdate(tags: string[], onUpdate?: (obj: Record<string, string>) => void) {
    tagList.value = tags.filter(Boolean) as string[]
    const obj: Record<string, string> = {}
    tagList.value.forEach((item) => {
      const [key, value] = item.split(':')
      if (key && value) {
        obj[key] = value
      }
    })
    onUpdate?.(obj)
  }

  function initFromObject(obj: Record<string, string> | undefined | null) {
    if (!obj) {
      tagList.value = []
      return
    }
    tagList.value = Object.entries(obj).map(([k, v]) => `${k}:${v}`)
  }

  return {
    tagList,
    convertValue,
    handleUpdate,
    initFromObject,
  }
}
