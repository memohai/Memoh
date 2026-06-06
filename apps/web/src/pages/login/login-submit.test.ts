import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { submitLogin, type SubmitLoginDependencies } from './login-submit'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function createDependencies(overrides: Partial<SubmitLoginDependencies> = {}): SubmitLoginDependencies {
  return {
    authenticate: vi.fn().mockResolvedValue({
      data: {
        access_token: 'token',
        user_id: 'user-1',
        username: 'alice',
        display_name: 'Alice',
        role: 'admin',
        avatar_url: '',
        timezone: 'UTC',
      },
    }),
    applyLogin: vi.fn(),
    navigateHome: vi.fn().mockResolvedValue(undefined),
    notifyInvalidCredentials: vi.fn(),
    ...overrides,
  }
}

describe('submitLogin', () => {
  it('ignores a submission while another login is already in progress', async () => {
    const isSubmitting = ref(true)
    const dependencies = createDependencies()

    await expect(submitLogin({ username: 'alice', password: 'secret' }, isSubmitting, dependencies)).resolves.toBe(false)

    expect(dependencies.authenticate).not.toHaveBeenCalled()
    expect(dependencies.applyLogin).not.toHaveBeenCalled()
    expect(isSubmitting.value).toBe(true)
  })

  it('keeps the form busy until the successful navigation settles', async () => {
    const isSubmitting = ref(false)
    const navigation = deferred<void>()
    const dependencies = createDependencies({
      navigateHome: vi.fn(() => navigation.promise),
    })

    const firstSubmission = submitLogin({ username: 'alice', password: 'secret' }, isSubmitting, dependencies)
    await Promise.resolve()

    expect(isSubmitting.value).toBe(true)
    expect(dependencies.authenticate).toHaveBeenCalledTimes(1)
    expect(dependencies.applyLogin).toHaveBeenCalledTimes(1)
    expect(dependencies.navigateHome).toHaveBeenCalledTimes(1)

    await expect(submitLogin({ username: 'alice', password: 'secret' }, isSubmitting, dependencies)).resolves.toBe(false)
    expect(dependencies.authenticate).toHaveBeenCalledTimes(1)

    navigation.resolve()
    await expect(firstSubmission).resolves.toBe(true)
    expect(isSubmitting.value).toBe(false)
  })

  it('notifies and releases the form when authentication fails', async () => {
    const isSubmitting = ref(false)
    const dependencies = createDependencies({
      authenticate: vi.fn().mockResolvedValue({ data: null }),
    })

    await expect(submitLogin({ username: 'alice', password: 'bad' }, isSubmitting, dependencies)).resolves.toBe(false)

    expect(dependencies.applyLogin).not.toHaveBeenCalled()
    expect(dependencies.navigateHome).not.toHaveBeenCalled()
    expect(dependencies.notifyInvalidCredentials).toHaveBeenCalledTimes(1)
    expect(isSubmitting.value).toBe(false)
  })
})
