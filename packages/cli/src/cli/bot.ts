import { Command } from 'commander'
import chalk from 'chalk'
import inquirer from 'inquirer'
import ora from 'ora'
import { table } from 'table'
import readline from 'node:readline/promises'
import { stdin as input, stdout as output } from 'node:process'

import {
  getBots,
  postBots,
  putBotsById,
  deleteBotsById,
  getModels,
  type BotsBot,
  type ModelsGetResponse,
} from '@memoh/sdk'
import { client } from '@memoh/sdk/client'
import { ensureAuth, getErrorMessage, resolveBotId } from './shared'
import { streamChat } from './stream'

const getModelId = (item: ModelsGetResponse) => item.model_id ?? ''
const getModelType = (item: ModelsGetResponse) => item.type ?? 'chat'

const ensureModelsReady = async () => {
  ensureAuth()
  const [chatResult, embeddingResult] = await Promise.all([
    getModels({ query: { type: 'chat' }, throwOnError: true }),
    getModels({ query: { type: 'embedding' }, throwOnError: true }),
  ])
  const chatModels = chatResult.data ?? []
  const embeddingModels = embeddingResult.data ?? []
  if (!Array.isArray(chatModels) || chatModels.length === 0 || !Array.isArray(embeddingModels) || embeddingModels.length === 0) {
    console.log(chalk.red('Model configuration incomplete.'))
    console.log(chalk.yellow('At least one chat model and one embedding model are required.'))
    process.exit(1)
  }
}

const renderBotsTable = (items: BotsBot[]) => {
  const rows: string[][] = [['ID', 'Name', 'Type', 'Active', 'Owner']]
  for (const bot of items) {
    rows.push([
      bot.id ?? '',
      bot.display_name || bot.id || '',
      bot.type ?? '',
      bot.is_active ? 'yes' : 'no',
      bot.owner_user_id ?? '',
    ])
  }
  return table(rows)
}

