import chalk from 'chalk'
import inquirer from 'inquirer'
import ora from 'ora'

import { getBots, type BotsBot } from '@memoh/sdk'
import { readToken, type TokenInfo } from '../utils/store'

export type BotSummary = BotsBot

export const ensureAuth = (): TokenInfo => {
  const token = readToken()
  if (!token?.access_token) {
    console.log(chalk.red('Not logged in. Run `memoh login` first.'))
    process.exit(1)
  }
  return token
}

export const getErrorMessage = (err: unknown) => {
  if (err && typeof err === 'object' && 'message' in err) {
    const value = (err as { message?: unknown }).message
    if (typeof value === 'string') return value
  }
  return 'Unknown error'
}

export const fetchBots = async () => {
  const { data } = await getBots({ throwOnError: true })
  return data.items ?? []
}

export const resolveBotId = async (preset?: string) => {
  if (preset && preset.trim()) {
    return preset.trim()
  }
  const spinner = ora('Fetching bots...').start()
  try {
    const bots = await fetchBots()
    spinner.stop()
    if (bots.length === 0) {
      console.log(chalk.yellow('No bots found. Please create a bot first.'))
      process.exit(0)
    }
    const { botId } = await inquirer.prompt<{ botId: string }>([
      {
        type: 'list',
        name: 'botId',
        message: 'Select a bot:',
        choices: bots.map(bot => ({
          name: `${bot.display_name || bot.id} ${chalk.gray(bot.type ?? '')}`,
          value: bot.id,
        })),
      },
    ])
    return botId
  } catch (err: unknown) {
    spinner.fail(`Failed to fetch bots: ${getErrorMessage(err)}`)
    process.exit(1)
  }
}
