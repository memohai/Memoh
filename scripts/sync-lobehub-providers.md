Run with `bun scripts/sync-lobehub-providers.ts`.

Optional:

- `LOBEHUB_PATH=/path/to/lobe-chat bun scripts/sync-lobehub-providers.ts`

How it works:

- reads the current `conf/providers/*.yaml`
- keeps the existing provider header (`name`, `client_type`, `icon`, `base_url`)
- loads the full upstream provider model list from `packages/model-bank/src/aiModels/<provider>.ts`
- syncs all upstream models that Memoh currently supports: `chat` and `embedding`
- drops upstream-only unsupported model types like `image`, `tts`, `stt`, and `realtime`

This syncs:

- `conf/providers/openai.yaml`
- `conf/providers/anthropic.yaml`
- `conf/providers/google.yaml`
