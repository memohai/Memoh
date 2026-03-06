import { z } from 'zod'

export const BrowserContextConfigModel = z.object({
  viewport: z.object({
    width: z.number(),
    height: z.number(),
  }).optional(),
  userAgent: z.string().optional(),
  deviceScaleFactor: z.number().optional(),
  isMobile: z.boolean().optional(),
  locale: z.string().optional(),
  timezoneId: z.string().optional(),
  geolocation: z.object({
    latitude: z.number(),
    longitude: z.number(),
    accuracy: z.number().optional(),
  }).optional(),
  permissions: z.array(z.string()).optional(),
  extraHTTPHeaders: z.record(z.string(), z.any()).optional(),
  ignoreHTTPSErrors: z.boolean().optional(),
  proxy: z.object({
    server: z.string(),
    bypass: z.string().optional(),
    username: z.string().optional(),
    password: z.string().optional(),
  }).optional(),
})

export type BrowserContextConfig = z.infer<typeof BrowserContextConfigModel>

export const ActionRequestModel = z.object({
  action: z.enum([
    'navigate',
    'click',
    'type',
    'screenshot',
    'get_content',
    'get_html',
    'evaluate',
    'scroll',
    'wait',
    'go_back',
    'go_forward',
    'reload',
    'get_url',
    'get_title',
  ]),
  url: z.string().optional(),
  selector: z.string().optional(),
  text: z.string().optional(),
  script: z.string().optional(),
  direction: z.enum(['up', 'down', 'left', 'right']).optional(),
  amount: z.number().optional(),
  timeout: z.number().optional(),
})

export type ActionRequest = z.infer<typeof ActionRequestModel>
