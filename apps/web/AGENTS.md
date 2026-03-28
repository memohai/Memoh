# Web Frontend (apps/web)

## Overview

`@memohai/web` is the management UI for Memoh, built with Vue 3 + Vite. It provides a chat interface for interacting with bots, plus visual configuration for bots, models, channels, memory, and more.

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
| Icons | FontAwesome (primary: fas/far/fab) + lucide-vue-next (secondary) |
| Toast | vue-sonner |
| Tables | @tanstack/vue-table |
| Markdown | markstream-vue + Shiki + Mermaid + KaTeX |
| Utilities | @vueuse/core |
| TypeScript | ~5.9 (strict mode) |

## Directory Structure

```
src/
├── App.vue                    # Root component (RouterView + Toaster + settings init)
├── main.ts                    # App entry (plugins, global components, API client setup)
├── router.ts                  # Route definitions, auth guard, chunk error recovery
├── style.css                  # Tailwind imports (delegates to @memohai/ui/style.css)
├── i18n.ts                    # vue-i18n configuration
├── assets/                    # Static assets (logo.png, vue.svg)
├── components/                # Shared components
│   ├── sidebar/               #   Bot list sidebar (collapsible, bot items, settings link)
│   │   ├── index.vue          #     Sidebar with bot list + settings gear footer
│   │   └── bot-item.vue       #     Individual bot entry in sidebar
│   ├── settings-sidebar/      #   Settings section sidebar (back-to-chat + nav items)
│   ├── main-container/        #   Main content area (KeepAlive RouterView)
│   ├── master-detail-sidebar-layout/  # Master-detail layout pattern
│   ├── chat-list/             #   Chat list helpers
│   │   └── channel-badge/     #     Channel badge component
│   ├── chat/                  #   Chat UI sub-components
│   │   ├── chat-status/       #     Chat connection status indicator
│   │   └── chat-step/         #     Chat processing step indicator
│   ├── data-table/            #   TanStack table wrapper
│   ├── file-manager/          #   File browser (list + viewer)
│   ├── monaco-editor/         #   Monaco code editor wrapper
│   ├── form-dialog-shell/     #   Dialog wrapper for forms
│   ├── confirm-popover/       #   Confirmation popover
│   ├── loading-button/        #   Button with loading state
│   ├── status-dot/            #   Status indicator dot
│   ├── warning-banner/        #   Warning banner
│   ├── channel-icon/          #   Channel platform icon
│   ├── provider-icon/         #   LLM provider icon (icons.ts + index.vue)
│   ├── search-provider-logo/  #   Search provider icons (custom-icons.ts + index.vue)
│   ├── searchable-select-popover/  # Searchable dropdown
│   ├── timezone-select/       #   Timezone selector
│   ├── key-value-editor/      #   Key-value pair editor
│   ├── import-models-dialog/  #   Bulk model import dialog
│   ├── add-platform/          #   Add platform dialog
│   ├── add-provider/          #   Add LLM provider dialog
│   └── create-model/          #   Create model dialog
├── composables/               # Reusable composition functions
│   ├── api/                   #   API-related composables
│   │   ├── useChat.ts         #     Aggregated re-export of chat composables
│   │   ├── useChat.types.ts   #     Bot, Session, Message, StreamEvent types
│   │   ├── useChat.chat-api.ts  #   Bot/session CRUD (fetchBots, fetchSessions, etc.)
│   │   ├── useChat.message-api.ts  # Message fetch, SSE streaming, local channel
│   │   ├── useChat.sse.ts     #     SSE stream reader and parser
│   │   ├── useChat.ws.ts      #     WebSocket connection (send, abort, reconnect)
│   │   ├── useChat.content.ts #     Message content parsing (tool calls, text, reasoning)
│   │   ├── useChat.sse.test.ts  #   SSE parser tests
│   │   ├── useContainerStream.ts  # Container creation SSE stream
│   │   └── usePlatform.ts     #     Platform list query + create mutation
│   ├── useDialogMutation.ts   #   Mutation wrapper with toast error handling
│   ├── useRetryingStream.ts   #   SSE retry with exponential backoff
│   ├── useSyncedQueryParam.ts #   URL query param sync
│   ├── useBotStatusMeta.ts    #   Bot status metadata
│   ├── useAvatarInitials.ts   #   Avatar initial generation
│   ├── useClipboard.ts        #   Clipboard utilities
│   ├── useKeyValueTags.ts     #   Tag management
│   ├── useShikiHighlighter.ts #   Shiki syntax highlighter singleton
│   └── useTerminalCache.ts    #   Terminal output cache
├── constants/                 # Constants
│   ├── client-types.ts        #   LLM client type definitions
│   └── compatibilities.ts     #   Feature compatibility flags
├── i18n/locales/              # Translation files (en.json, zh.json)
├── layout/
│   └── main-layout/           # Top-level layout (SidebarProvider)
├── lib/
│   └── api-client.ts          # SDK client setup (base URL, auth interceptor)
├── pages/                     # Route page components
│   ├── login/                 #   Login page
│   ├── main-section/          #   Chat section layout (bot sidebar + main container)
│   ├── settings-section/      #   Settings section layout (settings sidebar + KeepAlive)
│   ├── home/                  #   Chat interface (used by both `/` and `/chat/:botId?/:sessionId?`)
│   │   ├── index.vue          #     Route ↔ store sync, session sidebar + chat area
│   │   ├── composables/       #     Page-specific composables
│   │   │   ├── useFileManagerProvider.ts  # File manager context
│   │   │   └── useMediaGallery.ts         # Media gallery state
│   │   └── components/        #     Chat UI components (26 files)
│   │       ├── chat-area.vue          # Main chat area (messages, input, attachments)
│   │       ├── session-sidebar.vue    # Session list sidebar (search, filter, CRUD)
│   │       ├── bot-sidebar.vue        # Alternative bot sidebar layout
│   │       ├── chat-header.vue        # Chat top bar (status, step indicator)
│   │       ├── message-item.vue       # Single message (user/assistant, markdown, blocks)
│   │       ├── session-item.vue       # Session list row (avatar, title, timestamp)
│   │       ├── session-metadata.vue   # Collapsible session metadata panel
│   │       ├── bot-item.vue           # Bot item in sidebar
│   │       ├── thinking-block.vue     # Collapsible thinking/reasoning block
│   │       ├── attachment-block.vue   # Attachment grid (images, audio, files)
│   │       ├── media-gallery-lightbox.vue  # Fullscreen media lightbox
│   │       ├── tool-call-generic.vue  # Generic tool call (name, status, JSON I/O)
│   │       ├── tool-call-list.vue     # File listing tool display
│   │       ├── tool-call-read.vue     # File read tool display
│   │       ├── tool-call-write.vue    # File write tool display
│   │       ├── tool-call-edit.vue     # File edit tool display
│   │       ├── tool-call-exec.vue     # Command execution tool display
│   │       ├── tool-call-web-search.vue    # Web search tool display
│   │       ├── tool-call-web-fetch.vue     # Web fetch tool display
│   │       ├── tool-call-browser.vue       # Browser action tool display
│   │       ├── tool-call-memory.vue        # Memory read/write tool display
│   │       ├── tool-call-message.vue       # Send message tool display
│   │       ├── tool-call-email.vue         # Email tool display
│   │       ├── tool-call-schedule.vue      # Schedule tool display
│   │       ├── tool-call-contacts.vue      # Contacts tool display
│   │       ├── tool-call-subagent.vue      # Sub-agent tool display
│   │       └── tool-call-skill.vue         # Skill activation tool display
│   ├── bots/                  #   Bot list + detail (tabs: overview, memory, channels, etc.)
│   │   ├── index.vue          #     Bot grid with create dialog
│   │   ├── detail.vue         #     Bot detail with tabbed interface
│   │   └── components/        #     Bot sub-components (25+ files)
│   ├── models/                #   LLM provider & model management
│   ├── search-providers/      #   Search provider management (12+ engine configs)
│   ├── memory-providers/      #   Memory provider management
│   ├── tts-providers/         #   TTS provider & model management
│   ├── email-providers/       #   Email provider management
│   ├── browser-contexts/      #   Browser context management
│   ├── usage/                 #   Token usage statistics
│   ├── settings/              #   User settings (profile, password, bind codes)
│   ├── platform/              #   Platform management
│   └── oauth/                 #   OAuth callback pages
│       └── mcp-callback.vue   #     MCP OAuth callback handler
├── store/                     # Pinia stores
│   ├── user.ts                #   User state, JWT token, login/logout
│   ├── settings.ts            #   UI settings (theme, language)
│   ├── capabilities.ts        #   Server capabilities (container backend)
│   ├── chat-selection.ts      #   Current bot/session selection (localStorage persisted)
│   └── chat-list.ts           #   Chat messages, streaming state, SSE/WS event processing
└── utils/                     # Utility functions
    ├── api-error.ts           #   API error message extraction
    ├── date-time.ts           #   Date/time formatting
    ├── date-time.test.ts      #   Date/time tests
    ├── channel-icons.ts       #   Channel platform icons
    ├── key-value-tags.ts      #   Tag ↔ Record conversion
    ├── key-value-tags.test.ts #   Tag conversion tests
    ├── image-ref.ts           #   Image reference URL resolution
    ├── image-ref.test.ts      #   Image ref tests
    ├── timezones.ts           #   Timezone list and utilities
    └── useControlVisibleStatus.ts  # Visibility control utility
```

