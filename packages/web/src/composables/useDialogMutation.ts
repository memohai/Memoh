import { toast } from 'vue-sonner'
import { resolveApiErrorMessage } from '@/utils/api-error'

interface DialogMutationOptions {
  fallbackMessage: string
  onSuccess?: () => void | Promise<void>
}

export function useDialogMutation() {
  async function run(
    mutation: () => Promise<unknown>,
    options: DialogMutationOptions,
  ): Promise<boolean> {
    try {
      await mutation()
      await options.onSuccess?.()
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
