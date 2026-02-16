import { Command } from 'commander'
import chalk from 'chalk'
import inquirer from 'inquirer'
import ora from 'ora'
import { table } from 'table'
import readline from 'node:readline/promises'
import { stdin as input, stdout as output } from 'node:process'

import packageJson from '../../package.json'
import { setupClient, client } from '../core/client'
import { registerBotCommands } from './bot'
import { registerChannelCommands } from './channel'
import { streamChat } from './stream'
import {
  readConfig,
  writeConfig,
  writeToken,
  clearToken,
  type TokenInfo,
} from '../utils/store'
import { ensureAuth, getErrorMessage, resolveBotId } from './shared'

import {
  postAuthLogin,
  getUsersMe,
  getProviders,
  postProviders,
  getProvidersNameByName,
  deleteProvidersById,
  getModels,
  postModels,
  deleteModelsModelByModelId,
  type ProvidersGetResponse,
  type ModelsGetResponse,
  type ScheduleSchedule,
  type ScheduleListResponse,
} from '@memoh/sdk'

// ---------------------------------------------------------------------------
// Initialize SDK client
// ---------------------------------------------------------------------------

setupClient()

// ---------------------------------------------------------------------------
// Program setup
// ---------------------------------------------------------------------------

const program = new Command()
program
  .name('memoh')
  .description('Memoh CLI')
  .version(packageJson.version)

registerBotCommands(program)
registerChannelCommands(program)

// ---------------------------------------------------------------------------
// Model/Provider helpers
// ---------------------------------------------------------------------------

const getModelId = (item: ModelsGetResponse) => item.model_id ?? ''
const getProviderId = (item: ModelsGetResponse) => item.llm_provider_id ?? ''
const getModelType = (item: ModelsGetResponse) => item.type ?? 'chat'
const getModelInputModalities = (item: ModelsGetResponse) => item.input_modalities ?? ['text']

const ensureModelsReady = async () => {
  ensureAuth()
  const [chatResult, embeddingResult] = await Promise.all([
    getModels({ query: { type: 'chat' }, throwOnError: true }),
    getModels({ query: { type: 'embedding' }, throwOnError: true }),
  ])
  const chatModels = chatResult.data ?? []
  const embeddingModels = embeddingResult.data ?? []
  if (!Array.isArray(chatModels) || chatModels.length === 0 ||
      !Array.isArray(embeddingModels) || embeddingModels.length === 0) {
    console.log(chalk.red('Model configuration incomplete.'))
    console.log(chalk.yellow('At least one chat model and one embedding model are required.'))
    process.exit(1)
  }
}

const renderProvidersTable = (providers: ProvidersGetResponse[], models: ModelsGetResponse[]) => {
  const rows: string[][] = [['Provider', 'Type', 'Base URL', 'Models']]
  for (const provider of providers) {
    const providerModels = models
      .filter(m => getProviderId(m) === provider.id)
      .map(m => `${getModelId(m)} (${getModelType(m)})`)
    rows.push([
      provider.name ?? '',
      provider.client_type ?? '',
      provider.base_url ?? '',
      providerModels.join(', ') || '-',
    ])
  }
  return table(rows)
}

const renderModelsTable = (models: ModelsGetResponse[], providers: ProvidersGetResponse[]) => {
  const providerMap = new Map(providers.map(p => [p.id, p.name]))
  const rows: string[][] = [['Model ID', 'Type', 'Provider', 'Input Modalities']]
  for (const item of models) {
    rows.push([
      getModelId(item),
      getModelType(item),
      providerMap.get(getProviderId(item)) ?? getProviderId(item),
      getModelInputModalities(item).join(', '),
    ])
  }
  return table(rows)
}

const renderSchedulesTable = (items: ScheduleSchedule[]) => {
  const rows: string[][] = [['ID', 'Name', 'Pattern', 'Enabled', 'Max Calls', 'Current Calls', 'Command']]
  for (const item of items) {
    rows.push([
      item.id ?? '',
      item.name ?? '',
      item.pattern ?? '',
      item.enabled ? 'yes' : 'no',
      item.max_calls === null || item.max_calls === undefined ? '-' : String(item.max_calls),
      item.current_calls === undefined ? '-' : String(item.current_calls),
      item.command ?? '',
    ])
  }
  return table(rows)
}

// ---------------------------------------------------------------------------
// Auth commands
// ---------------------------------------------------------------------------

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
      const { data } = await postAuthLogin({
        body: {
          username: answers.username,
          password: answers.password,
        },
        throwOnError: true,
      })
      const tokenInfo: TokenInfo = {
        access_token: data.access_token ?? '',
        token_type: data.token_type ?? 'bearer',
        expires_at: data.expires_at ?? '',
        user_id: data.user_id ?? '',
        username: data.username,
      }
      writeToken(tokenInfo)
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
  .action(async () => {
    const token = ensureAuth()
    try {
      const { data } = await getUsersMe({ throwOnError: true })
      if (data.username) console.log(`username: ${data.username}`)
      if (data.display_name) console.log(`display_name: ${data.display_name}`)
      if (data.id) console.log(`user_id: ${data.id}`)
      if (data.role) console.log(`role: ${data.role}`)
    } catch {
      // Fallback to token info if API call fails
      if (token.username) console.log(`username: ${token.username}`)
      if (token.user_id) console.log(`user_id: ${token.user_id}`)
    }
  })

