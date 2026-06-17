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
      // replace, not push: this syncs in-page state (tabs, filters) into the URL,
      // which is not a navigation, so it shouldn't pile entries onto the history
      // stack. The back affordance is guarded independently — installBackHistory
      // ignores same-path transitions — so back stays correct regardless; replace
      // also keeps the browser's own back button honest.
      void router.replace({ query: { ...route.query, [key]: value } })
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
