import { Elysia } from 'elysia'
import { devices } from 'playwright'

export const devicesModule = new Elysia({ prefix: '/devices' })
  .get('/', () => {
    return devices
  })