// ---------------------------------------------------------------------------
// Config commands
// ---------------------------------------------------------------------------

const configCmd = program
  .command('config')
  .description('Show or update current config')

configCmd.action(async () => {
  const config = readConfig()
  console.log(`host = "${config.host}"`)
  console.log(`port = ${config.port}`)
})

configCmd
  .command('set')
  .description('Update config')
  .option('--host <host>')
  .option('--port <port>')
  .action(async (opts) => {
    const current = readConfig()
    let host = opts.host
    let port = opts.port ? Number.parseInt(opts.port, 10) : undefined

    if (!host && !port) {
      const answers = await inquirer.prompt([
        { type: 'input', name: 'host', message: 'Host:', default: current.host },
        { type: 'input', name: 'port', message: 'Port:', default: current.port },
      ])
      host = answers.host
      port = Number.parseInt(answers.port, 10)
    }

    if (host) current.host = host
    if (port && !Number.isNaN(port)) current.port = port

    writeConfig(current)
    console.log(chalk.green('Config updated'))
  })

// ---------------------------------------------------------------------------
// Provider commands
// ---------------------------------------------------------------------------

const provider = program.command('provider').description('Provider management')

provider
  .command('list')
  .description('List providers')
  .option('--provider <name>', 'Filter by provider name')
  .action(async (opts) => {
    ensureAuth()
    let providers: ProvidersGetResponse[]
    if (opts.provider) {
      const { data } = await getProvidersNameByName({
        path: { name: opts.provider },
        throwOnError: true,
      })
      providers = [data]
    } else {
      const { data } = await getProviders({ throwOnError: true })
      providers = data as ProvidersGetResponse[]
    }
    const { data: models } = await getModels({ throwOnError: true })
    console.log(renderProvidersTable(providers, models as ModelsGetResponse[]))
  })

