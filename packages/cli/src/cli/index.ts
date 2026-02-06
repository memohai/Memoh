#!/usr/bin/env bun
import { Command } from 'commander'
import chalk from 'chalk'
import inquirer from 'inquirer'
import ora from 'ora'
import { table } from 'table'
import readline from 'node:readline/promises'
import { stdin as input, stdout as output } from 'node:process'
import { randomUUID } from 'node:crypto'

import packageJson from '../../package.json'
import { apiRequest } from '../core/api'
import { registerBotCommands } from './bot'
import { registerChannelCommands } from './channel'
import {
  readConfig,
  writeConfig,
  readToken,
  writeToken,
  clearToken,
  TokenInfo,
  getBaseURL,
} from '../utils/store'

type Provider = {
  id: string
  name: string
  client_type: string
  base_url: string
  api_key?: string
}

type Model = {
  model_id: string
  name?: string
  llm_provider_id: string
  is_multimodal: boolean
  type: 'chat' | 'embedding'
  dimensions?: number
}

type ModelResponse = Partial<Model> & {
  model_id?: string
  model?: Model
}

type Schedule = {
  id: string
  name: string
  description: string
  pattern: string
  max_calls?: number | null
  current_calls?: number
  created_at?: string
  updated_at?: string
  enabled: boolean
  command: string
  user_id?: string
}

type ScheduleListResponse = {
  items: Schedule[]
}

type Settings = {
  chat_model_id: string
  memory_model_id: string
  embedding_model_id: string
  max_context_load_time: number
  language: string
}

type Bot = {
  id: string
  name?: string
  display_name?: string
  description?: string
  avatar?: string
  type?: string
  owner_user_id: string
  is_public?: boolean
  created_at: string
  updated_at: string
}

type BotListResponse = {
  items: Bot[]
}

const program = new Command()
program
  .name('memoh')
  .description('Memoh CLI')
  .version(packageJson.version)

registerBotCommands(program)
registerChannelCommands(program)

const ensureAuth = () => {
  const token = readToken()
  if (!token?.access_token) {
    console.log(chalk.red('Not logged in. Run `memoh login` first.'))
    process.exit(1)
  }
  return token
}

const ensureModelsReady = async () => {
  const token = ensureAuth()
  const [chatModels, embeddingModels] = await Promise.all([
    apiRequest<ModelResponse[]>('/models?type=chat', {}, token),
    apiRequest<ModelResponse[]>('/models?type=embedding', {}, token),
  ])
  if (chatModels.length === 0 || embeddingModels.length === 0) {
    console.log(chalk.red('Model configuration incomplete.'))
    console.log(chalk.yellow('At least one chat model and one embedding model are required.'))
    process.exit(1)
  }
}

const getErrorMessage = (err: unknown) => {
  if (err && typeof err === 'object' && 'message' in err) {
    const value = (err as { message?: unknown }).message
    if (typeof value === 'string') return value
  }
  return 'Unknown error'
}

const resolveBotId = async (token: TokenInfo, preset?: string) => {
  if (preset && preset.trim()) {
    return preset.trim()
  }
  const spinner = ora('Fetching bots...').start()
  let bots: Bot[] = []
  try {
    const resp = await apiRequest<BotListResponse>('/bots', {}, token)
    bots = resp.items
    spinner.stop()
  } catch (err: unknown) {
    spinner.fail(`Failed to fetch bots: ${getErrorMessage(err)}`)
    process.exit(1)
  }
  if (bots.length === 0) {
    console.log(chalk.yellow('No bots found. Please create a bot first.'))
    process.exit(0)
  }
  const { botId } = await inquirer.prompt([
    {
      type: 'list',
      name: 'botId',
      message: 'Select a bot to chat with:',
      choices: bots.map(b => ({
        name: `${b.display_name || b.name || b.id || 'unknown'} ${b.type ? chalk.gray(b.type) : ''}`.trim(),
        value: b.id,
      })),
    },
  ])
  return botId as string
}

