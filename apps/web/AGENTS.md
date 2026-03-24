# Web Frontend (apps/web)

## Overview

`@memohai/web` is the management UI for Memoh, built with Vue 3 + Vite. It provides visual configuration for bots, models, channels, memory, and more.

## Tech Stack

| Category | Technology |
|----------|-----------|
| Framework | Vue 3 (Composition API, `<script setup>`) |
| Build | Vite 8 + `@vitejs/plugin-vue` |
| CSS | Tailwind CSS 4 (CSS-based config, no `tailwind.config.*`) |
| UI Library | `@memohai/ui` (built on Reka UI + class-variance-authority) |
| State | Pinia 3 + `pinia-plugin-persistedstate` |
| Data Fetching | Pinia Colada (`@pinia/colada`) + `@memohai/sdk` |
| Forms | vee-validate + `@vee-validate/zod` + Zod |
| i18n | vue-i18n (en / zh) |
| Icons | FontAwesome (primary) + lucide-vue-next (secondary) |
| Toast | vue-sonner |
| Tables | @tanstack/vue-table |
| Markdown | markstream-vue + Shiki + Mermaid + KaTeX |
| Utilities | @vueuse/core |
| TypeScript | ~5.9 (strict mode) |

## Directory Structure

```
src/
├── App.vue                    # Root component (RouterView + Toaster)
├── main.ts                    # App entry (plugins, global components, API client setup)
├── router.ts                  # Route definitions and auth guard
├── style.css                  # Tailwind imports (delegates to @memohai/ui/style.css)
├── i18n.ts                    # vue-i18n configuration
├── assets/                    # Static assets
├── components/                # Shared components
│   ├── sidebar/               #   App sidebar navigation
│   ├── main-container/        #   Main content area (header + breadcrumb + content)
│   ├── master-detail-sidebar-layout/  # Master-detail layout pattern
│   ├── data-table/            #   TanStack table wrapper
│   ├── form-dialog-shell/     #   Dialog wrapper for forms
│   ├── confirm-popover/       #   Confirmation popover
│   ├── loading-button/        #   Button with loading state
│   ├── status-dot/            #   Status indicator dot
│   ├── warning-banner/        #   Warning banner
│   ├── search-provider-logo/  #   Search provider icons
│   ├── searchable-select-popover/  # Searchable dropdown
│   ├── add-platform/          #   Add platform dialog
│   ├── add-provider/          #   Add LLM provider dialog
│   ├── create-model/          #   Create model dialog
│   └── chat-list/             #   Chat list helpers
├── composables/               # Reusable composition functions
│   ├── api/                   #   API-related composables (chat, SSE, platform)
│   ├── useDialogMutation.ts   #   Mutation wrapper with toast error handling
│   ├── useRetryingStream.ts   #   SSE retry with exponential backoff
│   ├── useSyncedQueryParam.ts #   URL query param sync
│   ├── useBotStatusMeta.ts    #   Bot status metadata
│   ├── useAvatarInitials.ts   #   Avatar initial generation
│   ├── useClipboard.ts        #   Clipboard utilities
│   └── useKeyValueTags.ts     #   Tag management
├── constants/                 # Constants (client types, etc.)
├── i18n/locales/              # Translation files (en.json, zh.json)
├── layout/
│   └── main-layout/           # Top-level layout (SidebarProvider)
├── lib/
│   └── api-client.ts          # SDK client setup (base URL, auth interceptor)
├── pages/                     # Route page components
│   ├── login/                 #   Login page
│   ├── main-section/          #   Authenticated layout wrapper
│   ├── home/                  #   Home page
│   ├── chat/                  #   Chat interface (SSE streaming)
│   ├── bots/                  #   Bot list + detail (tabs: overview, memory, channels, etc.)
│   ├── models/                #   LLM provider & model management
│   ├── search-providers/      #   Search provider management
│   ├── email-providers/       #   Email provider management
│   ├── settings/              #   User settings (profile, password, theme, channels)
│   └── platform/              #   Platform management
├── store/                     # Pinia stores
│   ├── user.ts                #   User state, JWT token, login/logout
│   ├── settings.ts            #   UI settings (theme, language)
│   ├── capabilities.ts        #   Server capabilities (container backend)
│   └── chat-list.ts           #   Chat state, messages, SSE streaming
└── utils/                     # Utility functions
    ├── api-error.ts           #   API error message extraction
    ├── date-time.ts           #   Date/time formatting
    ├── channel-icons.ts       #   Channel platform icons
    └── key-value-tags.ts      #   Tag ↔ Record conversion
```

