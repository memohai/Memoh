export const DISPLAY_SNAPSHOT_WIDTH = 320
export const DISPLAY_SNAPSHOT_MIME = 'image/png'

export function captureDisplaySnapshot(video: HTMLVideoElement): string | null {
  if (video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA || !video.videoWidth || !video.videoHeight) {
    return null
  }

  const height = Math.max(1, Math.round(DISPLAY_SNAPSHOT_WIDTH * video.videoHeight / video.videoWidth))
  const canvas = document.createElement('canvas')
  canvas.width = DISPLAY_SNAPSHOT_WIDTH
  canvas.height = height

  const ctx = canvas.getContext('2d')
  if (!ctx) {
    return null
  }

  ctx.drawImage(video, 0, 0, DISPLAY_SNAPSHOT_WIDTH, height)
  return canvas.toDataURL(DISPLAY_SNAPSHOT_MIME)
}
