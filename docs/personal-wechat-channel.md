# Personal WeChat Channel

`personal_wechat` is a Memoh-native channel for personal WeChat accounts. It does not call Memoh's web message API. The Go adapter owns channel registration, configuration, lifecycle, routing and Memoh message normalization. A Node sidecar owns Wechaty login, WeChat events, media download and outbound delivery.

## Architecture

- Go adapter: `internal/channel/adapters/personalwechat`
- Sidecar: `packages/personal-wechat-bridge`
- Protocol: newline-delimited JSON over sidecar stdout/stdin
- Channel type: `personal_wechat`

Inbound sidecar event:

```json
{"type":"message","message":{"id":"...","text":"...","sender":{"id":"..."},"conversation":{"id":"...","type":"group"},"replyTarget":"room:...","reply":{"messageId":"...","sender":"...","preview":"..."},"attachments":[{"type":"image","path":"/data/media/a.jpg","mime":"image/jpeg"}]}}
```

Outbound Go command:

```json
{"type":"send","target":"contact:wxid_xxx","message":{"text":"hello"}}
```

## Configuration

- `bridgeExecutable`: executable for the sidecar, default `node`
- `bridgeScript`: sidecar script, default `packages/personal-wechat-bridge/bin/personal-wechat-bridge.mjs`
- `dataDir`: persistent Wechaty session and diagnostics directory
- `mediaDir`: inbound media directory
- `sessionName`: Wechaty memory-card name
- `allowPrivate`, `allowGroups`: coarse inbound switches
- `contactWhitelist`, `groupWhitelist`: comma-separated IDs or display names; empty means allow all for the enabled chat type
- `diagnosticRawPayload`: includes sanitized raw payload fields for quote/media verification

## Capability Notes

Sender identity is mapped from Wechaty `talker()` and room context into `channel.Identity` and `channel.Conversation`.

Quote support is evidence-based. The sidecar first checks raw payload fields such as `quote`, `referMsg`, `refMsg`, `reply`, `source`, and `appmsg`. If they are absent, it parses WeChat's visible quote text form as a fallback and marks `reply.raw.source = "text_fallback"`. If neither raw fields nor text fallback are present, `Message.Reply` is omitted.

Images are received through Wechaty `message.toFileBox()`, saved under `mediaDir`, and passed to Memoh as `Attachment{Type:image, Path, Mime, Name, Size}`. Outbound attachments are evaluated through the same sidecar protocol, but real WeChat file sending still depends on the account and `wechaty-puppet-wechat4u` filebox behavior.

## Verification

Run unit tests:

```bash
go test ./internal/channel/adapters/personalwechat
pnpm --filter @memohai/personal-wechat-bridge test
```

For real WeChat verification, enable `diagnosticRawPayload`, start the channel, scan the QR code printed by the sidecar, then send:

1. A private text message and a group mention.
2. A WeChat quote/reply message mentioning the bot.
3. An image.

Check logs for `message.raw` keys and media files in `mediaDir`. Report quote as verified only when raw quote fields are present; otherwise report text-fallback only.
