import { status } from '@grpc/grpc-js'
import { describe, expect, it } from 'vitest'

import {
  assertSafeEnvironmentName,
  inheritedEnvironment,
} from '../src/core/guards'

describe('runtime guards', () => {
  it.each([
    'LD_PRELOAD',
    'DYLD_INSERT_LIBRARIES',
    'NODE_OPTIONS',
    'BASH_ENV',
    'ENV',
    'PATH',
    'SHELL',
    'COMSPEC',
    'SYSTEMROOT',
    'WINDIR',
    'PATHEXT',
    'IFS',
    'MEMOH_RUNTIME_KEY',
  ])(
    'rejects dangerous environment variable %s',
    name => {
      expect(() => assertSafeEnvironmentName(name)).toThrow(expect.objectContaining({ code: status.PERMISSION_DENIED }))
    },
  )

  it('inherits only the explicit shell allowlist and strips secrets case-insensitively', () => {
    const environment = inheritedEnvironment({
      HOME: '/home/alice',
      USER: 'alice',
      LANG: 'en_US.UTF-8',
      LC_TIME: 'C',
      TMPDIR: '/tmp/alice',
      PATH: '/custom/bin::relative:/usr/bin',
      MEMOH_RUNTIME_KEY: 'mrk_secret',
      node_options: '--require malware.js',
      Bash_Env: '/tmp/startup.sh',
      LD_PRELOAD: '/tmp/inject.so',
      DYLD_INSERT_LIBRARIES: '/tmp/inject.dylib',
      AWS_SECRET_ACCESS_KEY: 'secret',
      GITHUB_TOKEN: 'secret',
      OPENAI_API_KEY: 'secret',
      DATABASE_URL: 'postgres://secret',
      UNLISTED_VALUE: 'must not leak',
    }, 'linux')

    expect(environment).toMatchObject({
      HOME: '/home/alice',
      USER: 'alice',
      LANG: 'en_US.UTF-8',
      LC_TIME: 'C',
      TMPDIR: '/tmp/alice',
      SHELL: '/bin/sh',
    })
    expect(environment.PATH).toContain('/custom/bin')
    expect(environment.PATH).not.toContain('relative')
    for (const name of [
      'MEMOH_RUNTIME_KEY',
      'node_options',
      'Bash_Env',
      'LD_PRELOAD',
      'DYLD_INSERT_LIBRARIES',
      'AWS_SECRET_ACCESS_KEY',
      'GITHUB_TOKEN',
      'OPENAI_API_KEY',
      'DATABASE_URL',
      'UNLISTED_VALUE',
    ]) {
      expect(environment).not.toHaveProperty(name)
      expect(environment).not.toHaveProperty(name.toUpperCase())
    }
  })

  it('preserves only the Windows environment needed to resolve native commands', () => {
    const environment = inheritedEnvironment({
      Path: String.raw`C:\Tools;;relative;D:\Node`,
      SystemRoot: String.raw`C:\Windows`,
      ComSpec: String.raw`C:\Windows\System32\cmd.exe`,
      PATHEXT: '.COM;.EXE;.BAT;.CMD',
      USERPROFILE: String.raw`C:\Users\alice`,
      MEMOH_RUNTIME_KEY: 'mrk_secret',
      GITHUB_TOKEN: 'secret',
    }, 'win32')

    expect(environment).toMatchObject({
      PATH: String.raw`C:\Tools;D:\Node`,
      SYSTEMROOT: String.raw`C:\Windows`,
      COMSPEC: String.raw`C:\Windows\System32\cmd.exe`,
      PATHEXT: '.COM;.EXE;.BAT;.CMD',
      USERPROFILE: String.raw`C:\Users\alice`,
      SHELL: 'cmd.exe',
    })
    expect(environment).not.toHaveProperty('MEMOH_RUNTIME_KEY')
    expect(environment).not.toHaveProperty('GITHUB_TOKEN')
  })
})
