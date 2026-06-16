import fs from 'node:fs'
import path from 'node:path'

const TYPE_NAMES = {
  2: 'Attachment',
  3: 'Audio',
  4: 'Contact',
  5: 'Emoticon',
  6: 'Image',
  7: 'Text',
  8: 'Video',
  9: 'Url',
  10: 'MiniProgram',
}

function clean(value) {
  return String(value || '').trim()
}

function safeName(value, fallback) {
  const name = clean(value || fallback).replace(/[^\w.\-()\u4e00-\u9fff]+/g, '_')
  return name || fallback
}

function typeName(message, bot) {
  const numeric = message.type?.()
  return bot?.Message?.Type?.[numeric] || TYPE_NAMES[numeric] || String(numeric || 'Unknown')
}

function pickReplyCandidate(payload = {}) {
  const keys = ['quote', 'quoted', 'refer', 'referMsg', 'refMsg', 'reply', 'source', 'appmsg', 'appMsg']
  for (const key of keys) {
    const value = payload?.[key]
    if (value && typeof value === 'object') return { key, value }
  }
  return null
}

function extractTextFallbackQuote(text) {
  const value = String(text || '')
  const marker = '- - - - - - - - - - - - - - -'
  if (!value.includes(marker)) return null
  const [quotedBlock, ...rest] = value.split(marker)
  const body = rest.join(marker).trim()
  const match = quotedBlock.match(/「([^:：\n]+)[:：]([\s\S]*?)」/)
  if (!match) return null
  return {
    reply: {
      sender: match[1].trim(),
      preview: match[2].trim(),
      raw: { source: 'text_fallback' },
    },
    text: body,
  }
}

export function extractReply(payload, text) {
  const candidate = pickReplyCandidate(payload)
  if (candidate) {
    const value = candidate.value
    return {
      reply: {
        messageId: clean(value.id || value.msgId || value.msgid || value.messageId || value.newMsgId),
        sender: clean(value.sender || value.fromUserName || value.from || value.title),
        preview: clean(value.text || value.content || value.message || value.displayContent || value.description),
        raw: { source: candidate.key },
      },
      text,
    }
  }
  return extractTextFallbackQuote(text) || { reply: null, text }
}

function rawPayload(message, cfg) {
  if (!cfg.diagnosticRawPayload) return undefined
  const payload = message.payload || {}
  const allowed = [
    'id',
    'type',
    'filename',
    'text',
    'talkerId',
    'listenerId',
    'roomId',
    'timestamp',
    'quote',
    'quoted',
    'refer',
    'referMsg',
    'refMsg',
    'reply',
    'source',
    'appmsg',
    'appMsg',
  ]
  return Object.fromEntries(allowed.filter((key) => payload[key] !== undefined).map((key) => [key, payload[key]]))
}

async function saveFileBox(fileBox, cfg, messageId, kind) {
  if (!fileBox) return null
  const name = safeName(fileBox.name, `${messageId}-${kind || 'attachment'}`)
  const dir = path.resolve(cfg.mediaDir, new Date().toISOString().slice(0, 10))
  fs.mkdirSync(dir, { recursive: true, mode: 0o700 })
  const filePath = path.join(dir, `${messageId}-${name}`)
  await fileBox.toFile(filePath, true)
  const stat = fs.statSync(filePath)
  return {
    type: kind || 'file',
    path: filePath,
    name,
    size: stat.size,
    mime: clean(fileBox.mimeType || fileBox.mediaType),
  }
}

export async function extractAttachments(message, cfg, msgType) {
  if (msgType === 'Text') return []
  if (typeof message.toFileBox !== 'function') return []
  try {
    const fileBox = await message.toFileBox()
    const kind = msgType === 'Image' ? 'image' : msgType === 'Emoticon' ? 'gif' : msgType.toLowerCase()
    const att = await saveFileBox(fileBox, cfg, message.id, kind)
    return att ? [{ ...att, variant: 'wechaty_filebox' }] : []
  } catch (error) {
    return [
      {
        type: 'file',
        name: `${message.id}.unresolved`,
        base64: `data:text/plain;base64,${Buffer.from(error?.message || String(error)).toString('base64')}`,
        mime: 'text/plain',
        metadata: { error: 'filebox_unavailable' },
      },
    ]
  }
}

export async function normalizeMessage(message, context, cfg) {
  const msgType = typeName(message, context.bot)
  const rawText = msgType === 'Text' ? message.text?.() || '' : ''
  const { reply, text } = extractReply(message.payload || {}, rawText)
  const attachments = await extractAttachments(message, cfg, msgType)
  if (!clean(text) && attachments.length === 0 && !reply) return null
  const room = context.room
  const conversation = room
    ? { id: clean(room.id || message.payload?.roomId), type: 'group', name: context.roomTopic }
    : { id: clean(context.talker?.id || message.payload?.talkerId), type: 'private', name: context.talkerAlias || context.talkerName }
  return {
    id: clean(message.id || message.payload?.id),
    type: msgType,
    text,
    timestamp: new Date().toISOString(),
    replyTarget: room ? `room:${conversation.id}` : `contact:${clean(context.talker?.id || message.payload?.talkerId)}`,
    sender: {
      id: clean(context.talker?.id || message.payload?.talkerId),
      name: context.talkerName,
      alias: context.talkerAlias,
      remark: context.talkerAlias,
      displayName: context.talkerAlias || context.talkerName,
      self: Boolean(context.talker?.self?.()),
    },
    conversation,
    reply,
    attachments,
    raw: rawPayload(message, cfg),
  }
}
