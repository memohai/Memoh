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
    })

    expect(environment).toMatchObject({
      HOME: '/home/alice',
      USER: 'alice',
      LANG: 'en_US.UTF-8',
      LC_TIME: 'C',
      TMPDIR: '/tmp/alice',
      SHELL: process.platform === 'win32' ? 'cmd.exe' : '/bin/sh',
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
})
