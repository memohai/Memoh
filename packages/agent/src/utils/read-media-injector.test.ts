import { describe, expect, it } from 'vitest'
import { createPrepareStepWithReadMedia } from './read-media-injector'
import { ClientType, ModelInput, ModelConfig } from '../types/model'

const baseModelConfig: ModelConfig = {
  apiKey: 'test',
  baseUrl: 'http://example.com',
  modelId: 'model',
  clientType: ClientType.OpenAIResponses,
  input: [ModelInput.Image],
}

const createToolOptions = (toolCallId: string) => ({
  toolCallId,
  messages: [],
})

describe('read_media runtime', () => {
  it('caches image and injects it into messages', async () => {
    const fs = {
      download: async () =>
        new Response(new Uint8Array([1, 2, 3]), {
          headers: { 'content-type': 'image/png' },
        }),
    }
    const { prepareStep, tools } = createPrepareStepWithReadMedia({
      modelConfig: baseModelConfig,
      fs,
      systemPrompt: 'sys',
    })
    const executeReadMedia = tools.read_media.execute!
    const output = await executeReadMedia(
      { path: '/data/media/a.png' },
      createToolOptions('call-1'),
    )
    expect((output as { ok?: boolean }).ok).toBe(true)
    const prepared = await prepareStep({
      messages: [{ role: 'user', content: 'hi' }],
      steps: [],
      stepNumber: 0,
      model: {} as never,
      experimental_context: undefined,
    })
    const injected = prepared.messages?.[1]
    expect(injected?.role).toBe('user')
    const content = injected?.content as Array<{ type?: string; image?: Uint8Array; mediaType?: string }>
    expect(content?.[0]?.type).toBe('image')
    expect(content?.[0]?.image).toBeInstanceOf(Uint8Array)
    expect(Array.from(content?.[0]?.image ?? [])).toEqual([1, 2, 3])
    expect(content?.[0]?.mediaType).toBe('image/png')
  })

  it('returns error result on download failure', async () => {
    const fs = {
      download: async () => {
        throw new Error('boom')
      },
    }
    const { prepareStep, tools } = createPrepareStepWithReadMedia({
      modelConfig: baseModelConfig,
      fs,
      systemPrompt: 'sys',
    })
    const executeReadMedia = tools.read_media.execute!
    const output = await executeReadMedia(
      { path: '/data/media/a.png' },
      createToolOptions('call-2'),
    )
    expect((output as { isError?: boolean }).isError).toBe(true)
    const prepared = await prepareStep({
      messages: [{ role: 'user', content: 'hi' }],
      steps: [],
      stepNumber: 0,
      model: {} as never,
      experimental_context: undefined,
    })
    expect(prepared.messages).toBeUndefined()
  })

  it('preserves tool call order when downloads finish out of order', async () => {
    const fs = {
      download: async (path: string) => {
        const delay = path.includes('a.png') ? 20 : 0
        await new Promise((resolve) => setTimeout(resolve, delay))
        const payload = path.includes('a.png') ? new Uint8Array([1]) : new Uint8Array([2])
        return new Response(payload, { headers: { 'content-type': 'image/png' } })
      },
    }
    const { prepareStep, tools } = createPrepareStepWithReadMedia({
      modelConfig: baseModelConfig,
      fs,
      systemPrompt: 'sys',
    })
    const executeReadMedia = tools.read_media.execute!
    const first = executeReadMedia(
      { path: '/data/media/a.png' },
      createToolOptions('call-1'),
    )
    const second = executeReadMedia(
      { path: '/data/media/b.png' },
      createToolOptions('call-2'),
    )
    await Promise.all([first, second])
    const prepared = await prepareStep({
      messages: [{ role: 'user', content: 'hi' }],
      steps: [],
      stepNumber: 0,
      model: {} as never,
      experimental_context: undefined,
    })
    const injected = prepared.messages?.[1]
    const content = injected?.content as Array<{ type?: string; image?: Uint8Array; mediaType?: string }>
    expect(Array.from(content?.[0]?.image ?? [])).toEqual([1])
    expect(Array.from(content?.[1]?.image ?? [])).toEqual([2])
    expect(content?.[0]?.mediaType).toBe('image/png')
    expect(content?.[1]?.mediaType).toBe('image/png')
  })
})
