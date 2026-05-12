import { app } from 'electron'
import { chmodSync, existsSync, mkdirSync } from 'node:fs'
import { delimiter, join } from 'node:path'
import { desktopResourcePath } from './paths'

const gstLaunchEnv = 'MEMOH_GSTREAMER_LAUNCH'

function gstreamerTarget(): string | null {
  if (process.platform === 'darwin') {
    return 'darwin-universal'
  }
  if (process.platform === 'win32' && process.arch === 'x64') {
    return 'win32-x64'
  }
  return null
}

function gstLaunchBinaryName(): string {
  return process.platform === 'win32' ? 'gst-launch-1.0.exe' : 'gst-launch-1.0'
}

function bundledGStreamerRoot(): string | null {
  const target = gstreamerTarget()
  if (!target) {
    return null
  }
  const root = desktopResourcePath('gstreamer', target)
  return existsSync(join(root, 'bin', gstLaunchBinaryName())) ? root : null
}

export function bundledGStreamerEnv(): NodeJS.ProcessEnv {
  if (process.env[gstLaunchEnv]?.trim()) {
    return {}
  }

  const root = bundledGStreamerRoot()
  if (!root) {
    return {}
  }

  const launchPath = join(root, 'bin', gstLaunchBinaryName())
  if (process.platform !== 'win32') {
    try {
      chmodSync(launchPath, 0o755)
    } catch {
      // The server can still fall back to the system GStreamer resolver.
      return {}
    }
  }

  const registryDir = join(app.getPath('userData'), 'gstreamer')
  mkdirSync(registryDir, { recursive: true })

  return {
    [gstLaunchEnv]: launchPath,
    PATH: `${join(root, 'bin')}${delimiter}${process.env.PATH ?? ''}`,
    GST_PLUGIN_PATH_1_0: join(root, 'lib', 'gstreamer-1.0'),
    GST_PLUGIN_SYSTEM_PATH_1_0: '',
    GST_PLUGIN_SCANNER: join(root, 'libexec', 'gstreamer-1.0', process.platform === 'win32' ? 'gst-plugin-scanner.exe' : 'gst-plugin-scanner'),
    GST_REGISTRY_1_0: join(registryDir, `registry-${gstreamerTarget()}.bin`),
  }
}
