import assert from 'node:assert/strict'
import { test } from 'node:test'
import { extractReply, normalizeMessage } from './normalize.mjs'

test('extractReply uses explicit raw quote fields', () => {
  const { reply, text } = extractReply(
    {
      referMsg: {
        msgId: 'quoted-id',
        title: 'Alice',
        content: 'quoted body',
      },
    },
    'reply body',
  )
  assert.equal(text, 'reply body')
  assert.equal(reply.messageId, 'quoted-id')
  assert.equal(reply.sender, 'Alice')
  assert.equal(reply.preview, 'quoted body')
  assert.equal(reply.raw.source, 'referMsg')
})

test('extractReply falls back to WeChat visible quote text', () => {
  const { reply, text } = extractReply({}, '「Bob：hello」\n- - - - - - - - - - - - - - -\nlook at this')
  assert.equal(text, 'look at this')
  assert.equal(reply.sender, 'Bob')
  assert.equal(reply.preview, 'hello')
  assert.equal(reply.raw.source, 'text_fallback')
})

test('normalizeMessage maps sender, room, quote and image attachment', async () => {
  const message = {
    id: 'msg-1',
    payload: { roomId: 'room-1', talkerId: 'wxid-a' },
    type: () => 6,
    toFileBox: async () => ({
      name: 'photo.jpg',
      mimeType: 'image/jpeg',
      toFile: async (filePath) => {
        await import('node:fs/promises').then((fs) => fs.writeFile(filePath, 'jpeg-data'))
      },
    }),
  }
  const room = { id: 'room-1' }
  const talker = { id: 'wxid-a', self: () => false }
  const normalized = await normalizeMessage(
    message,
    { bot: { Message: { Type: { 6: 'Image' } } }, room, roomTopic: 'Room', talker, talkerName: 'Alice', talkerAlias: 'A' },
    { mediaDir: await import('node:os').then((os) => os.tmpdir()) },
  )
  assert.equal(normalized.sender.id, 'wxid-a')
  assert.equal(normalized.conversation.type, 'group')
  assert.equal(normalized.replyTarget, 'room:room-1')
  assert.equal(normalized.attachments[0].type, 'image')
  assert.equal(normalized.attachments[0].mime, 'image/jpeg')
})