provider
  .command('create')
  .description('Create provider')
  .option('--name <name>')
  .option('--type <type>')
  .option('--base_url <url>')
  .option('--api_key <key>')
  .action(async (opts) => {
    ensureAuth()
    const questions = []
    if (!opts.name) questions.push({ type: 'input', name: 'name', message: 'Provider name:' })
    if (!opts.type) {
      questions.push({
        type: 'list',
        name: 'client_type',
        message: 'Client type:',
        choices: ['openai', 'openai-compat', 'anthropic', 'google', 'azure', 'bedrock', 'mistral', 'xai', 'ollama', 'dashscope'],
      })
    }
    if (!opts.base_url) questions.push({ type: 'input', name: 'base_url', message: 'Base URL:' })
    if (!opts.api_key) questions.push({ type: 'password', name: 'api_key', message: 'API key:' })
    const answers = questions.length ? await inquirer.prompt(questions) : {}
    const spinner = ora('Creating provider...').start()
    try {
      await postProviders({
        body: {
          name: opts.name ?? answers.name,
          client_type: opts.type ?? answers.client_type,
          base_url: opts.base_url ?? answers.base_url,
          api_key: opts.api_key ?? answers.api_key,
        },
        throwOnError: true,
      })
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
    ensureAuth()
    if (!opts.provider) {
      console.log(chalk.red('Provider name is required.'))
      process.exit(1)
    }
    const { data: providerInfo } = await getProvidersNameByName({
      path: { name: opts.provider },
      throwOnError: true,
    })
    const spinner = ora('Deleting provider...').start()
    try {
      await deleteProvidersById({
        path: { id: providerInfo.id! },
        throwOnError: true,
      })
      spinner.succeed('Provider deleted')
    } catch (err: unknown) {
      spinner.fail(getErrorMessage(err) || 'Failed to delete provider')
      process.exit(1)
    }
  })

// ---------------------------------------------------------------------------
// Model commands
// ---------------------------------------------------------------------------

const model = program.command('model').description('Model management')

model
  .command('list')
  .description('List models')
  .action(async () => {
    ensureAuth()
    const [modelsResult, providersResult] = await Promise.all([
      getModels({ throwOnError: true }),
      getProviders({ throwOnError: true }),
    ])
    console.log(renderModelsTable(
      modelsResult.data as ModelsGetResponse[],
      providersResult.data as ProvidersGetResponse[],
    ))
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
    ensureAuth()
    const { data: providerList } = await getProviders({ throwOnError: true })
    const providers = providerList as ProvidersGetResponse[]
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
    const inputModalities = opts.multimodal ? ['text', 'image'] : ['text']
    const spinner = ora('Creating model...').start()
    try {
      await postModels({
        body: {
          model_id: modelId,
          name: opts.name ?? modelId,
          llm_provider_id: provider.id,
          input_modalities: inputModalities,
          type: modelType,
          dimensions,
        },
        throwOnError: true,
      })
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
    ensureAuth()
    if (!opts.model) {
      console.log(chalk.red('Model name is required.'))
      process.exit(1)
    }
    const spinner = ora('Deleting model...').start()
    try {
      await deleteModelsModelByModelId({
        path: { modelId: opts.model },
        throwOnError: true,
      })
      spinner.succeed('Model deleted')
    } catch (err: unknown) {
      spinner.fail(getErrorMessage(err) || 'Failed to delete model')
      process.exit(1)
    }
  })

// ---------------------------------------------------------------------------
// Schedule commands (uses raw client due to untyped bot_id path param)
// ---------------------------------------------------------------------------

const schedule = program.command('schedule').description('Schedule management')
  .option('--bot <id>', 'Bot ID (required for schedule operations)')

const resolveScheduleBotId = async (opts: { bot?: string }) => {
  return await resolveBotId(opts.bot)
}

schedule
  .command('list')
  .description('List schedules')
  .action(async () => {
    ensureAuth()
    const botId = await resolveScheduleBotId(schedule.opts())
    const { data } = await client.get({
      url: `/bots/${encodeURIComponent(botId)}/schedule`,
      throwOnError: true,
    })
    const resp = data as ScheduleListResponse
    if (!resp.items?.length) {
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
    ensureAuth()
    const botId = await resolveScheduleBotId(schedule.opts())
    const { data } = await client.get({
      url: `/bots/${encodeURIComponent(botId)}/schedule/${encodeURIComponent(id)}`,
      throwOnError: true,
    })
    console.log(JSON.stringify(data, null, 2))
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
      max_calls: maxCalls !== undefined ? { set: true, value: maxCalls } : undefined,
      enabled: opts.enabled ? true : (opts.disabled ? false : undefined),
    }
    ensureAuth()
    const botId = await resolveScheduleBotId(schedule.opts())
    const spinner = ora('Creating schedule...').start()
    try {
      await client.post({
        url: `/bots/${encodeURIComponent(botId)}/schedule`,
        body: payload,
        headers: { 'Content-Type': 'application/json' },
        throwOnError: true,
      })
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
      payload.max_calls = { set: true, value: parsed }
    }
    if (opts.enabled) payload.enabled = true
    if (opts.disabled) payload.enabled = false
    if (Object.keys(payload).length === 0) {
      console.log(chalk.red('No updates provided.'))
      process.exit(1)
    }
    ensureAuth()
    const botId = await resolveScheduleBotId(schedule.opts())
    const spinner = ora('Updating schedule...').start()
    try {
      await client.put({
        url: `/bots/${encodeURIComponent(botId)}/schedule/${encodeURIComponent(id)}`,
        body: payload,
        headers: { 'Content-Type': 'application/json' },
        throwOnError: true,
      })
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
    ensureAuth()
    const botId = await resolveScheduleBotId(schedule.opts())
    const { data: current } = await client.get({
      url: `/bots/${encodeURIComponent(botId)}/schedule/${encodeURIComponent(id)}`,
      throwOnError: true,
    })
    const currentSchedule = current as ScheduleSchedule
    const spinner = ora('Updating schedule...').start()
    try {
      await client.put({
        url: `/bots/${encodeURIComponent(botId)}/schedule/${encodeURIComponent(id)}`,
        body: { enabled: !currentSchedule.enabled },
        headers: { 'Content-Type': 'application/json' },
        throwOnError: true,
      })
      spinner.succeed(`Schedule ${currentSchedule.enabled ? 'disabled' : 'enabled'}`)
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
    ensureAuth()
    const botId = await resolveScheduleBotId(schedule.opts())
    const spinner = ora('Deleting schedule...').start()
    try {
      await client.delete({
        url: `/bots/${encodeURIComponent(botId)}/schedule/${encodeURIComponent(id)}`,
        throwOnError: true,
      })
      spinner.succeed('Schedule deleted')
    } catch (err: unknown) {
      spinner.fail(getErrorMessage(err) || 'Failed to delete schedule')
      process.exit(1)
    }
  })

// ---------------------------------------------------------------------------
// Default action: interactive chat
// ---------------------------------------------------------------------------

program
  .option('--bot <id>', 'Bot id to chat with')
  .action(async () => {
    await ensureModelsReady()
    ensureAuth()
    const botId = await resolveBotId(program.opts().bot)

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
      await streamChat(line, botId)
    }
    rl.close()
  })

// ---------------------------------------------------------------------------
// Version command
// ---------------------------------------------------------------------------

program
  .command('version')
  .description('Show version information')
  .action(() => {
    console.log(`Memoh CLI v${packageJson.version}`)
  })

// ---------------------------------------------------------------------------
// TUI command
// ---------------------------------------------------------------------------

program
  .command('tui')
  .description('Terminal UI chat session')
  .option('--bot <id>', 'Bot id to chat with')
  .action(async (opts: { bot?: string }) => {
    await ensureModelsReady()
    ensureAuth()
    const botId = await resolveBotId(opts.bot)
    await runTui(botId)
  })

program.parseAsync(process.argv)

const runTui = async (botId: string) => {
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
    await streamChat(line, botId)
  }
  rl.close()
}
