import { describe, expect, it, vi } from 'vitest'

vi.mock('@felinic/ui', () => ({
  toast: { error: vi.fn() },
}))

vi.mock('@/utils/api-error', () => ({
  resolveApiErrorMessage: vi.fn(() => 'resolved error'),
}))

import { useDialogMutation } from './useDialogMutation'

describe('useDialogMutation', () => {
  it('passes the mutation result to the success callback', async () => {
    const onSuccess = vi.fn()
    const { run } = useDialogMutation()

    await expect(run(
      async () => ({ created: 2, updated: 1 }),
      { fallbackMessage: 'failed', onSuccess },
    )).resolves.toBe(true)

    expect(onSuccess).toHaveBeenCalledWith({ created: 2, updated: 1 })
  })
})
