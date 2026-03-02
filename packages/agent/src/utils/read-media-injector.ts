import { ImagePart, PrepareStepFunction, ToolSet, UserModelMessage, tool } from 'ai'
import { z } from 'zod'
import { ModelConfig, ModelInput, hasInputModality } from '../types/model'

const READ_MEDIA_TOOL_NAME = 'readMedia'

const isImageMime = (mime: string): boolean => {
  return mime.trim().toLowerCase().startsWith('image/')
}

const toImagePart = (payload: string): ImagePart => {
  return { type: 'image', image: payload } as ImagePart
}

type ReadMediaFS = {
  download: (path: string) => Promise<Response>
}

const buildReadMediaToolError = (message: string) => ({
  isError: true,
  content: [{ type: 'text', text: message }],
  structuredContent: { ok: false, error: message },
})

const loadImageAsDataUrl = async (
  fs: ReadMediaFS,
  path: string,
): Promise<{ ok: true; dataUrl: string; mime: string } | { ok: false; error: string }> => {
  try {
    const response = await fs.download(path)
    const arrayBuffer = await response.arrayBuffer()
    const base64 = Buffer.from(arrayBuffer).toString('base64')
    const header = response.headers.get('content-type') ?? ''
    const mime = header.split(';')[0]?.trim() ?? ''
    if (!mime || !isImageMime(mime)) {
      return { ok: false, error: 'readMedia only supports image files' }
    }
    return { ok: true, dataUrl: `data:${mime};base64,${base64}`, mime }
  } catch (error) {
    console.error(error)
    const message = error instanceof Error ? error.message : String(error)
    return { ok: false, error: `readMedia failed to load image: ${message}` }
  }
}

export const createPrepareStepWithReadMedia = (params: {
  modelConfig: ModelConfig
  fs: ReadMediaFS
  systemPrompt: string
  basePrepareStep?: PrepareStepFunction
}) => {
  const supportsImage = hasInputModality(params.modelConfig, ModelInput.Image)
  if (!supportsImage) {
    const prepareStep = async (options: Parameters<PrepareStepFunction>[0]) => {
      return (params.basePrepareStep ? await params.basePrepareStep(options) : {}) ?? {}
    }
    return { prepareStep, tools: {} as ToolSet }
  }
  const cachedImages = new Map<string, ImagePart | null>()
  const callOrder: string[] = []

  const readMediaTool = tool({
    description: 'Load an image file into context so the model can view it.',
    inputSchema: z.object({
      path: z.string().describe('Image file path inside the container.'),
    }),
    execute: async ({ path }, options) => {
      const trimmedPath = typeof path === 'string' ? path.trim() : ''
      if (!trimmedPath) {
        return buildReadMediaToolError('path is required')
      }
      const toolCallId = typeof options?.toolCallId === 'string' ? options.toolCallId : ''
      if (!toolCallId) {
        return buildReadMediaToolError('readMedia missing toolCallId')
      }
      if (!cachedImages.has(toolCallId)) {
        cachedImages.set(toolCallId, null)
        callOrder.push(toolCallId)
      }
      const loaded = await loadImageAsDataUrl(params.fs, trimmedPath)
      if (!loaded.ok) {
        return buildReadMediaToolError(loaded.error)
      }
      cachedImages.set(toolCallId, toImagePart(loaded.dataUrl) as ImagePart)
      return { ok: true, path: trimmedPath, mime: loaded.mime }
    },
  })

  const prepareStep = async (options: Parameters<PrepareStepFunction>[0]) => {
    const base = (params.basePrepareStep ? await params.basePrepareStep(options) : {}) ?? {}
    const baseMessages = base.messages ?? options.messages
    if (cachedImages.size === 0) {
      if (!base.system) {
        base.system = params.systemPrompt
      }
      return base
    }
    const imageParts = callOrder
      .map((toolCallId) => cachedImages.get(toolCallId))
      .filter((part): part is ImagePart => Boolean(part))
    if (imageParts.length === 0) {
      if (!base.system) {
        base.system = params.systemPrompt
      }
      return base
    }
    const injectedMessage: UserModelMessage = {
      role: 'user',
      content: imageParts,
    }
    const merged = {
      ...base,
      messages: [...baseMessages, injectedMessage],
    }
    if (!merged.system) {
      merged.system = params.systemPrompt
    }
    return merged
  }

  const readMediaTools: ToolSet = {
    [READ_MEDIA_TOOL_NAME]: readMediaTool,
  }

  return { prepareStep, tools: readMediaTools }
}
