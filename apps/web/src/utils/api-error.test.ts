import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resolveApiErrorMessage } from '@/utils/api-error'

describe('resolveApiErrorMessage', () => {
  let locale = 'en'

  beforeEach(() => {
    locale = 'en'
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => key === 'language' ? locale : null,
      setItem: vi.fn(),
      removeItem: vi.fn(),
      clear: vi.fn(),
    })
  })

  it('renders ACP feedback i18n keys before raw backend messages', () => {
    locale = 'zh'

    const message = resolveApiErrorMessage({
      body: {
        code: 'no_workspace_exec',
        i18n_key: 'chat.acp.noWorkspaceExec',
        args: {},
        message: 'raw backend message',
      },
    }, 'fallback')

    expect(message).toBe('你没有执行该 Bot 工作区命令的权限。')
  })

  it('renders ACP feedback when structured payload is nested under message', () => {
    const message = resolveApiErrorMessage({
      message: {
        code: 'no_workspace_exec',
        i18n_key: 'chat.acp.noWorkspaceExec',
        args: {},
        message: 'raw backend message',
      },
    }, 'fallback')

    expect(message).toBe('You do not have permission to run workspace commands for this bot.')
  })

  it('renders ACP feedback when WebSocket stream errors carry it under feedback', () => {
    locale = 'zh'

    const message = resolveApiErrorMessage({
      type: 'error',
      message: 'raw backend message',
      feedback: {
        code: 'no_workspace_exec',
        i18n_key: 'chat.acp.noWorkspaceExec',
        args: {},
        message: 'raw backend message',
      },
    }, 'fallback')

    expect(message).toBe('你没有执行该 Bot 工作区命令的权限。')
  })

  it('falls back to existing detail extraction', () => {
    expect(resolveApiErrorMessage({ detail: 'plain detail' }, 'fallback')).toBe('plain detail')
  })
})
