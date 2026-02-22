import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

function readQueryValue(value: unknown): string | null {
  if (typeof value === 'string' && value) {
    return value
  }
  if (Array.isArray(value) && typeof value[0] === 'string' && value[0]) {
    return value[0]
  }
  return null
}

export function useSyncedQueryParam(key: string, defaultValue: string) {
  const route = useRoute()
  const router = useRouter()
  const model = ref(readQueryValue(route.query[key]) ?? defaultValue)

  watch(model, (value) => {
    if (value !== route.query[key]) {
      void router.push({ query: { ...route.query, [key]: value } })
    }
  })

  watch(() => route.query[key], (value) => {
    const queryValue = readQueryValue(value)
    if (queryValue && queryValue !== model.value) {
      model.value = queryValue
    }
  })

  return model
}
