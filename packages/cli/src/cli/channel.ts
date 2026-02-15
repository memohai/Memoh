import { Command } from 'commander'
import chalk from 'chalk'
import inquirer from 'inquirer'
import ora from 'ora'
import { table } from 'table'

import {
  getChannels,
  getChannelsByPlatform,
  getBotsByIdChannelByPlatform,
  putBotsByIdChannelByPlatform,
  getUsersMeChannelsByPlatform,
  putUsersMeChannelsByPlatform,
  type HandlersChannelMeta,
} from '@memoh/sdk'
import { ensureAuth, getErrorMessage, resolveBotId } from './shared'

const renderChannelsTable = (items: HandlersChannelMeta[]) => {
  const rows: string[][] = [['Type', 'Name', 'Configless']]
  for (const item of items) {
    rows.push([item.type ?? '', item.display_name ?? '', item.configless ? 'yes' : 'no'])
  }
  return table(rows)
}

const fetchChannelList = async () => {
  const { data } = await getChannels({ throwOnError: true })
  return data as HandlersChannelMeta[]
}

const resolveChannelType = async (
  preset?: string,
  options?: { allowConfigless?: boolean }
) => {
  if (preset && preset.trim()) {
    return preset.trim()
  }
  const channels = await fetchChannelList()
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

  const questions = []
  if (!appId) questions.push({ type: 'input', name: 'appId', message: 'Feishu App ID:' })
  if (!appSecret) questions.push({ type: 'password', name: 'appSecret', message: 'Feishu App Secret:' })
  if (!encryptKey) {
    questions.push({ type: 'input', name: 'encryptKey', message: 'Encrypt Key (optional):', default: '' })
  }
  if (!verificationToken) {
    questions.push({ type: 'input', name: 'verificationToken', message: 'Verification Token (optional):', default: '' })
  }
  const answers = questions.length ? await inquirer.prompt<Record<string, string>>(questions) : {}

  appId = appId ?? answers.appId
  appSecret = appSecret ?? answers.appSecret
  encryptKey = encryptKey ?? answers.encryptKey
  verificationToken = verificationToken ?? answers.verificationToken

  const payload: Record<string, unknown> = {
    appId: String(appId).trim(),
    appSecret: String(appSecret).trim(),
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
      ensureAuth()
      const channels = await fetchChannelList()
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
      ensureAuth()
      const channelType = await resolveChannelType(type, { allowConfigless: true })
      const { data } = await getChannelsByPlatform({
        path: { platform: channelType },
        throwOnError: true,
      })
      console.log(JSON.stringify(data, null, 2))
    })

  const config = channel.command('config').description('Bot channel configuration')

  config
    .command('get')
    .description('Get bot channel config')
    .argument('[bot_id]')
    .option('--type <type>', 'Channel type')
    .action(async (botId, opts) => {
      ensureAuth()
      const resolvedBotId = await resolveBotId(botId)
      const channelType = await resolveChannelType(opts.type)
      const { data } = await getBotsByIdChannelByPlatform({
        path: { id: resolvedBotId, platform: channelType },
        throwOnError: true,
      })
      console.log(JSON.stringify(data, null, 2))
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
    .action(async (botId, opts) => {
      ensureAuth()
      const resolvedBotId = await resolveBotId(botId)
      const channelType = await resolveChannelType(opts.type)
      if (channelType !== 'feishu') {
        console.log(chalk.red(`Channel type ${channelType} is not supported by this command.`))
        process.exit(1)
      }
      const credentials = await collectFeishuCredentials(opts)
      const spinner = ora('Updating channel config...').start()
      try {
        await putBotsByIdChannelByPlatform({
          path: { id: resolvedBotId, platform: channelType },
          body: { credentials },
          throwOnError: true,
        })
        spinner.succeed('Channel config updated')
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
      ensureAuth()
      const channelType = await resolveChannelType(opts.type)
      const { data } = await getUsersMeChannelsByPlatform({
        path: { platform: channelType },
        throwOnError: true,
      })
      console.log(JSON.stringify(data, null, 2))
    })

  binding
    .command('set')
    .description('Set current user channel binding')
    .option('--type <type>', 'Channel type (feishu)')
    .option('--open_id <open_id>')
    .option('--user_id <user_id>')
    .action(async (opts) => {
      ensureAuth()
      const channelType = await resolveChannelType(opts.type)
      if (channelType !== 'feishu') {
        console.log(chalk.red(`Channel type ${channelType} is not supported by this command.`))
        process.exit(1)
      }
      const configPayload = await collectFeishuUserConfig(opts)
      const spinner = ora('Updating user binding...').start()
      try {
        await putUsersMeChannelsByPlatform({
          path: { platform: channelType },
          body: { config: configPayload },
          throwOnError: true,
        })
        spinner.succeed('User binding updated')
      } catch (err: unknown) {
        spinner.fail(getErrorMessage(err) || 'Failed to update user binding')
        process.exit(1)
      }
    })
}
