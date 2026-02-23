import { Command } from 'commander'
import chalk from 'chalk'
import inquirer from 'inquirer'
import ora from 'ora'
import { table } from 'table'

import { apiRequest } from '../core/api'
import { ensureAuth, getErrorMessage, resolveBotId } from './shared'
import { getBaseURL, readConfig } from '../utils/store'

type ChannelFieldSchema = {
  type: 'string' | 'secret' | 'bool' | 'number' | 'enum'
  required: boolean
  title?: string
  description?: string
  enum?: string[]
  example?: unknown
}

type ChannelConfigSchema = {
  version: number
  fields: Record<string, ChannelFieldSchema>
}

type ChannelMeta = {
  type: string
  display_name: string
  configless: boolean
  capabilities: Record<string, boolean>
  config_schema: ChannelConfigSchema
  user_config_schema: ChannelConfigSchema
}

type ChannelUserBinding = {
  id: string
  channel_type: string
  user_id: string
  config: Record<string, unknown>
  created_at: string
  updated_at: string
}

type ChannelConfig = {
  id: string
  bot_id: string
  channel_type: string
  credentials: Record<string, unknown>
  external_identity: string
  self_identity: Record<string, unknown>
  routing: Record<string, unknown>
  capabilities: Record<string, unknown>
  disabled: boolean
  verified_at: string
  created_at: string
  updated_at: string
}

const readInboundMode = (credentials: Record<string, unknown>) => {
  const raw = credentials.inboundMode ?? credentials.inbound_mode
  if (typeof raw !== 'string') return ''
  return raw.trim().toLowerCase()
}

const buildWebhookCallbackUrl = (configId: string) => {
  const baseUrl = getBaseURL(readConfig()).replace(/\/+$/, '')
  return `${baseUrl}/channels/feishu/webhook/${encodeURIComponent(configId)}`
}

const printWebhookCallbackIfEnabled = (channelType: string, config: ChannelConfig) => {
  if (channelType !== 'feishu') return
  if (readInboundMode(config.credentials || {}) !== 'webhook') return
  const configId = String(config.id || '').trim()
  if (!configId) {
    console.log(chalk.yellow('Webhook is enabled, but config id is missing so callback URL cannot be generated yet.'))
    return
  }
  console.log(chalk.cyan(`Webhook callback URL: ${buildWebhookCallbackUrl(configId)}`))
}

const renderChannelsTable = (items: ChannelMeta[]) => {
  const rows: string[][] = [['Type', 'Name', 'Configless']]
  for (const item of items) {
    rows.push([item.type, item.display_name, item.configless ? 'yes' : 'no'])
  }
  return table(rows)
}

const fetchChannels = async (token: ReturnType<typeof ensureAuth>) => {
  return apiRequest<ChannelMeta[]>('/channels', {}, token)
}

const resolveChannelType = async (
  token: ReturnType<typeof ensureAuth>,
  preset?: string,
  options?: { allowConfigless?: boolean }
) => {
  if (preset && preset.trim()) {
    return preset.trim()
  }
  const channels = await fetchChannels(token)
  const allowConfigless = options?.allowConfigless ?? false
  const candidates = channels.filter(item => allowConfigless || !item.configless)
  if (candidates.length === 0) {
    console.log(chalk.yellow('No configurable channels available.'))
    process.exit(0)
  }
  const { channelType } = await inquirer.prompt<{ channelType: string }>([
    {
      type: 'list',
      name: 'channelType',
      message: 'Select channel type:',
      choices: candidates.map(item => ({
        name: `${item.display_name} (${item.type})`,
        value: item.type,
      })),
    },
  ])
  return channelType
}