const getModelId = (item: ModelResponse) => item.model?.model_id ?? item.model_id ?? ''
const getProviderId = (item: ModelResponse) => item.model?.llm_provider_id ?? item.llm_provider_id ?? ''
const getModelType = (item: ModelResponse) => item.model?.type ?? item.type ?? 'chat'
const getModelMultimodal = (item: ModelResponse) => item.model?.is_multimodal ?? item.is_multimodal ?? false

const renderProvidersTable = (providers: Provider[], models: ModelResponse[]) => {
  const rows: string[][] = [['Provider', 'Type', 'Base URL', 'Models']]
  for (const provider of providers) {
    const providerModels = models
      .filter(m => getProviderId(m) === provider.id)
      .map(m => `${getModelId(m)} (${getModelType(m)})`)
    rows.push([
      provider.name,
      provider.client_type,
      provider.base_url,
      providerModels.join(', ') || '-',
    ])
  }
  return table(rows)
}

const renderModelsTable = (models: ModelResponse[], providers: Provider[]) => {
  const providerMap = new Map(providers.map(p => [p.id, p.name]))
  const rows: string[][] = [['Model ID', 'Type', 'Provider', 'Multimodal']]
  for (const item of models) {
    rows.push([
      getModelId(item),
      getModelType(item),
      providerMap.get(getProviderId(item)) ?? getProviderId(item),
      getModelMultimodal(item) ? 'yes' : 'no',
    ])
  }
  return table(rows)
}

const renderSchedulesTable = (items: Schedule[]) => {
  const rows: string[][] = [['ID', 'Name', 'Pattern', 'Enabled', 'Max Calls', 'Current Calls', 'Command']]
  for (const item of items) {
    rows.push([
      item.id,
      item.name,
      item.pattern,
      item.enabled ? 'yes' : 'no',
      item.max_calls === null || item.max_calls === undefined ? '-' : String(item.max_calls),
      item.current_calls === undefined ? '-' : String(item.current_calls),
      item.command,
    ])
  }
  return table(rows)
}

program
  .command('login')
  .description('Login')
  .action(async () => {
    const answers = await inquirer.prompt([
      { type: 'input', name: 'username', message: 'Username:' },
      { type: 'password', name: 'password', message: 'Password:' },
    ])
    const spinner = ora('Logging in...').start()
    try {
      const resp = await apiRequest<TokenInfo>('/auth/login', {
        method: 'POST',
        body: JSON.stringify({
          username: answers.username,
          password: answers.password,
        }),
      }, null)
      writeToken(resp)
      spinner.succeed('Logged in')
    } catch (err: unknown) {
      spinner.fail(getErrorMessage(err) || 'Login failed')
      process.exit(1)
    }
  })

program
  .command('logout')
  .description('Logout')
  .action(() => {
    clearToken()
    console.log(chalk.green('Logged out'))
  })

program
  .command('whoami')
  .description('Show current user')
  .action(() => {
    const token = readToken()
    if (!token?.access_token) {
      console.log(chalk.red('Not logged in.'))
      process.exit(1)
    }
    if (token.username) {
      console.log(`username: ${token.username}`)
    }
    if (token.user_id) {
      console.log(`user_id: ${token.user_id}`)
      return
    }
    const payload = token.access_token.split('.')[1]
    if (!payload) {
      console.log(chalk.yellow('Token found but payload missing.'))
      return
    }
    const decoded = Buffer.from(payload, 'base64').toString('utf-8')
    try {
      const data = JSON.parse(decoded)
      console.log(`user_id: ${data.user_id ?? data.sub ?? 'unknown'}`)
    } catch {
      console.log(chalk.yellow('Unable to parse token payload.'))
    }
  })

const configCmd = program
  .command('config')
  .description('Show or update current config')

