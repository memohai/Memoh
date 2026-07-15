import { win32 } from 'node:path'

export interface ShellSpawnSpec {
  command: string
  args: string[]
  detached: boolean
  shell: false | string
  windowsHide: boolean
}

/** Builds the host-native shell invocation for one bridge Exec request. */
export function shellSpawnSpec(
  command: string,
  os: NodeJS.Platform = process.platform,
  environment: NodeJS.ProcessEnv = process.env,
): ShellSpawnSpec {
  if (os === 'win32') {
    return {
      // Let Node construct cmd.exe /d /s /c quoting. Passing the command as
      // an argv element here would apply CreateProcess quoting before cmd.exe
      // parses it and breaks commands containing shell metacharacters.
      command,
      args: [],
      detached: false,
      shell: windowsCommandShell(environment),
      windowsHide: true,
    }
  }
  return {
    command: '/bin/sh',
    args: ['-c', command],
    detached: true,
    shell: false,
    windowsHide: false,
  }
}

export function windowsCommandShell(environment: NodeJS.ProcessEnv = process.env): string {
  const comspec = environmentValue(environment, 'COMSPEC')?.trim()
  if (comspec && win32.isAbsolute(comspec) && win32.basename(comspec).toLowerCase() === 'cmd.exe') {
    return comspec
  }
  return windowsSystemExecutable('cmd.exe', environment)
}

export function windowsSystemExecutable(
  name: string,
  environment: NodeJS.ProcessEnv = process.env,
): string {
  const systemRoot = ['SYSTEMROOT', 'WINDIR']
    .map(variable => environmentValue(environment, variable)?.trim())
    .find(value => value && !value.includes('\0') && win32.isAbsolute(value))
    ?? String.raw`C:\Windows`
  return win32.join(systemRoot, 'System32', win32.basename(name))
}

function environmentValue(environment: NodeJS.ProcessEnv, name: string): string | undefined {
  for (const [key, value] of Object.entries(environment)) {
    if (key.toUpperCase() === name) {
      return value
    }
  }
  return undefined
}
