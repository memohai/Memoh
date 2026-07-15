import { describe, expect, it } from 'vitest'

import { shellSpawnSpec, windowsSystemExecutable } from '../src/core/shell'

describe('shell execution', () => {
  it('uses the host shell and only detaches Unix process groups', () => {
    expect(shellSpawnSpec('echo ok', 'linux', {})).toEqual({
      command: '/bin/sh',
      args: ['-c', 'echo ok'],
      detached: true,
      shell: false,
      windowsHide: false,
    })
    expect(shellSpawnSpec('echo ok', 'win32', {
      ComSpec: String.raw`C:\Windows\System32\cmd.exe`,
    })).toEqual({
      command: 'echo ok',
      args: [],
      detached: false,
      shell: String.raw`C:\Windows\System32\cmd.exe`,
      windowsHide: true,
    })
    expect(windowsSystemExecutable('taskkill.exe', { SystemRoot: 'relative' }))
      .toBe(String.raw`C:\Windows\System32\taskkill.exe`)
  })
})
