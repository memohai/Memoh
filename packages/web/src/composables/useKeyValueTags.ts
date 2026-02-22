import { ref } from 'vue'
import { recordToTags, tagsToRecord } from '@/utils/key-value-tags'

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
    const obj = tagsToRecord(tagList.value)
    onUpdate?.(obj)
  }

  function initFromObject(obj: Record<string, string> | undefined | null) {
    tagList.value = recordToTags(obj)
  }

  return {
    tagList,
    convertValue,
    handleUpdate,
    initFromObject,
  }
}
