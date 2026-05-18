import { describe, expect, it } from 'vitest'
import { join } from 'node:path'
import { detectDockerHost } from './docker-host'

function detectDockerHostWithFixtures(
  home: string,
  fixtures: {
    dirs?: Record<string, string[]>
    env?: NodeJS.ProcessEnv
    files?: Record<string, string>
    inspectDockerContext?: () => string
    platform?: NodeJS.Platform
    sockets?: string[]
  },
): string {
  return detectDockerHost(home, {
    env: fixtures.env ?? {},
    exists: path => fixtures.sockets?.includes(path) ?? false,
    inspectDockerContext: fixtures.inspectDockerContext ?? (() => ''),
    platform: fixtures.platform ?? 'darwin',
    readDir: path => fixtures.dirs?.[path] ?? [],
    readTextFile: path => fixtures.files?.[path],
  })
}

describe('detectDockerHost', () => {
  const home = '/Users/ran'
  const metaRoot = join(home, '.docker', 'contexts', 'meta')

  it('uses DOCKER_HOST before probing contexts or socket paths', () => {
    expect(detectDockerHostWithFixtures(home, {
      env: { DOCKER_HOST: 'tcp://docker.example.test:2376' },
      inspectDockerContext: () => 'unix:///should-not-be-used.sock',
      sockets: ['/var/run/docker.sock'],
    })).toBe('tcp://docker.example.test:2376')
  })

  it('uses the current Docker context metadata before stale macOS sockets', () => {
    expect(detectDockerHostWithFixtures(home, {
      dirs: { [metaRoot]: ['orbstack-context'] },
      files: {
        [join(home, '.docker', 'config.json')]: JSON.stringify({ currentContext: 'orbstack' }),
        [join(metaRoot, 'orbstack-context', 'meta.json')]: JSON.stringify({
          Name: 'orbstack',
          Endpoints: { docker: { Host: 'unix:///Users/ran/.orbstack/run/docker.sock' } },
        }),
      },
      sockets: [
        join(home, '.docker', 'run', 'docker.sock'),
        '/var/run/docker.sock',
      ],
    })).toBe('unix:///Users/ran/.orbstack/run/docker.sock')
  })

  it('lets DOCKER_CONTEXT override the configured current context', () => {
    expect(detectDockerHostWithFixtures(home, {
      dirs: { [metaRoot]: ['desktop', 'orbstack'] },
      env: { DOCKER_CONTEXT: 'orbstack' },
      files: {
        [join(home, '.docker', 'config.json')]: JSON.stringify({ currentContext: 'desktop-linux' }),
        [join(metaRoot, 'desktop', 'meta.json')]: JSON.stringify({
          Name: 'desktop-linux',
          Endpoints: { docker: { Host: 'unix:///Users/ran/.docker/run/docker.sock' } },
        }),
        [join(metaRoot, 'orbstack', 'meta.json')]: JSON.stringify({
          Name: 'orbstack',
          Endpoints: { docker: { Host: 'unix:///Users/ran/.orbstack/run/docker.sock' } },
        }),
      },
    })).toBe('unix:///Users/ran/.orbstack/run/docker.sock')
  })

  it('reads Docker context data from DOCKER_CONFIG when present', () => {
    const dockerConfig = '/tmp/custom-docker-config'
    const customMetaRoot = join(dockerConfig, 'contexts', 'meta')

    expect(detectDockerHostWithFixtures(home, {
      dirs: { [customMetaRoot]: ['colima'] },
      env: { DOCKER_CONFIG: dockerConfig },
      files: {
        [join(dockerConfig, 'config.json')]: JSON.stringify({ currentContext: 'colima' }),
        [join(customMetaRoot, 'colima', 'meta.json')]: JSON.stringify({
          Name: 'colima',
          Endpoints: { docker: { Host: 'unix:///Users/ran/.colima/default/docker.sock' } },
        }),
      },
    })).toBe('unix:///Users/ran/.colima/default/docker.sock')
  })

  it('falls back to /var/run/docker.sock before Docker Desktop user sockets on macOS', () => {
    expect(detectDockerHostWithFixtures(home, {
      sockets: [
        join(home, '.docker', 'run', 'docker.sock'),
        '/var/run/docker.sock',
      ],
    })).toBe('unix:///var/run/docker.sock')
  })

  it('falls back to rootless Docker sockets on Linux', () => {
    expect(detectDockerHostWithFixtures('/home/ran', {
      env: { XDG_RUNTIME_DIR: '/run/user/501' },
      platform: 'linux',
      sockets: ['/run/user/501/docker.sock'],
    })).toBe('unix:///run/user/501/docker.sock')
  })

  it('keeps Windows on Docker defaults unless env or context provides a host', () => {
    expect(detectDockerHostWithFixtures('C:\\Users\\ran', {
      platform: 'win32',
      sockets: ['C:\\Users\\ran\\.docker\\run\\docker.sock'],
    })).toBe('')
  })

  it('supports Windows named pipe hosts from Docker context metadata', () => {
    const windowsHome = 'C:\\Users\\ran'
    const windowsMetaRoot = join(windowsHome, '.docker', 'contexts', 'meta')

    expect(detectDockerHostWithFixtures(windowsHome, {
      dirs: { [windowsMetaRoot]: ['desktop'] },
      env: { DOCKER_CONTEXT: 'desktop-linux' },
      files: {
        [join(windowsMetaRoot, 'desktop', 'meta.json')]: JSON.stringify({
          Name: 'desktop-linux',
          Endpoints: { docker: { Host: 'npipe:////./pipe/dockerDesktopLinuxEngine' } },
        }),
      },
      platform: 'win32',
    })).toBe('npipe:////./pipe/dockerDesktopLinuxEngine')
  })
})