configCmd.action(async () => {
  const config = readConfig()
  console.log(`host = "${config.host}"`)
  console.log(`port = ${config.port}`)
  const token = readToken()
  if (!token?.access_token) return
  try {
    const settings = await apiRequest<Settings>('/settings', {}, token)
    console.log(`chat_model_id = "${settings.chat_model_id || ''}"`)
    console.log(`memory_model_id = "${settings.memory_model_id || ''}"`)
    console.log(`embedding_model_id = "${settings.embedding_model_id || ''}"`)
    console.log(`max_context_load_time = ${settings.max_context_load_time}`)
    console.log(`language = "${settings.language}"`)
  } catch (err: unknown) {
    console.log(chalk.yellow(`Unable to load settings: ${getErrorMessage(err)}`))
  }
})

configCmd
  .command('set')
  .description('Update config')
  .option('--host <host>')
  .option('--port <port>')
  .option('--chat_model_id <model_id>')
  .option('--memory_model_id <model_id>')
  .option('--embedding_model_id <model_id>')
  .option('--max_context_load_time <minutes>')
  .option('--language <language>')
  .action(async (opts) => {
    const current = readConfig()
    let host = opts.host
    let port = opts.port ? Number.parseInt(opts.port, 10) : undefined
    let maxContextLoadTime: number | undefined
    if (opts.max_context_load_time !== undefined) {
      const parsed = Number.parseInt(opts.max_context_load_time, 10)
      if (Number.isNaN(parsed) || parsed <= 0) {
        console.log(chalk.red('max_context_load_time must be a positive integer.'))
        process.exit(1)
      }
      maxContextLoadTime = parsed
    }
    let language = opts.language
    const hasSettingsInput = opts.max_context_load_time !== undefined
      || opts.language !== undefined
      || opts.chat_model_id !== undefined
      || opts.memory_model_id !== undefined
      || opts.embedding_model_id !== undefined
    const hasConfigInput = Boolean(host || port)

    if (!hasConfigInput && !hasSettingsInput) {
      const answers = await inquirer.prompt([
        { type: 'input', name: 'host', message: 'Host:', default: current.host },
        { type: 'input', name: 'port', message: 'Port:', default: current.port },
      ])
      host = answers.host
      port = Number.parseInt(answers.port, 10)
    }

    if (host) current.host = host
    if (port && !Number.isNaN(port)) current.port = port

    if (host || (port && !Number.isNaN(port))) {
      writeConfig(current)
      console.log(chalk.green('Config updated'))
    }

    if (hasSettingsInput) {
      if (language) {
        language = String(language).trim()
      }
      const payload: Partial<Settings> = {}
      if (opts.chat_model_id) payload.chat_model_id = String(opts.chat_model_id).trim()
      if (opts.memory_model_id) payload.memory_model_id = String(opts.memory_model_id).trim()
      if (opts.embedding_model_id) payload.embedding_model_id = String(opts.embedding_model_id).trim()
      if (maxContextLoadTime !== undefined) payload.max_context_load_time = maxContextLoadTime
      if (language) payload.language = language
      const token = ensureAuth()
      const spinner = ora('Updating settings...').start()
      try {
        await apiRequest('/settings', { method: 'PUT', body: JSON.stringify(payload) }, token)
        spinner.succeed('Settings updated')
      } catch (err: unknown) {
        spinner.fail(getErrorMessage(err) || 'Failed to update settings')
        process.exit(1)
      }
    }
  })

const provider = program.command('provider').description('Provider management')

provider
  .command('list')
  .description('List providers')
  .option('--provider <name>', 'Filter by provider name')
  .action(async (opts) => {
    const token = ensureAuth()
    const providers = opts.provider
      ? [await apiRequest<Provider>(`/providers/name/${encodeURIComponent(opts.provider)}`, {}, token)]
      : await apiRequest<Provider[]>('/providers', {}, token)
    const models = await apiRequest<ModelResponse[]>('/models', {}, token)
    console.log(renderProvidersTable(providers, models))
  })

