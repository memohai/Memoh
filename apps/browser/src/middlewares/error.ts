import { Elysia } from 'elysia'

export interface ErrorResponse {
  success: false
  error: string
  code?: string
  details?: unknown
}

export const errorMiddleware = new Elysia({ name: 'error' })
  .onError(({ code, error, set, request }) => {
    const url = new URL(request.url)
    const status = set.status ?? 500
    const message = error instanceof Error ? error.message : String(error)
    console.error('[Error]', {
      method: request.method,
      path: url.pathname,
      code,
      status,
      message,
    })

    switch (code) {
      case 'VALIDATION':
        set.status = 400
        return {
          success: false,
          error: 'Validation failed',
          code: 'VALIDATION_ERROR',
          details: error.message,
        } satisfies ErrorResponse

      case 'NOT_FOUND':
        set.status = 404
        return {
          success: false,
          error: 'Resource not found',
          code: 'NOT_FOUND',
        } satisfies ErrorResponse

      case 'PARSE':
        set.status = 400
        return {
          success: false,
          error: 'Invalid request format',
          code: 'PARSE_ERROR',
          details: error.message,
        } satisfies ErrorResponse

      case 'INTERNAL_SERVER_ERROR':
        set.status = 500
        return {
          success: false,
          error: 'Internal server error',
          code: 'INTERNAL_SERVER_ERROR',
        } satisfies ErrorResponse

      case 'UNKNOWN':
      default:
        if (error instanceof Error) {
          const message = error.message

          if (
            message.includes('No bearer token') ||
            message.includes('Invalid or expired token')
          ) {
            set.status = 401
            return {
              success: false,
              error: message,
              code: 'UNAUTHORIZED',
            } satisfies ErrorResponse
          }

          if (message.includes('Forbidden') || message.includes('Admin access required')) {
            set.status = 403
            return {
              success: false,
              error: message,
              code: 'FORBIDDEN',
            } satisfies ErrorResponse
          }

          if (message.includes('already exists')) {
            set.status = 409
            return {
              success: false,
              error: message,
              code: 'CONFLICT',
            } satisfies ErrorResponse
          }

          if (message.includes('not found')) {
            set.status = 404
            return {
              success: false,
              error: message,
              code: 'NOT_FOUND',
            } satisfies ErrorResponse
          }

          set.status = 500
          return {
            success: false,
            error: message,
            code: 'ERROR',
          } satisfies ErrorResponse
        }

        set.status = 500
        return {
          success: false,
          error: 'An unexpected error occurred',
          code: 'UNKNOWN_ERROR',
        } satisfies ErrorResponse
    }
  })

