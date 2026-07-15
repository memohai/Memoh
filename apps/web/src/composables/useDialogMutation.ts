import { toast } from '@felinic/ui'
import { resolveApiErrorMessage } from '@/utils/api-error'

interface DialogMutationOptions<T> {
  fallbackMessage: string
  onSuccess?: (result: T) => void | Promise<void>
}

export function useDialogMutation() {
  async function run<T>(
    mutation: () => Promise<T>,
    options: DialogMutationOptions<T>,
  ): Promise<boolean> {
    try {
      const result = await mutation()
      await options.onSuccess?.(result)
      return true
    } catch (error) {
      toast.error(resolveApiErrorMessage(error, options.fallbackMessage, { prefixFallback: true }))
      return false
    }
  }

  return {
    run,
  }
}
