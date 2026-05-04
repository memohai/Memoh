import { app } from 'electron'
import { existsSync } from 'node:fs'
import { join, resolve } from 'node:path'

export function repoRoot(): string {
  if (app.isPackaged) {
    return resolve(app.getAppPath(), '..', '..')
  }
  const cwd = process.cwd()
  if (existsSync(join(cwd, 'apps', 'desktop', 'package.json'))) {
    return cwd
  }
  if (existsSync(join(cwd, '..', '..', 'apps', 'desktop', 'package.json'))) {
    return resolve(cwd, '..', '..')
  }
  return resolve(app.getAppPath(), '..', '..')
}

export function desktopResourcePath(...segments: string[]): string {
  if (app.isPackaged) {
    return join(process.resourcesPath, ...segments)
  }
  return join(repoRoot(), 'apps', 'desktop', 'resources', ...segments)
}

export function desktopServerWorkDir(): string {
  return join(app.getPath('userData'), 'local-server')
}
