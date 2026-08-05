export const AVATAR_ACCEPT = 'image/png,image/jpeg,image/gif,image/webp'
export const MAX_AVATAR_BYTES = 5 * 1024 * 1024

export type AvatarUploadErrorCode = 'invalid_type' | 'too_large' | 'read_failed'

export class AvatarUploadError extends Error {
  readonly code: AvatarUploadErrorCode

  constructor(code: AvatarUploadErrorCode) {
    super(code)
    this.name = 'AvatarUploadError'
    this.code = code
  }
}

export function readAvatarFile(file: File): Promise<string> {
  const accepted = AVATAR_ACCEPT.split(',')
  if (!accepted.includes(file.type.toLowerCase())) {
    return Promise.reject(new AvatarUploadError('invalid_type'))
  }
  if (file.size > MAX_AVATAR_BYTES) {
    return Promise.reject(new AvatarUploadError('too_large'))
  }

  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(new AvatarUploadError('read_failed'))
    reader.onload = () => {
      if (typeof reader.result !== 'string' || !reader.result.startsWith('data:')) {
        reject(new AvatarUploadError('read_failed'))
        return
      }
      resolve(reader.result)
    }
    reader.readAsDataURL(file)
  })
}

export function avatarUploadErrorKey(error: unknown): string {
  if (error instanceof AvatarUploadError) {
    switch (error.code) {
      case 'invalid_type':
        return 'common.avatarInvalidType'
      case 'too_large':
        return 'common.avatarTooLarge'
    }
  }
  return 'common.avatarReadFailed'
}