export const registerBotCommands = (program: Command) => {
  const bot = program.command('bot').description('Bot management')

  bot
    .command('list')
    .description('List bots')
    .option('--owner <user_id>', 'Filter by owner user ID (admin only)')
    .action(async (opts) => {
      ensureAuth()
      const { data } = await getBots({
        query: opts.owner ? { owner_id: opts.owner } : undefined,
        throwOnError: true,
      })
      const items = data.items ?? []
      if (!items.length) {
        console.log(chalk.yellow('No bots found.'))
        return
      }
      console.log(renderBotsTable(items))
    })

  bot
    .command('create')
    .description('Create a bot')
    .option('--type <type>', 'Bot type (personal, public)')
    .option('--name <name>', 'Bot display name')
    .option('--avatar <url>', 'Bot avatar URL')
    .option('--active', 'Set bot active')
    .option('--inactive', 'Set bot inactive')
    .action(async (opts) => {
      if (opts.active && opts.inactive) {
        console.log(chalk.red('Use only one of --active or --inactive.'))
        process.exit(1)
      }
      ensureAuth()
      let type = opts.type
      if (!type) {
        const answer = await inquirer.prompt<{ type: string }>([
          {
            type: 'list',
            name: 'type',
            message: 'Bot type:',
            choices: ['personal', 'public'],
          },
        ])
        type = answer.type
      }
      if (!['personal', 'public'].includes(String(type))) {
        console.log(chalk.red('Bot type must be personal or public.'))
        process.exit(1)
      }
      const name = opts.name ?? (await inquirer.prompt<{ name: string }>([
        { type: 'input', name: 'name', message: 'Bot name (optional):', default: '' },
      ])).name
      const body: Record<string, unknown> = {
        type: String(type),
      }
      if (String(name).trim()) body.display_name = String(name).trim()
      if (opts.avatar) body.avatar_url = String(opts.avatar).trim()
      if (opts.active) body.is_active = true
      if (opts.inactive) body.is_active = false
      const spinner = ora('Creating bot...').start()
      try {
        const { data } = await postBots({
          body: body as any,
          throwOnError: true,
        })
        spinner.succeed(`Bot created: ${data.display_name || data.id}`)
      } catch (err: unknown) {
        spinner.fail(getErrorMessage(err) || 'Failed to create bot')
        process.exit(1)
      }
    })

  bot
    .command('update')
    .description('Update bot info')
    .argument('[id]')
    .option('--name <name>', 'Bot display name')
    .option('--avatar <url>', 'Bot avatar URL')
    .option('--active', 'Set bot active')
    .option('--inactive', 'Set bot inactive')
    .action(async (id, opts) => {
      if (opts.active && opts.inactive) {
        console.log(chalk.red('Use only one of --active or --inactive.'))
        process.exit(1)
      }
      ensureAuth()
      const botId = await resolveBotId(id)
      const body: Record<string, unknown> = {}
      if (opts.name) body.display_name = String(opts.name).trim()
      if (opts.avatar) body.avatar_url = String(opts.avatar).trim()
      if (opts.active) body.is_active = true
      if (opts.inactive) body.is_active = false
      if (Object.keys(body).length === 0) {
        const answers = await inquirer.prompt<{ name: string; avatar: string; status: string }>([
          { type: 'input', name: 'name', message: 'Bot name (leave empty to skip):', default: '' },
          { type: 'input', name: 'avatar', message: 'Bot avatar URL (leave empty to skip):', default: '' },
          {
            type: 'list',
            name: 'status',
            message: 'Bot status:',
            choices: [
              { name: 'keep', value: 'keep' },
              { name: 'active', value: 'active' },
              { name: 'inactive', value: 'inactive' },
            ],
          },
        ])
        if (answers.name.trim()) body.display_name = answers.name.trim()
        if (answers.avatar.trim()) body.avatar_url = answers.avatar.trim()
        if (answers.status === 'active') body.is_active = true
        if (answers.status === 'inactive') body.is_active = false
      }
      if (Object.keys(body).length === 0) {
        console.log(chalk.red('No updates provided.'))
        process.exit(1)
      }
      const spinner = ora('Updating bot...').start()
      try {
        await putBotsById({
          path: { id: botId },
          body: body as any,
          throwOnError: true,
        })
        spinner.succeed('Bot updated')
      } catch (err: unknown) {
        spinner.fail(getErrorMessage(err) || 'Failed to update bot')
        process.exit(1)
      }
    })

  bot
    .command('delete')
    .description('Delete a bot')
    .argument('[id]')
    .action(async (id) => {
      ensureAuth()
      const botId = await resolveBotId(id)
      const { confirmed } = await inquirer.prompt<{ confirmed: boolean }>([
        { type: 'confirm', name: 'confirmed', message: `Delete bot ${botId}?`, default: false },
      ])
      if (!confirmed) return
      const spinner = ora('Deleting bot...').start()
      try {
        await deleteBotsById({
          path: { id: botId },
          throwOnError: true,
        })
        spinner.succeed('Bot deleted')
      } catch (err: unknown) {
        spinner.fail(getErrorMessage(err) || 'Failed to delete bot')
        process.exit(1)
      }
    })

  bot
    .command('chat')
    .description('Chat with a bot (stream)')
    .argument('[id]')
    .action(async (id) => {
      await ensureModelsReady()
      ensureAuth()
      const botId = await resolveBotId(id)
      const rl = readline.createInterface({ input, output })
      console.log(chalk.green(`Chatting with ${chalk.bold(botId)}. Type \`exit\` to quit.`))
      while (true) {
        const line = (await rl.question(chalk.cyan('> '))).trim()
        if (!line) {
          if (!input.isTTY && input.readableEnded) break
          continue
        }
        if (line.toLowerCase() === 'exit') {
          break
        }
        await streamChat(line, botId)
      }
      rl.close()
    })

  bot
    .command('set-model')
    .description('Enable model for a bot')
    .argument('[id]')
    .option('--as <usage>', 'chat | memory | embedding')
    .option('--model <model_id>', 'Model ID')
    .action(async (id, opts) => {
      ensureAuth()
      const botId = await resolveBotId(id)
      let enableAs = opts.as
      if (!enableAs) {
        const answer = await inquirer.prompt<{ usage: string }>([{
          type: 'list',
          name: 'usage',
          message: 'Enable as:',
          choices: ['chat', 'memory', 'embedding'],
        }])
        enableAs = answer.usage
      }
      enableAs = String(enableAs).trim()
      if (!['chat', 'memory', 'embedding'].includes(enableAs)) {
        console.log(chalk.red('Enable as must be one of chat, memory, embedding.'))
        process.exit(1)
      }
      const { data: models } = await getModels({ throwOnError: true })
      const modelList = Array.isArray(models) ? models as ModelsGetResponse[] : []
      const requiredType = enableAs === 'embedding' ? 'embedding' : 'chat'
      const candidates = modelList.filter(m => getModelType(m) === requiredType)
      if (candidates.length === 0) {
        console.log(chalk.red(`No ${requiredType} models available.`))
        process.exit(1)
      }
      let modelId = opts.model
      if (!modelId) {
        const answer = await inquirer.prompt<{ model: string }>([{
          type: 'list',
          name: 'model',
          message: 'Select model:',
          choices: candidates.map(m => getModelId(m)),
        }])
        modelId = answer.model
      }
      const selected = candidates.find(m => getModelId(m) === modelId)
      if (!selected) {
        console.log(chalk.red('Selected model not found.'))
        process.exit(1)
      }
      const body: Record<string, unknown> = {}
      if (enableAs === 'chat') body.chat_model_id = getModelId(selected)
      if (enableAs === 'memory') body.memory_model_id = getModelId(selected)
      if (enableAs === 'embedding') body.embedding_model_id = getModelId(selected)
      const spinner = ora('Updating bot settings...').start()
      try {
        // Use raw client because bot_id path parameter is not typed in SDK
        await client.put({
          url: `/bots/${encodeURIComponent(botId)}/settings`,
          body,
          headers: { 'Content-Type': 'application/json' },
          throwOnError: true,
        })
        spinner.succeed('Model enabled')
      } catch (err: unknown) {
        spinner.fail(getErrorMessage(err) || 'Failed to enable model')
        process.exit(1)
      }
    })
}