const collectFeishuCredentials = async (opts: Record<string, unknown>) => {
  let appId = typeof opts.app_id === 'string' ? opts.app_id : undefined
  let appSecret = typeof opts.app_secret === 'string' ? opts.app_secret : undefined
  let encryptKey = typeof opts.encrypt_key === 'string' ? opts.encrypt_key : undefined
  let verificationToken = typeof opts.verification_token === 'string' ? opts.verification_token : undefined
  let region = typeof opts.region === 'string' ? opts.region : undefined
  let inboundMode = typeof opts.inbound_mode === 'string' ? opts.inbound_mode : undefined

  const questions = []
  if (!appId) questions.push({ type: 'input', name: 'appId', message: 'Feishu App ID:' })
  if (!appSecret) questions.push({ type: 'password', name: 'appSecret', message: 'Feishu App Secret:' })
  if (!encryptKey) {
    questions.push({ type: 'input', name: 'encryptKey', message: 'Encrypt Key (optional):', default: '' })
  }
  if (!verificationToken) {
    questions.push({ type: 'input', name: 'verificationToken', message: 'Verification Token (optional):', default: '' })
  }
  if (!region) {
    questions.push({
      type: 'list',
      name: 'region',
      message: 'Region:',
      choices: [
        { name: 'Feishu (open.feishu.cn)', value: 'feishu' },
        { name: 'Lark (open.larksuite.com)', value: 'lark' },
      ],
      default: 'feishu',
    })
  }
  if (!inboundMode) {
    questions.push({
      type: 'list',
      name: 'inboundMode',
      message: 'Inbound mode:',
      choices: [
        { name: 'WebSocket', value: 'websocket' },
        { name: 'Webhook', value: 'webhook' },
      ],
      default: 'websocket',
    })
  }
  const answers = questions.length ? await inquirer.prompt<Record<string, string>>(questions) : {}

  appId = appId ?? answers.appId
  appSecret = appSecret ?? answers.appSecret
  encryptKey = encryptKey ?? answers.encryptKey
  verificationToken = verificationToken ?? answers.verificationToken
  region = region ?? answers.region
  inboundMode = inboundMode ?? answers.inboundMode

  const payload: Record<string, unknown> = {
    appId: String(appId).trim(),
    appSecret: String(appSecret).trim(),
    region: String(region || 'feishu').trim(),
    inboundMode: String(inboundMode || 'websocket').trim(),
  }
  if (String(encryptKey || '').trim()) payload.encryptKey = String(encryptKey).trim()
  if (String(verificationToken || '').trim()) payload.verificationToken = String(verificationToken).trim()
  return payload
}

const collectFeishuUserConfig = async (opts: Record<string, unknown>) => {
  let openId = typeof opts.open_id === 'string' ? opts.open_id : undefined
  let userId = typeof opts.user_id === 'string' ? opts.user_id : undefined

  if (!openId && !userId) {
    const answers = await inquirer.prompt<{ kind: 'open_id' | 'user_id'; value: string }>([
      {
        type: 'list',
        name: 'kind',
        message: 'Bind using:',
        choices: [
          { name: 'Open ID', value: 'open_id' },
          { name: 'User ID', value: 'user_id' },
        ],
      },
      {
        type: 'input',
        name: 'value',
        message: 'Value:',
      },
    ])
    if (answers.kind === 'open_id') openId = answers.value
    if (answers.kind === 'user_id') userId = answers.value
  }
  if (!openId && !userId) {
    console.log(chalk.red('open_id or user_id is required.'))
    process.exit(1)
  }
  const config: Record<string, unknown> = {}
  if (openId) config.open_id = String(openId).trim()
  if (userId) config.user_id = String(userId).trim()
  return config
}