provider
  .command('create')
  .description('Create provider')
  .option('--name <name>')
  .option('--type <type>')
  .option('--base_url <url>')
  .option('--api_key <key>')
  .action(async (opts) => {
    const token = ensureAuth()
    const questions = []
    if (!opts.name) questions.push({ type: 'input', name: 'name', message: 'Provider name:' })
    if (!opts.type) {
      questions.push({
        type: 'list',
        name: 'client_type',
        message: 'Client type:',
        choices: ['openai', 'anthropic', 'google', 'ollama'],
      })
    }
    if (!opts.base_url) questions.push({ type: 'input', name: 'base_url', message: 'Base URL:' })
    if (!opts.api_key) questions.push({ type: 'password', name: 'api_key', message: 'API key:' })
    const answers = questions.length ? await inquirer.prompt(questions) : {}
    const payload = {
      name: opts.name ?? answers.name,
      client_type: opts.type ?? answers.client_type,
      base_url: opts.base_url ?? answers.base_url,
      api_key: opts.api_key ?? answers.api_key,
    }
    const spinner = ora('Creating provider...').start()
    try {
      await apiRequest('/providers', { method: 'POST', body: JSON.stringify(payload) }, token)
      spinner.succeed('Provider created')
    } catch (err: unknown) {
      spinner.fail(getErrorMessage(err) || 'Failed to create provider')
      process.exit(1)
    }
  })

provider
  .command('delete')
  .description('Delete provider')
  .option('--provider <name>', 'Provider name')
  .action(async (opts) => {
    const token = ensureAuth()
    if (!opts.provider) {
      console.log(chalk.red('Provider name is required.'))
      process.exit(1)
    }
    const providerInfo = await apiRequest<Provider>(`/providers/name/${encodeURIComponent(opts.provider)}`, {}, token)
    const spinner = ora('Deleting provider...').start()
    try {
      await apiRequest(`/providers/${providerInfo.id}`, { method: 'DELETE' }, token)
      spinner.succeed('Provider deleted')
    } catch (err: unknown) {
      spinner.fail(getErrorMessage(err) || 'Failed to delete provider')
      process.exit(1)
    }
  })

const model = program.command('model').description('Model management')

model
  .command('list')
  .description('List models')
  .action(async () => {
    const token = ensureAuth()
    const [models, providers] = await Promise.all([
      apiRequest<ModelResponse[]>('/models', {}, token),
      apiRequest<Provider[]>('/providers', {}, token),
    ])
    console.log(renderModelsTable(models, providers))
  })

model
  .command('create')
  .description('Create model')
  .option('--model_id <model_id>')
  .option('--name <name>')
  .option('--provider <provider>')
  .option('--type <type>')
  .option('--dimensions <dimensions>')
  .option('--multimodal', 'Is multimodal')
  .action(async (opts) => {
    const token = ensureAuth()
    const providers = await apiRequest<Provider[]>('/providers', {}, token)
    let provider = providers.find(p => p.name === opts.provider)
    if (!provider) {
      const answer = await inquirer.prompt([{
        type: 'list',
        name: 'provider',
        message: 'Select provider:',
        choices: providers.map(p => p.name),
      }])
      provider = providers.find(p => p.name === answer.provider)
    }
    if (!provider) {
      console.log(chalk.red('Provider not found.'))
      process.exit(1)
    }
    const questions = []
    if (!opts.model_id) questions.push({ type: 'input', name: 'model_id', message: 'Model ID (e.g. gpt-4):' })
    if (!opts.type) questions.push({ type: 'list', name: 'type', message: 'Model type:', choices: ['chat', 'embedding'] })
    const answers = questions.length ? await inquirer.prompt(questions) : {}
    const modelId = opts.model_id ?? answers.model_id
    const modelType = opts.type ?? answers.type
    let dimensions = opts.dimensions ? Number.parseInt(opts.dimensions, 10) : undefined
    if (modelType === 'embedding' && (!dimensions || Number.isNaN(dimensions))) {
      const dimAnswer = await inquirer.prompt([{
        type: 'input',
        name: 'dimensions',
        message: 'Embedding dimensions (e.g. 1536):',
      }])
      dimensions = Number.parseInt(dimAnswer.dimensions, 10)
    }
    if (modelType === 'embedding' && (!dimensions || Number.isNaN(dimensions) || dimensions <= 0)) {
      console.log(chalk.red('Embedding models require a valid dimensions value.'))
      process.exit(1)
    }
    const isMultimodal = Boolean(opts.multimodal)
    const payload = {
      model_id: modelId,
      name: opts.name ?? modelId,
      llm_provider_id: provider.id,
      is_multimodal: isMultimodal,
      type: modelType,
      dimensions,
    }
    const spinner = ora('Creating model...').start()
    try {
      await apiRequest('/models', { method: 'POST', body: JSON.stringify(payload) }, token)
      spinner.succeed('Model created')
    } catch (err: unknown) {
      spinner.fail(getErrorMessage(err) || 'Failed to create model')
      process.exit(1)
    }
  })