## Routes

| Path | Name | Component | Description |
|------|------|-----------|-------------|
| `/login` | Login | `login/index.vue` | Login form (no auth required) |
| `/chat` | chat | `chat/index.vue` | Chat interface with bot sidebar |
| `/home` | home | `home/index.vue` | Home dashboard |
| `/bots` | bots | `bots/index.vue` | Bot list grid |
| `/bots/:botId` | bot-detail | `bots/detail.vue` | Bot detail with tabs |
| `/models` | models | `models/index.vue` | LLM provider & model management |
| `/search-providers` | search-providers | `search-providers/index.vue` | Search provider management |
| `/email-providers` | email-providers | `email-providers/index.vue` | Email provider management |
| `/settings` | settings | `settings/index.vue` | User settings |
| `/platform` | platform | `platform/index.vue` | Platform management |

Auth guard: all routes except `/login` require `localStorage.getItem('token')`. Logged-in users accessing `/login` are redirected to `/chat`.

## Layout System

Three-tier layout architecture:

1. **MainLayout** (`layout/main-layout/`) — Top-level wrapper using `SidebarProvider` from `@memohai/ui`. Provides `#sidebar` and `#main` slots.
2. **Sidebar** (`components/sidebar/`) — Collapsible navigation with logo, menu items, and user avatar footer. Active route highlighting.
3. **MainContainer** (`components/main-container/`) — Header (sidebar trigger + breadcrumb) + scrollable content area with `<KeepAlive>` wrapped `<RouterView>`.

Several pages use **MasterDetailSidebarLayout** (`components/master-detail-sidebar-layout/`) for left-sidebar + detail-panel patterns (chat, models, search providers, email providers).

## CSS & Theming

Design tokens, color palette, typography, elevation strategy, and component visual specs are defined in `packages/ui/DESIGN.md`. **Read that file before making any UI changes.**

### Tailwind CSS 4

CSS-based configuration (no `tailwind.config.*` file). All design tokens (CSS variables, `@theme inline` mapping, base styles) live in `packages/ui/src/style.css`. The web app imports them via:

```css
@import "@memohai/ui/style.css";
```

### Dark Mode

- Runtime: `useColorMode` from `@vueuse/core` in `store/settings.ts`
- Storage: theme preference persisted via `useStorage`
- Toggle: Available in Settings page and login page
- Usage: semantic tokens auto-switch; no `dark:` prefix needed

### Styling Rules

- Use Tailwind utility classes; avoid `<style>` blocks.
- Always use semantic color tokens (`text-foreground`, `bg-card`, `border-border`, etc.) — never hardcode raw colors (`gray-*`, `bg-white`, `text-black`).
- Follow the design system rules in `packages/ui/DESIGN.md`.

## UI Components (@memohai/ui)

All UI primitives are provided by `@memohai/ui` (40+ components built on Reka UI). Do not import Reka UI directly. For the component design specification (variants, colors, elevation, spacing), see `packages/ui/DESIGN.md`.

### Form Pattern (vee-validate + Zod)

```vue
<script setup>
const form = useForm({
  validationSchema: toTypedSchema(z.object({
    name: z.string().min(1),
  })),
})
</script>

<template>
  <FormField v-slot="{ componentField }" name="name">
    <FormItem>
      <Label>Name</Label>
      <FormControl>
        <Input v-bind="componentField" />
      </FormControl>
      <FormMessage />
    </FormItem>
  </FormField>
</template>
```

### Icon Usage

- **FontAwesome** (primary): Global `<FontAwesomeIcon :icon="['fas', 'robot']" />`, icons registered in `main.ts`
- **Lucide** (secondary): Direct imports `<Sun />`, `<Moon />`, used for theme toggle

### Notification Pattern

```typescript
import { toast } from 'vue-sonner'
toast.success(t('common.saved'))
toast.error(resolveApiErrorMessage(error, 'Failed'))
```

## Data Fetching

### API Client Setup (`lib/api-client.ts`)

- SDK: `@memohai/sdk` auto-generated from OpenAPI via `@hey-api/openapi-ts`
- Base URL: `VITE_API_URL` env var (defaults to `/api`, proxied by Vite dev server to backend)
- Auth: Request interceptor attaches `Authorization: Bearer ${token}` from localStorage
- 401 handling: Response interceptor removes token and redirects to `/login`