export const registerChannelCommands = (program: Command) => {
  const channel = program.command('channel').description('Channel management')

  channel
    .command('list')
    .description('List available channels')
    .action(async () => {
      const token = ensureAuth()
      const channels = await fetchChannels(token)
      if (!channels.length) {
        console.log(chalk.yellow('No channels available.'))
        return
      }
      console.log(renderChannelsTable(channels))
    })

  channel
    .command('info')
    .description('Show channel meta and schema')
    .argument('[type]')
    .action(async (type) => {
      const token = ensureAuth()
      const channelType = await resolveChannelType(token, type, { allowConfigless: true })
      const meta = await apiRequest<ChannelMeta>(`/channels/${encodeURIComponent(channelType)}`, {}, token)
      console.log(JSON.stringify(meta, null, 2))
    })

  const config = channel.command('config').description('Bot channel configuration')

  config
    .command('get')
    .description('Get bot channel config')
    .argument('[bot_id]')
    .option('--type <type>', 'Channel type')
    .action(async (botId, opts) => {
      const token = ensureAuth()
      const resolvedBotId = await resolveBotId(token, botId)
      const channelType = await resolveChannelType(token, opts.type)
      const resp = await apiRequest<ChannelConfig>(`/bots/${encodeURIComponent(resolvedBotId)}/channel/${encodeURIComponent(channelType)}`, {}, token)
      console.log(JSON.stringify(resp, null, 2))
      printWebhookCallbackIfEnabled(channelType, resp)
    })

  config
    .command('set')
    .description('Set bot channel config')
    .argument('[bot_id]')
    .option('--type <type>', 'Channel type (feishu)')
    .option('--app_id <app_id>')
    .option('--app_secret <app_secret>')
    .option('--encrypt_key <encrypt_key>')
    .option('--verification_token <verification_token>')
    .option('--region <region>', 'feishu|lark')
    .option('--inbound_mode <inbound_mode>', 'websocket|webhook')
    .action(async (botId, opts) => {
      const token = ensureAuth()
      const resolvedBotId = await resolveBotId(token, botId)
      const channelType = await resolveChannelType(token, opts.type)
      if (channelType !== 'feishu') {
        console.log(chalk.red(`Channel type ${channelType} is not supported by this command.`))
        process.exit(1)
      }
      const credentials = await collectFeishuCredentials(opts)
      const spinner = ora('Updating channel config...').start()
      try {
        const resp = await apiRequest<ChannelConfig>(`/bots/${encodeURIComponent(resolvedBotId)}/channel/${encodeURIComponent(channelType)}`, {
          method: 'PUT',
          body: JSON.stringify({ credentials }),
        }, token)
        spinner.succeed('Channel config updated')
        printWebhookCallbackIfEnabled(channelType, resp)
      } catch (err: unknown) {
        spinner.fail(getErrorMessage(err) || 'Failed to update channel config')
        process.exit(1)
      }
    })

  const binding = channel.command('bind').description('User channel binding')

  binding
    .command('get')
    .description('Get current user channel binding')
    .option('--type <type>', 'Channel type')
    .action(async (opts) => {
      const token = ensureAuth()
      const channelType = await resolveChannelType(token, opts.type)
      const resp = await apiRequest<ChannelUserBinding>(`/users/me/channels/${encodeURIComponent(channelType)}`, {}, token)
      console.log(JSON.stringify(resp, null, 2))
    })

  binding
    .command('set')
    .description('Set current user channel binding')
    .option('--type <type>', 'Channel type (feishu)')
    .option('--open_id <open_id>')
    .option('--user_id <user_id>')
    .action(async (opts) => {
      const token = ensureAuth()
      const channelType = await resolveChannelType(token, opts.type)
      if (channelType !== 'feishu') {
        console.log(chalk.red(`Channel type ${channelType} is not supported by this command.`))
        process.exit(1)
      }
      const configPayload = await collectFeishuUserConfig(opts)
      const spinner = ora('Updating user binding...').start()
      try {
        await apiRequest(`/users/me/channels/${encodeURIComponent(channelType)}`, {
          method: 'PUT',
          body: JSON.stringify({ config: configPayload }),
        }, token)
        spinner.succeed('User binding updated')
      } catch (err: unknown) {
        spinner.fail(getErrorMessage(err) || 'Failed to update user binding')
        process.exit(1)
      }
    })
}
