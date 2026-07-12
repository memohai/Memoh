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

  it.each([
    ['zh', '启动工作区失败'],
    ['ja', 'Workspace を起動できませんでした'],
  ])('localizes workspace errors for %s instead of exposing backend English', (language, expected) => {
    locale = language

    const message = resolveApiErrorMessage({
      code: 'workspace_start_failed',
      i18n_key: 'bots.container.startFailed',
      args: {},
      message: 'failed to start container: connection refused',
    }, 'fallback')

    expect(message).toBe(expected)
  })
})