model
  .command('delete')
  .description('Delete model')
  .option('--model <model>')
  .action(async (opts) => {
    const token = ensureAuth()
    if (!opts.model) {
      console.log(chalk.red('Model name is required.'))
      process.exit(1)
    }
    const spinner = ora('Deleting model...').start()
    try {
      await apiRequest(`/models/model/${encodeURIComponent(opts.model)}`, { method: 'DELETE' }, token)
      spinner.succeed('Model deleted')
    } catch (err: unknown) {
      spinner.fail(getErrorMessage(err) || 'Failed to delete model')
      process.exit(1)
    }
  })

const schedule = program.command('schedule').description('Schedule management')

schedule
  .command('list')
  .description('List schedules')
  .action(async () => {
    const token = ensureAuth()
    const resp = await apiRequest<ScheduleListResponse>('/schedule', {}, token)
    if (!resp.items.length) {
      console.log(chalk.yellow('No schedules found.'))
      return
    }
    console.log(renderSchedulesTable(resp.items))
  })

schedule
  .command('get')
  .description('Get schedule')
  .argument('<id>')
  .action(async (id) => {
    const token = ensureAuth()
    const resp = await apiRequest<Schedule>(`/schedule/${encodeURIComponent(id)}`, {}, token)
    console.log(JSON.stringify(resp, null, 2))
  })

schedule
  .command('create')
  .description('Create schedule')
  .option('--name <name>')
  .option('--description <description>')
  .option('--pattern <pattern>')
  .option('--command <command>')
  .option('--max_calls <max_calls>')
  .option('--enabled')
  .option('--disabled')
  .action(async (opts) => {
    if (opts.enabled && opts.disabled) {
      console.log(chalk.red('Use only one of --enabled or --disabled.'))
      process.exit(1)
    }
    const questions = []
    if (!opts.name) questions.push({ type: 'input', name: 'name', message: 'Name:' })
    if (!opts.description) questions.push({ type: 'input', name: 'description', message: 'Description:' })
    if (!opts.pattern) questions.push({ type: 'input', name: 'pattern', message: 'Cron pattern:' })
    if (!opts.command) questions.push({ type: 'input', name: 'command', message: 'Command:' })
    if (opts.max_calls === undefined) {
      questions.push({
        type: 'input',
        name: 'max_calls',
        message: 'Max calls (optional, empty for unlimited):',
        default: '',
      })
    }
    const answers = questions.length ? await inquirer.prompt(questions) : {}
    const maxCallsInput = opts.max_calls ?? answers.max_calls
    let maxCalls: number | undefined
    if (maxCallsInput !== undefined && String(maxCallsInput).trim() !== '') {
      const parsed = Number.parseInt(String(maxCallsInput), 10)
      if (Number.isNaN(parsed) || parsed <= 0) {
        console.log(chalk.red('max_calls must be a positive integer.'))
        process.exit(1)
      }
      maxCalls = parsed
    }
    const payload = {
      name: opts.name ?? answers.name,
      description: opts.description ?? answers.description,
      pattern: opts.pattern ?? answers.pattern,
      command: opts.command ?? answers.command,
      max_calls: maxCalls,
      enabled: opts.enabled ? true : (opts.disabled ? false : undefined),
    }
    const token = ensureAuth()
    const spinner = ora('Creating schedule...').start()
    try {
      await apiRequest('/schedule', { method: 'POST', body: JSON.stringify(payload) }, token)
      spinner.succeed('Schedule created')
    } catch (err: unknown) {
      spinner.fail(getErrorMessage(err) || 'Failed to create schedule')
      process.exit(1)
    }
  })

