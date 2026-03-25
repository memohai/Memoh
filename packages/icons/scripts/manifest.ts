/**
 * Icon manifest: list of SVG filenames (without .svg extension) in the icons/ directory.
 * The generate script reads each file and produces a Vue SFC component.
 *
 * To add a new icon: place the .svg file in icons/ and add the filename here.
 */

function withVariants(name: string, variants: string[]): string[] {
  return [name, ...variants.map(v => `${name}-${v}`)]
}

// ---------------------------------------------------------------------------
// LLM Providers
// ---------------------------------------------------------------------------

const llmProviders: string[] = [
  ...withVariants('openai', []),
  ...withVariants('anthropic', []),
  ...withVariants('google', ['color', 'brand-color']),
  ...withVariants('deepseek', ['color']),
  ...withVariants('groq', []),
  ...withVariants('huggingface', ['color']),
  ...withVariants('lmstudio', []),
  ...withVariants('minimax', ['color']),
  ...withVariants('mistral', ['color']),
  ...withVariants('moonshot', []),
  ...withVariants('ollama', []),
  ...withVariants('openrouter', []),
  ...withVariants('qwen', ['color']),
  ...withVariants('xai', []),
  ...withVariants('claude', ['color']),
  ...withVariants('gemini', ['color']),
  ...withVariants('meta', ['color']),
  ...withVariants('cohere', ['color']),
  ...withVariants('azure', ['color']),
  ...withVariants('nvidia', ['color']),
  ...withVariants('fireworks', ['color']),
  ...withVariants('together', ['color']),
  ...withVariants('bedrock', ['color']),
  ...withVariants('vertexai', ['color']),
  ...withVariants('baichuan', ['color']),
  ...withVariants('zhipu', ['color']),
  ...withVariants('yi', ['color']),
  ...withVariants('stepfun', ['color']),
  ...withVariants('kimi', ['color']),
  ...withVariants('doubao', ['color']),
  ...withVariants('spark', ['color']),
  ...withVariants('hunyuan', ['color']),
  ...withVariants('bailian', ['color']),
  ...withVariants('siliconcloud', ['color']),
  ...withVariants('volcengine', ['color']),
  ...withVariants('newapi', ['color']),
]

// ---------------------------------------------------------------------------
// Search Providers
// ---------------------------------------------------------------------------

const searchProviders: string[] = [
  ...withVariants('bing', ['color']),
  ...withVariants('yandex', []),
  ...withVariants('tavily', ['color']),
  ...withVariants('jina', []),
  ...withVariants('exa', ['color']),
  ...withVariants('microsoft', ['color']),
  'brave',
  'bocha',
  'duckduckgo',
  'searxng',
  'sogou',
  'serper',
]

// ---------------------------------------------------------------------------
// Channel Platforms
// ---------------------------------------------------------------------------

const channelPlatforms: string[] = [
  'qq',
  'telegram',
  'discord',
  'slack',
  'feishu',
  'wechat',
  'matrix',
]

// ---------------------------------------------------------------------------
// Export
// ---------------------------------------------------------------------------

export const manifest: string[] = [
  ...llmProviders,
  ...searchProviders,
  ...channelPlatforms,
]