## Routes

The app uses a two-section layout architecture:

### Chat Section (`/`)

| Path | Name | Component | Description |
|------|------|-----------|-------------|
| `/` | home | `home/index.vue` | Home — empty state when no bot selected |
| `/chat/:botId?/:sessionId?` | chat | `home/index.vue` | Chat interface with bot + session params |

Both routes render the same `home/index.vue` component. The `home` route shows an empty state; the `chat` route auto-selects a bot and optionally a session based on URL params. URL and store state are bidirectionally synced.

### Settings Section (`/settings`)

| Path | Name | Component | Description |
|------|------|-----------|-------------|
| `/settings/bots` | bots | `bots/index.vue` | Bot list grid |
| `/settings/bots/:botId` | bot-detail | `bots/detail.vue` | Bot detail with tabs |
| `/settings/models` | models | `models/index.vue` | LLM provider & model management |
| `/settings/search-providers` | search-providers | `search-providers/index.vue` | Search provider management |
| `/settings/memory-providers` | memory-providers | `memory-providers/index.vue` | Memory provider management |
| `/settings/tts-providers` | tts-providers | `tts-providers/index.vue` | TTS provider & model management |
| `/settings/email-providers` | email-providers | `email-providers/index.vue` | Email provider management |
| `/settings/browser-contexts` | browser-contexts | `browser-contexts/index.vue` | Browser context management |
| `/settings/usage` | usage | `usage/index.vue` | Token usage statistics |
| `/settings/profile` | settings | `settings/index.vue` | User profile settings |
| `/settings/platform` | platform | `platform/index.vue` | Platform management |

