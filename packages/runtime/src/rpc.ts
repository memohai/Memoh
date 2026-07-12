import { Metadata, status, type ServiceError } from '@grpc/grpc-js'

export { status as RpcStatus }

export function rpcError(code: status, details: string): ServiceError {
  const error = new Error(details) as ServiceError
  error.code = code
  error.details = details
  error.metadata = new Metadata()
  return error
}

export function mapNodeError(error: unknown, operation: string): ServiceError {
  if (isServiceError(error)) {
    return error
  }
  const code = nodeErrorCode(error)
  switch (code) {
    case 'ENOENT':
      return rpcError(status.NOT_FOUND, `${operation}: not found`)
    case 'EACCES':
    case 'EPERM':
    case 'ELOOP':
      return rpcError(status.PERMISSION_DENIED, `${operation}: permission denied`)
    case 'EINVAL':
    case 'ENAMETOOLONG':
    case 'ENOTDIR':
      return rpcError(status.INVALID_ARGUMENT, `${operation}: invalid path or argument`)
    default:
      return rpcError(status.INTERNAL, `${operation} failed`)
  }
}

export function nodeErrorCode(error: unknown): string | undefined {
  return typeof error === 'object' && error !== null && 'code' in error
    ? String(error.code)
    : undefined
}

export function isServiceError(error: unknown): error is ServiceError {
  return error instanceof Error
    && 'code' in error
    && typeof error.code === 'number'
    && 'details' in error
}
