import fs from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import readline from 'node:readline'
import { WechatyBuilder, ScanStatus } from 'wechaty'
import qrcodeTerminal from 'qrcode-terminal'
import { FileBox } from 'file-box'
import { normalizeMessage } from './normalize.mjs'

function emit(event) {
  process.stdout.write(`${JSON.stringify(event)}\n`)
}

function log(message, extra = {}) {
  process.stderr.write(`${JSON.stringify({ message, ...extra })}\n`)
}

function loadConfig() {
  const raw = process.env.MEMOH_PERSONAL_WECHAT_CONFIG || '{}'
  const cfg = JSON.parse(raw)
  cfg.dataDir ||= '.data/personal-wechat'
  cfg.mediaDir ||= path.join(cfg.dataDir, 'media')
  cfg.sessionName ||= 'MemohPersonalWeChat'
  cfg.allowPrivate = cfg.allowPrivate !== false
  cfg.allowGroups = cfg.allowGroups !== false
  cfg.contactWhitelist ||= []
  cfg.groupWhitelist ||= []
  return cfg
}

function allowedByList(list, values) {
  if (!Array.isArray(list) || list.length === 0 || list.includes('*')) return true
  return values.some((value) => value && list.includes(value))
}

async function collectContext(message, bot) {
  const talker = message.talker()
  const receiver = message.to()
  const room = message.room()
  const [talkerAlias, talkerName, receiverName, roomTopic] = await Promise.all([
    talker?.alias?.().catch(() => ''),
    talker?.name?.().catch(() => ''),
    receiver?.name?.().catch(() => ''),
    room?.topic?.().catch(() => ''),
  ])
  return {
    bot,
    talker,
    receiver,
    room,
    roomTopic: roomTopic || '',
    talkerAlias: talkerAlias || '',
    talkerName: talkerName || '',
    receiverName: receiverName || '',
  }
}

async function shouldAccept(message, context, cfg) {
  if (context.talker?.self?.()) return false
  if (context.room) {
    if (!cfg.allowGroups) return false
    return allowedByList(cfg.groupWhitelist, [context.room.id, context.roomTopic])
  }
  if (!cfg.allowPrivate) return false
  return allowedByList(cfg.contactWhitelist, [context.talker?.id, context.talkerAlias, context.talkerName])
}

async function resolveTarget(bot, target) {
  const value = String(target || '').trim()
  if (value.startsWith('room:')) {
    const id = value.slice('room:'.length)
    const room = bot.Room.load(id)
    await room.ready?.()
    return room
  }
  const id = value.startsWith('contact:') ? value.slice('contact:'.length) : value
  const contact = bot.Contact.load(id)
  await contact.ready?.()
  return contact
}

async function sendCommand(bot, command) {
  const target = await resolveTarget(bot, command.target)
  const text = String(command.message?.text || '').trim()
  if (text) {
    await target.say(text)
  }
  for (const attachment of command.message?.attachments || []) {
    if (!attachment.path) continue
    await target.say(FileBox.fromFile(attachment.path, attachment.name || path.basename(attachment.path)))
  }
}

function attachCommandReader(bot) {
  const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity })
  rl.on('line', async (line) => {
    const trimmed = line.trim()
    if (!trimmed) return
    try {
      const command = JSON.parse(trimmed)
      if (command.type === 'stop') {
        await bot.stop()
        process.exit(0)
      }
      if (command.type === 'send') {
        await sendCommand(bot, command)
      }
    } catch (error) {
      emit({ type: 'error', error: error?.stack || String(error) })
    }
  })
}

export async function startBridge() {
  const cfg = loadConfig()
  fs.mkdirSync(cfg.dataDir, { recursive: true, mode: 0o700 })
  fs.mkdirSync(cfg.mediaDir, { recursive: true, mode: 0o700 })

  const bot = WechatyBuilder.build({
    name: cfg.sessionName,
    puppet: 'wechaty-puppet-wechat4u',
    puppetOptions: { uos: true },
  })

  bot.on('scan', (qrcode, status) => {
    if (status === ScanStatus.Waiting || status === ScanStatus.Timeout) {
      qrcodeTerminal.generate(qrcode, { small: true })
      log('scan_qr', { status })
    }
    emit({ type: 'status', status: `scan:${ScanStatus[status] || status}` })
  })
  bot.on('login', (user) => emit({ type: 'ready', status: `login:${String(user)}` }))
  bot.on('logout', (user) => emit({ type: 'status', status: `logout:${String(user)}` }))
  bot.on('error', (error) => emit({ type: 'error', error: error?.stack || String(error) }))
  bot.on('message', async (message) => {
    try {
      const context = await collectContext(message, bot)
      if (!(await shouldAccept(message, context, cfg))) return
      const normalized = await normalizeMessage(message, context, cfg)
      if (normalized) emit({ type: 'message', message: normalized })
    } catch (error) {
      emit({ type: 'error', error: error?.stack || String(error) })
    }
  })

  attachCommandReader(bot)
  await bot.start()
  emit({ type: 'status', status: 'started' })
}