schedule
  .command('update')
  .description('Update schedule')
  .argument('<id>')
  .option('--name <name>')
  .option('--description <description>')
  .option('--pattern <pattern>')
  .option('--command <command>')
  .option('--max_calls <max_calls>')
  .option('--enabled')
  .option('--disabled')
  .action(async (id, opts) => {
    if (opts.enabled && opts.disabled) {
      console.log(chalk.red('Use only one of --enabled or --disabled.'))
      process.exit(1)
    }
    const payload: Record<string, unknown> = {}
    if (opts.name) payload.name = opts.name
    if (opts.description) payload.description = opts.description
    if (opts.pattern) payload.pattern = opts.pattern
    if (opts.command) payload.command = opts.command
    if (opts.max_calls !== undefined) {
      const parsed = Number.parseInt(String(opts.max_calls), 10)
      if (Number.isNaN(parsed) || parsed <= 0) {
        console.log(chalk.red('max_calls must be a positive integer.'))
        process.exit(1)
      }
      payload.max_calls = parsed
    }
    if (opts.enabled) payload.enabled = true
    if (opts.disabled) payload.enabled = false
    if (Object.keys(payload).length === 0) {
      console.log(chalk.red('No updates provided.'))
      process.exit(1)
    }
    const token = ensureAuth()
    const spinner = ora('Updating schedule...').start()
    try {
      await apiRequest(`/schedule/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify(payload),
      }, token)
      spinner.succeed('Schedule updated')
    } catch (err: unknown) {
      spinner.fail(getErrorMessage(err) || 'Failed to update schedule')
      process.exit(1)
    }
  })

schedule
  .command('toggle')
  .description('Enable/disable schedule')
  .argument('<id>')
  .action(async (id) => {
    const token = ensureAuth()
    const current = await apiRequest<Schedule>(`/schedule/${encodeURIComponent(id)}`, {}, token)
    const spinner = ora('Updating schedule...').start()
    try {
      await apiRequest(`/schedule/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify({ enabled: !current.enabled }),
      }, token)
      spinner.succeed(`Schedule ${current.enabled ? 'disabled' : 'enabled'}`)
    } catch (err: unknown) {
      spinner.fail(getErrorMessage(err) || 'Failed to update schedule')
      process.exit(1)
    }
  })

schedule
  .command('delete')
  .description('Delete schedule')
  .argument('<id>')
  .action(async (id) => {
    const token = ensureAuth()
    const spinner = ora('Deleting schedule...').start()
    try {
      await apiRequest(`/schedule/${encodeURIComponent(id)}`, { method: 'DELETE' }, token)
      spinner.succeed('Schedule deleted')
    } catch (err: unknown) {
      spinner.fail(getErrorMessage(err) || 'Failed to delete schedule')
      process.exit(1)
    }
  })

program
  .option('--bot <id>', 'Bot id to chat with')
  .action(async () => {
    await ensureModelsReady()
    const token = ensureAuth()
    const botId = await resolveBotId(token, program.opts().bot)
    const sessionId = `cli:${randomUUID()}`

    const rl = readline.createInterface({ input, output })
    console.log(chalk.green(`Chatting with ${chalk.bold(botId)}. Type \`exit\` to quit.`))

    while (true) {
      const line = (await rl.question(chalk.cyan('> '))).trim()
      if (!line) {
        if (input.readableEnded) break
        continue
      }
      if (line.toLowerCase() === 'exit') {
        break
      }
      try {
        const ok = await streamChat(line, botId, sessionId, token)
        if (!ok) {
          console.log(chalk.red('Chat failed or stream unavailable.'))
        }
      } catch (err: unknown) {
        console.log(chalk.red(getErrorMessage(err) || 'Chat failed'))
      }
    }
    rl.close()
  })