### Pinia Colada (Server State)

Primary data fetching mechanism for CRUD operations:

```typescript
// Query — auto-generated from SDK
const { data, isLoading } = useQuery(getBotsQuery())

// Custom query with dynamic key
const { data } = useQuery({
  key: () => ['bot-settings', botId.value],
  query: async () => {
    const { data } = await getBotsByBotIdSettings({
      path: { bot_id: botId.value },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!botId.value,
})

// Mutation with cache invalidation
const queryCache = useQueryCache()
const { mutateAsync } = useMutation({
  mutation: async (body) => {
    const { data } = await putBotsByBotIdSettings({
      path: { bot_id: botId.value },
      body,
      throwOnError: true,
    })
    return data
  },
  onSettled: () => queryCache.invalidateQueries({
    key: ['bot-settings', botId.value],
  }),
})
```

SDK also generates colada helpers: `getBotsQuery()`, `postBotsMutation()`, query key factories.

### Pinia Stores (Client State)

| Store | Purpose |
|-------|---------|
| `user` | JWT token (`useLocalStorage`), user info, login/logout |
| `settings` | Theme (dark/light), language (en/zh), persisted |
| `capabilities` | Server feature flags (container backend, snapshot support) |
| `chat-list` | Chat messages, streaming state, SSE event processing |

Stores use Composition API style (`defineStore(() => { ... })`), with persistence via `pinia-plugin-persistedstate`.

### SSE Streaming (Chat)

Chat responses are streamed via Server-Sent Events:

- **Endpoints**: `/bots/{bot_id}/web/stream` (chat), `/bots/{bot_id}/messages/events` (real-time updates)
- **Parsing**: `composables/api/useChat.sse.ts` reads `ReadableStream<Uint8Array>` and parses SSE `data:` lines
- **Events**: `text_delta`, `reasoning_delta`, `tool_call_start/end`, `attachment_delta`, `processing_completed/failed`
- **Retry**: `useRetryingStream` composable provides exponential backoff for reconnection
- **State**: `store/chat-list.ts` processes streaming events into reactive message blocks in real-time
- **Abort**: Stream cancellation via `AbortSignal`

### Error Handling

- **Global**: `utils/api-error.ts` — `resolveApiErrorMessage()` extracts error from `message`, `error`, `detail` fields
- **Mutations**: `useDialogMutation` composable wraps mutations with automatic `toast.error()` on failure
- **SDK**: All calls use `throwOnError: true`; try/catch at component level
- **Streams**: `processing_failed` / `error` events appended to message blocks

## i18n

- Plugin: vue-i18n (Composition API, `legacy: false`)
- Locales: `en` (English, default), `zh` (Chinese)
- Files: `src/i18n/locales/en.json`, `src/i18n/locales/zh.json`
- Usage: `const { t } = useI18n()` → `t('bots.title')`
- Key namespaces: `common`, `auth`, `sidebar`, `settings`, `chat`, `models`, `provider`, `searchProvider`, `emailProvider`, `mcp`, `bots`, `home`

## Vite Configuration

- Dev server port: 8082 (from `config.toml`)
- Proxy: `/api` → backend (default `http://localhost:8080`)
- Aliases: `@` → `./src`, `#` → `../ui/src`
- Config: reads from `MEMOH_CONFIG_PATH` / `CONFIG_PATH` when provided, otherwise `../../config.toml`, via `@memohai/config`

## Development Rules

- Use Vue 3 Composition API with `<script setup>` exclusively.
- Style with Tailwind utility classes; avoid `<style>` blocks. Follow the design system in `packages/ui/DESIGN.md`.
- Always use semantic color tokens (`text-foreground`, `bg-card`, `border-border`, etc.) — never hardcode raw colors.
- Use `@memohai/ui` components for all UI primitives; do not import Reka UI directly.
- Use Pinia Colada (`useQuery`/`useMutation`) for server state; use Pinia stores for client state only.
- API calls must go through `@memohai/sdk`; never call `fetch()` directly.
- All user-facing strings must use i18n keys (`t('key')`) — never hardcode text.
- Forms must use vee-validate + Zod schemas via `toTypedSchema()`.
- Error messages via `resolveApiErrorMessage()` + `toast.error()`.
- Page components go in `pages/{feature}/`; page-specific sub-components go in `pages/{feature}/components/`.
- Shared components go in `components/`.
- Composables go in `composables/`; API-specific composables in `composables/api/`.
