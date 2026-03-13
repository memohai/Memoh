import { Elysia } from 'elysia'
import { getAvailableCores } from '../browser'

export const coresModule = new Elysia({ prefix: '/cores' })
  .get('/', () => {
    return { cores: getAvailableCores() }
  })