program
  .command('version')
  .description('Show version information')
  .action(() => {
    console.log(`Memoh CLI v${packageJson.version}`)
  })

program
  .command('tui')
  .description('Terminal UI chat session')
  .option('--bot <id>', 'Bot id to chat with')
  .action(async (opts: { bot?: string }) => {
    await ensureModelsReady()
    const token = ensureAuth()
    const botId = await resolveBotId(token, opts.bot)
    await runTui(botId, token)
  })

program.parseAsync(process.argv)

const streamChat = async (query: string, botId: string, sessionId: string, token: TokenInfo) => {
  const config = readConfig()
  const baseURL = getBaseURL(config)
  const resp = await fetch(`${baseURL}/bots/${botId}/chat/stream?session_id=${sessionId}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token.access_token}`,
    },
    body: JSON.stringify({ query }),
  }).catch(() => null)
  if (!resp || !resp.ok || !resp.body) return false

  const stream = resp.body
  const reader = stream.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let printed = false
  while (true) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let idx
    while ((idx = buffer.indexOf('\n')) >= 0) {
      const line = buffer.slice(0, idx).trim()
      buffer = buffer.slice(idx + 1)
      if (!line.startsWith('data:')) continue
      const payload = line.slice(5).trim()
      if (!payload || payload === '[DONE]') continue
      const text = extractTextFromEvent(payload)
      if (text) {
        process.stdout.write(text)
        printed = true
      }
    }
  }
  if (printed) {
    process.stdout.write('\n')
  }
  return true
}

const extractTextFromMessage = (message: unknown) => {
  if (typeof message === 'string') return message
  if (message && typeof message === 'object') {
    const value = message as { text?: unknown; parts?: unknown[] }
    if (typeof value.text === 'string') return value.text
    if (Array.isArray(value.parts)) {
      const lines = value.parts
        .map((part) => {
          if (!part || typeof part !== 'object') return ''
          const typed = part as { text?: unknown; url?: unknown; emoji?: unknown }
          if (typeof typed.text === 'string' && typed.text.trim()) return typed.text
          if (typeof typed.url === 'string' && typed.url.trim()) return typed.url
          if (typeof typed.emoji === 'string' && typed.emoji.trim()) return typed.emoji
          return ''
        })
        .filter(Boolean)
      if (lines.length) return lines.join('\n')
    }
  }
  return null
}

const extractTextFromEvent = (payload: string) => {
  try {
    const event = JSON.parse(payload)
    if (typeof event === 'string') return event
    if (typeof event?.text === 'string') return event.text
    const messageText = extractTextFromMessage(event?.message)
    if (messageText) return messageText
    if (typeof event?.delta === 'string') return event.delta
    if (typeof event?.delta?.content === 'string') return event.delta.content
    if (typeof event?.content === 'string') return event.content
    if (typeof event?.data === 'string') return event.data
    if (typeof event?.data?.text === 'string') return event.data.text
    if (typeof event?.data?.delta?.content === 'string') return event.data.delta.content
    const nestedMessageText = extractTextFromMessage(event?.data?.message)
    if (nestedMessageText) return nestedMessageText
    return null
  } catch {
    return payload
  }
}

const runTui = async (botId: string, token: TokenInfo) => {
  const sessionId = `cli:${randomUUID()}`

  const rl = readline.createInterface({ input, output })
  console.log(chalk.green(`TUI session (line mode) with ${chalk.bold(botId)}. Type \`exit\` to quit.`))
  while (true) {
    const line = (await rl.question(chalk.cyan('> '))).trim()
    if (!line) {
      if (input.readableEnded) break
      continue
    }
    if (line.toLowerCase() === 'exit') {
      break
    }
    try {
      const ok = await streamChat(line, botId, sessionId, token)
      if (!ok) {
        console.log(chalk.red('Chat failed or stream unavailable.'))
      }
    } catch (err: unknown) {
      console.log(chalk.red(getErrorMessage(err) || 'Chat failed'))
    }
  }
  rl.close()
}