`/settings` redirects to `/settings/bots` by default.

### Standalone Routes

| Path | Name | Component | Description |
|------|------|-----------|-------------|
| `/login` | Login | `login/index.vue` | Login form (no auth required) |
| `/oauth/mcp/callback` | oauth-mcp-callback | `oauth/mcp-callback.vue` | MCP OAuth callback (no auth required) |

### Auth Guard

- All routes except `/login` and `/oauth/*` require `localStorage.getItem('token')`.
- Logged-in users accessing `/login` are redirected to `/`.
- Chunk load errors (dynamic import failures) trigger an automatic page reload.
- Tauri integration: `afterEach` hook calls `resize_for_route` when running inside Tauri.

## Layout System

Two-section layout architecture, both sharing the same `MainLayout` wrapper:

1. **MainLayout** (`layout/main-layout/`) — Top-level wrapper using `SidebarProvider` from `@memohai/ui`. Provides `#sidebar` and `#main` slots.

2. **Chat Section** (`pages/main-section/`) — Uses `MainLayout` with:
   - **Sidebar** (`components/sidebar/`) — Bot list sidebar (collapsible). Header shows "Bots" label + create button. Body lists all bots as `BotItem` entries. Footer has a settings gear link to `/settings`.
   - **MainContainer** (`components/main-container/`) — `<KeepAlive>` wrapped `<RouterView>` for chat pages.

3. **Settings Section** (`pages/settings-section/`) — Uses `MainLayout` with:
   - **SettingsSidebar** (`components/settings-sidebar/`) — Collapsible settings navigation. Top has a "back to chat" button that restores the last selected bot/session. Menu items: Bots, Models, Search Providers, Memory Providers, TTS Providers, Email Providers, Browser Contexts, Usage, Settings, Platform.
   - **SidebarInset** — `<KeepAlive>` wrapped `<RouterView>` for settings pages.

4. **Home/Chat Page** (`pages/home/`) — Internal layout:
   - **SessionSidebar** — Left panel: session search, source filter, new session button, session list.
   - **ChatArea** — Center panel: message list with scroll, input area with attachments.
   - **SessionMetadata** — Right panel (currently disabled): collapsible session metadata.

Several settings pages use **MasterDetailSidebarLayout** (`components/master-detail-sidebar-layout/`) for left-sidebar + detail-panel patterns (models, search providers, email providers, memory providers, tts providers, browser contexts).

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

All UI primitives are provided by `@memohai/ui` (43 component groups built on Reka UI). Do not import Reka UI directly. For the component design specification (variants, colors, elevation, spacing), see `packages/ui/DESIGN.md`.

- **Exception**: Physical UI knobs (Switch thumb, Slider thumb) may keep `bg-white` as they need to contrast against colored tracks regardless of theme.
- **No scoped CSS modules**: Styling is done inline via utility classes.

### CSS Imports (main.ts)

```
style.css                    — Tailwind + theme tokens
markstream-vue/index.css     — Markdown rendering
katex/dist/katex.min.css     — Math rendering
vue-sonner/style.css         — Toast notifications (in App.vue)
```

