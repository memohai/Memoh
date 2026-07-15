const runtimeKeyPattern = /^mrk_[0-9a-f]{64}$/

export class InvalidRuntimeKeyError extends Error {
  constructor() {
    super('invalid runtime key')
    this.name = 'InvalidRuntimeKeyError'
  }
}

export function validateRuntimeKey(key: string): void {
  if (!runtimeKeyPattern.test(key.trim())) {
    throw new InvalidRuntimeKeyError()
  }
}
