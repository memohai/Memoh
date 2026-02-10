import { ref } from 'vue'

/**
 * TagsInput key:value 双向转换逻辑。
 * 用于 add-platform 的 config 和 create-mcp 的 env 字段。
 *
 * 输入格式："key:value" 字符串数组
 * 输出格式：{ key: value } 对象
 */
export function useKeyValueTags() {
  const tagList = ref<string[]>([])

  /** 验证标签格式：必须是 key:value */
  function convertValue(tagStr: string): string {
    return /^\w+:\w+$/.test(tagStr) ? tagStr : ''
  }

  /** 标签更新时，过滤无效值并转换为对象，通过回调输出 */
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

  /** 从对象初始化标签列表 */
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