`@memohai/ui` provides 43 component groups built on Reka UI primitives + Tailwind + class-variance-authority:

- **Form**: `Form`, `FormField`, `FormFieldArray`, `FormItem`, `FormControl`, `FormLabel`, `FormMessage`, `FormDescription`
- **Input**: `Input`, `Textarea`, `InputGroup` (Addon, Button, Input, Text, Textarea), `NativeSelect`, `Combobox`, `TagsInput`, `InputOTP` (Group, Slot, Separator)
- **Selection**: `Select`, `RadioGroup`, `Checkbox`, `Switch`, `Toggle`, `Slider`
- **Layout**: `Card`, `Separator`, `Sheet`, `Sidebar` (24 sub-components), `ScrollArea`, `Collapsible`, `Item` (10 sub-components)
- **Overlays**: `Dialog` (incl. `DialogScrollContent`), `Popover`, `Tooltip`, `DropdownMenu`, `ContextMenu`, `Command` (Dialog, Group, Input, Item, List)
- **Data**: `Table` (9 sub-components), `Badge`, `BadgeCount`, `Avatar`, `Skeleton`, `Empty` (5 sub-components)
- **Navigation**: `Breadcrumb`, `Tabs`, `Pagination`, `PinInput` (Group, Slot, Separator)
- **Feedback**: `Button`, `ButtonGroup` (Separator, Text), `Spinner`, `Alert`, `Toaster` (Sonner), `Kbd`
- **Effects**: `TextGenerateEffect`

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

- **FontAwesome** (primary): Global `<FontAwesomeIcon :icon="['fas', 'robot']" />`, full `fas`/`far`/`fab` sets + custom search icons registered in `main.ts`
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

| Store | ID | Purpose |
|-------|----|---------|
| `user` | `user` | JWT token (`useLocalStorage`), user info (id, username, role, displayName, avatarUrl, timezone), login/logout |
| `settings` | `settings` | Theme (dark/light), language (en/zh), synced with `useColorMode` and vue-i18n locale |
| `capabilities` | `capabilities` | Server feature flags (container backend, snapshot support), loaded once from `getPing()` |
| `chat-selection` | `chat-selection` | Current bot ID and session ID, persisted via `useStorage` to localStorage |
| `chat-list` | `chat` | Chat messages, sessions, bots, streaming state, SSE/WS event processing. Depends on `chat-selection` store for current bot/session |

Stores use Composition API style (`defineStore(() => { ... })`), with persistence via `pinia-plugin-persistedstate` or `useStorage`.

### Streaming (Chat)

Chat supports two transport modes: **Server-Sent Events (SSE)** and **WebSocket**.

#### SSE Streaming
- **Endpoints**: `/bots/{bot_id}/web/stream` (send + stream), `/messages/events` (real-time message updates)
- **Parsing**: `composables/api/useChat.sse.ts` reads `ReadableStream<Uint8Array>` and parses SSE `data:` lines
- **Events**: `text_delta`, `reasoning_delta`, `tool_call_start/end`, `attachment_delta`, `processing_completed/failed`
- **Retry**: `useRetryingStream` composable provides exponential backoff for reconnection

#### WebSocket
- **Endpoint**: `/bots/{bot_id}/web/ws` (with token query param)
- **Implementation**: `composables/api/useChat.ws.ts` wraps native `WebSocket` with send, abort, close, and auto-reconnect
- **State**: `store/chat-list.ts` processes streaming events from either transport into reactive message blocks in real-time
- **Abort**: Stream cancellation via `AbortSignal` (SSE) or close message (WS)

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
- **Always use semantic color tokens** (`text-foreground`, `bg-card`, `border-border`, `text-muted-foreground`, `bg-accent`, etc.) instead of raw colors (`gray-*`, `bg-white`, `text-black`). Never introduce hardcoded Tailwind color classes for themed elements — this breaks dark mode consistency.
- Use `@memohai/ui` components for all UI primitives; do not import Reka UI directly.
- Use Pinia Colada (`useQuery`/`useMutation`) for server state; use Pinia stores for client state only.
- API calls must go through `@memohai/sdk`; never call `fetch()` directly.
- All user-facing strings must use i18n keys (`t('key')`) — never hardcode text.
- Forms must use vee-validate + Zod schemas via `toTypedSchema()`.
- Error messages via `resolveApiErrorMessage()` + `toast.error()`.
- Page components go in `pages/{feature}/`; page-specific sub-components go in `pages/{feature}/components/`.
- Page-specific composables go in `pages/{feature}/composables/`.
- Shared components go in `components/`.
- Composables go in `composables/`; API-specific composables in `composables/api/`.
