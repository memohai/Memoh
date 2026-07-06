# Spacing Coverage-Closure Sweep

Date: 2026-07-02
Method: 14 sonnet subagents, one per apps/web surface domain, each read EVERY
`.vue` in its directory (282 files total, 0 skimmed) and classified every
row/field/card/tile shape as MISS / LOCAL / DIFF-RELATIONSHIP with file+line+class
evidence. This is the coverage closure the earlier click-surface audit could not
provide: that audit selected 40 files by grepping `<Dialog>`/`<Popover>`, so any
surface opened by routing / `v-if` / `<component :is>` — and any hand-rolled shape
*inside* an already-migrated file — was invisible to it. Raw output archived at
`tasks/ws5gg3e4l.output` (session transcript dir).

## Headline: 49 new MISS across 14 domains, 282 files fully read

The two systemic blind spots this proves:
- **File-level grading misses in-file residue.** bots had 44 files already migrated,
  yet 7 hand-rolled owner-shapes survive *inside* migrated files (bot-network's three
  status-line states, bot-overview's metric grid, bot-heartbeat's advanced + danger
  rows). Only a per-row read finds these.
- **Whole domains were never audited.** onboarding (11), web-search (7), people (7)
  never entered the earlier 40-file list at all.

## Coverage per domain

| Domain | MISS | Deep-read | Already on owners |
|---|---:|---|---:|
| onboarding | 11 | 6/6 | 0 |
| profile-account | 11 | 7/7 | 4 |
| bots | 7 | 51/51 | 44 |
| web-search | 7 | 20/20 | 14 |
| provider-domains | 5 | 15/15 | 9 |
| supermarket | 4 | 7/7 | 2 |
| providers-models | 2 | 13/13 | 6 |
| shell-misc-pages | 2 | 10/10 | 7 |
| home | 0 | 79/79 | 5 |
| sidebar-shell | 0 | 16/16 | 0 |
| file-manager | 0 | 3/3 | 0 |
| settings-owners | 0 | 9/9 | 9 |
| atoms-misc | 0 | 24/24 | 2 |
| dev-wall | 0 | 20/20 | 1 |

## All MISS findings

### bots (7)

- **`pages/bots/components/bot-heartbeat.vue:36`** — hand-rolled settings row: label + outline chevron-button that toggles a sibling 'Advanced' block below
  - → **SettingsRow (label prop + button in default slot — same pattern already used correctly two lines away at L19 and in the sibling file bot-network.vue L227 / bot-access.vue L248)**
  - reach: Heartbeat tab, top SettingsSection, second row when heartbeat_enabled is true
  - `mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3 last:border-b-0`
  - why: Byte-identical to SettingsRow's own root class (mx-4 flex min-h-[3.75rem] border-b border-border py-3 last:border-b-0, row.vue L3) composed as a raw div instead of the component. The sibling file bot-compaction.vue solves the exact same 'Model is a power-user override, folded behind Advanced' requirement with ExpandableSettingsRow/SettingsRow, proving the owner-based version already works for this precise use case.
- **`pages/bots/components/bot-heartbeat.vue:285`** — hand-rolled danger-zone row: destructive label + description on the left, a destructive Button (behind ConfirmPopover) on the right
  - → **SettingsRow (#content slot for the destructive-colored label/description, default slot for the ConfirmPopover+Button — identical to bot-compaction.vue's danger-zone row and the shared settings-danger-zone.vue component)**
  - reach: Heartbeat tab, bottom 'Danger Zone' SettingsSection (only rendered when logs.length > 0)
  - `mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 py-3`
  - why: This is the exact same clear-logs danger-zone row that bot-compaction.vue (a near-identical sibling file covering the same enable/threshold/ratio/danger-zone shape) implements correctly as `<SettingsRow :label :description><ConfirmPopover>...`. bot-heartbeat hand-rolls it as a raw div instead.
- **`pages/bots/components/bot-network.vue:41`** — hand-rolled status-line row: status dot + muted text, sitting between two SettingsRow siblings inside the same SettingsSection
  - → **SettingsRow #content slot (the component's own doc comment names this exact case: 'a fully custom block (#content, for status lines, nested meta...)')**
  - reach: Network tab, overlay SettingsSection, shown when needsSaveForStatus is true (pending-save state)
  - `mx-4 flex min-h-[3.75rem] items-center gap-2.5 border-b border-border py-3`
  - why: Same file already uses SettingsRow extensively (10+ occurrences) including for the sibling states of this very status line's logical row; this pending-save variant and the two below it (loading, resolved) are raw divs replicating SettingsRow's exact geometry instead of using #content.
- **`pages/bots/components/bot-network.vue:48`** — hand-rolled status-line row: spinner + 'loading' text, second mutually-exclusive state of the same logical status row as line 41 and line 55
  - → **SettingsRow #content slot**
  - reach: Network tab, overlay SettingsSection, shown while isNetworkStatusLoading
  - `mx-4 flex min-h-[3.75rem] items-center gap-2 border-b border-border py-3 text-sm text-muted-foreground`
  - why: Same duplication as line 41 — one logical row (network connection status) rendered as three hand-rolled divs with identical class-string geometry instead of one SettingsRow with conditional #content.
- **`pages/bots/components/bot-network.vue:57`** — hand-rolled status-line row: status dot + two-line label/detail + trailing action buttons, third mutually-exclusive state of the same logical status row
  - → **SettingsRow (#content for the dot+label+detail block, default slot for the trailing Button group)**
  - reach: Network tab, overlay SettingsSection, shown when networkStatusLine resolves (connected/degraded/needs-login/etc.)
  - `mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3`
  - why: This is the richest of the three status-line states and the clearest SettingsRow fit: label+detail belongs in #content, the 'Open login page' / 'Details' buttons belong in the default trailing slot — exactly SettingsRow's two-slot contract, just not composed as the component.
- **`pages/bots/components/bot-overview.vue:168`** — hand-rolled metric tile: framed box with caption label, tabular value, optional sub-line — repeated 3x in a grid
  - → **MetricReadout (framed=true, the default) — its own template root class is `flex min-h-[4.375rem] flex-col rounded-[var(--radius-menu-shell)] border border-border bg-card p-3` and it renders exactly this caption/value/sub structure**
  - reach: Overview tab, 'Runtime' section, shown when runtimeHasMetrics is true (container-backed bot)
  - `rounded-[var(--radius-menu-shell)] border border-border bg-card px-3 py-2.5`
  - why: The sibling file bot-container.vue in the same directory renders the identical CPU/memory/storage metric-tile grid using `<MetricReadout v-for="m in runtimeMetricCards">` (already migrated). bot-overview.vue computes the same runtimeMetricCards shape (key/label/value/sub) but renders it with a hand-written div grid instead of composing the owner it sits two files away from.
- **`pages/bots/components/bot-hooks.vue:2`** — hand-rolled page shell: full 'title + description(via subtitle-in-header) + actions toolbar' header, plus the centered max-w-3xl column, replicated instead of using PageShell
  - → **PageShell (variant='tab'), whose own rootClass for variant='tab' is byte-identical: `mx-auto max-w-3xl pt-6 pb-8` (page-shell/index.vue L54), with a `#actions` slot for the reload/save button cluster and a `title`/`description` prop pair for the h2+p**
  - reach: Hooks tab — the entire page root, before any SettingsSection
  - `mx-auto max-w-3xl space-y-6 pt-6 pb-8 (root) / flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between (header)`
  - why: Every other bot-detail tab in this same directory (bot-heartbeat, bot-compaction, bot-memory, bot-schedule, bot-tool-approval, etc.) opens with `<PageShell variant="tab" :title=...>` and puts header actions in `#actions`. bot-hooks.vue is the one tab in the whole domain that hand-rolls its own header/shell instead, duplicating PageShell's exact column width and vertical padding.

### web-search (7)

- **`pages/web-search/components/add-search-provider.vue:31`** — raw <FormItem><Label class="mb-2"><FormControl> stacked field — exactly the pre-FieldStack pattern the skill names as superseded
  - → **FieldStack (wrap the pair in FormStack for the two-field dialog body)**
  - reach: Web Search root list → "Add" button → AddSearchProvider FormDialogShell #body (also reachable via AddWebProvider's merged dialog is a separate component, not this one)
  - `<FormItem> ... <Label class="mb-2" :for="'search-provider-create-name'"> ... <FormControl><Input .../></FormControl></FormItem> (repeated at line 53 for the provider Select)`
  - why: This is a vee-validate FormField wrapping a Label-above-Input field — the skill's decision map (`A stacked form field (label above control)? → FieldStack`) and the explicit note that a validated field's pattern is FormField > FieldStack > FormControl. Two occurrences in this file (name field, provider Select field).
- **`pages/web-search/components/add-fetch-provider.vue:31`** — same raw <FormItem><Label class="mb-2"><FormControl> stacked field, duplicated from add-search-provider.vue
  - → **FieldStack (wrap in FormStack)**
  - reach: Web Search root list → "Add" button (fetch section) → AddFetchProvider FormDialogShell #body
  - `<FormItem> ... <Label class="mb-2" :for="'fetch-provider-create-name'"> ... <FormControl><Input .../></FormControl></FormItem> (repeated at line 53 for the provider Select)`
  - why: Identical shape/relationship to add-search-provider.vue's MISS above — same-shape-different-code duplicated a third time across the three add-*-provider dialogs.
- **`pages/web-search/components/add-web-provider.vue:31`** — same raw <FormItem><Label class="mb-2"><FormControl> stacked field, a third copy
  - → **FieldStack (wrap in FormStack)**
  - reach: Web Search root — this component isn't wired into index.vue currently (index.vue uses AddSearchProvider/AddFetchProvider directly), but it exists as a merged add-dialog and duplicates the same field shape a third time
  - `<FormItem> ... <Label class="mb-2" for="web-provider-create-name"> ... <FormControl><Input .../></FormControl></FormItem> (repeated at line 53 for the target Select)`
  - why: Same relationship as the other two add dialogs — three copies of the same hand-rolled field instead of one FieldStack usage. Flagging for completeness even though its current wiring into the page is unclear from index.vue alone.
- **`pages/web-search/components/provider-setting.vue:55`** — raw <FormItem class="w-80"><FormControl><Input/></FormControl><FormMessage/></FormItem> inside a SettingsRow's default slot, instead of a validated FieldStack
  - → **FieldStack is not the right owner here since label sits beside control via SettingsRow already (this is the SettingsRow+trailing-control shape, correctly using SettingsRow) — but the trailing control itself still leans on the superseded FormItem/FormMessage combo instead of the plain validated-input pattern; at minimum FormMessage should render through the row consistently with other validated SettingsRow trailing controls elsewhere in the app. Flagging as DIFF-RELATIONSHIP/minor rather than a clean MISS — see note.**
  - reach: Web Search root → click a search-provider BackendCard → ProviderSetting detail pane → Configuration section, Name row
  - `<FormItem class="w-80"><FormControl><Input ... v-bind="componentField" /></FormControl><FormMessage /></FormItem>`
  - why: Not a clean FieldStack miss (SettingsRow already owns the row), but the bare FormItem/FormControl/FormMessage trio for the trailing control is the older validated-field idiom; noting it because it's the same file that also has the footer-band miss below, and both derive from not fully adopting the current form idioms.
- **`pages/web-search/components/provider-setting.vue:114`** — hand-rolled footer action band inside SettingsSection instead of using the #footer slot
  - → **SettingsSection #footer slot**
  - reach: Web Search root → click a search-provider BackendCard → ProviderSetting detail pane → bottom of Configuration SettingsSection (Save button)
  - `<div class="mx-4 flex items-center justify-end border-t border-border py-3">`
  - why: This is exactly the shape SettingsSection's #footer slot exists for: a top hairline + justify-end action band holding a single Save button — the skill states '#footer is justify-end. It fits Save/Cancel.' This is a plain Save button, not a justify-between pagination strip, so it fits the existing #footer contract with no owner-growth needed. The section is otherwise already SettingsSection/SettingsRow-based, so this is a partial migration that missed the footer.
- **`pages/web-search/components/fetch-provider-setting.vue:65`** — raw <FormItem class="w-80"><FormControl><Input/></FormControl><FormMessage/></FormItem> inside a SettingsRow's default slot — identical to provider-setting.vue
  - → **same note as provider-setting.vue line 55 — not a clean FieldStack miss since SettingsRow owns the row shape; flagged alongside the footer miss below for the same partial-migration pattern**
  - reach: Web Search root → click a fetch-provider BackendCard → FetchProviderSetting detail pane → Configuration section, Name row (non-native providers only)
  - `<FormItem class="w-80"><FormControl><Input ... v-bind="componentField" /></FormControl><FormMessage /></FormItem>`
  - why: Same duplicated idiom as provider-setting.vue — the two detail panes were clearly copy-pasted from each other, so this is the same shape appearing a second time.
- **`pages/web-search/components/fetch-provider-setting.vue:93`** — hand-rolled footer action band inside SettingsSection instead of using the #footer slot, identical to provider-setting.vue
  - → **SettingsSection #footer slot**
  - reach: Web Search root → click a fetch-provider BackendCard → FetchProviderSetting detail pane → bottom of Configuration SettingsSection (Save button, non-native providers only)
  - `<div class="mx-4 flex items-center justify-end border-t border-border py-3">`
  - why: Same footer-band miss as provider-setting.vue line 114, copy-pasted into the fetch-provider twin — a single Save button inside a justify-end band that SettingsSection's #footer already covers.

### providers-models (2)

- **`pages/providers/model-setting.vue:4`** — Identity header card: leading avatar/icon + truncated title + trailing action cluster (delete + toggle), inside its own bordered rounded card, sitting above a SettingsSection form
  - → **No current owner exactly fits (closest is BackendCard, but BackendCard's root is a clickable <button> for object-picking, not a static detail header) — this is a strong candidate for a NEW 'ObjectHeaderCard' owner. At minimum it should not keep being copy-pasted.**
  - reach: pages/providers/index.vue → DetailPane → ModelSetting (opened via openDetail() when a provider card is clicked)
  - `class="flex items-center gap-3 rounded-[var(--radius-menu-shell)] border border-border bg-card px-4 py-3"`
  - why: This exact class string plus the same icon+title+trailing-actions structure is duplicated verbatim — including matching code comments that literally say 'same header shape as the provider detail' / 'same header shape as the provider / web search / voice details' — in pages/speech/components/provider-setting.vue, pages/video/provider-setting.vue, pages/web-search/components/provider-setting.vue, pages/web-search/components/fetch-provider-setting.vue, pages/email/components/provider-setting.vue, and referenced again in pages/bots/components/{bot-acp,bot-container,settings-acp-detail,channel-settings-panel}.vue and pages/bots/detail.vue. This is textbook same-shape-different-code debt (同形异码): six-plus files independently hand-roll the identical geometry with only the trailing-action contents differing. It was never given an owner, so any future spacing tweak needs six-plus synchronized edits.
- **`components/config-schema-form/index.vue:141`** — Label-over-control form field with optional muted description line, built via Vue h() render calls instead of markup
  - → **FieldStack (settings/field-stack.vue)**
  - reach: Rendered by ConfigSchemaForm, which is consumed from pages/bots/components/bot-network.vue (outside this domain's directory list, but the component itself is in-scope: apps/web/src/components/config-schema-form/)
  - `h(Label, { for: isLabelFor }, ...) followed by h('p', { class: 'text-xs text-muted-foreground' }, field.description) followed by the control h(...), all wrapped by the caller's `class="space-y-2"` div (lines 6 and 32)`
  - why: This is the FieldStack shape (Label above control, optional muted help text below, `space-y` rhythm) reimplemented by hand via the h() API instead of composing <FieldStack :label :help>. It predates FieldStack and was not swept when add-provider/create-model/add-platform were migrated. Because it's schema-driven and dynamically renders 6 field type variants (secret/bool/number/enum/textarea/text), it is a good FieldStack candidate: FieldStack already supports a custom #label slot (used elsewhere for the '(optional)' suffix pattern this file also hand-rolls at line 145) and a `help` prop for the description paragraph, both of which this file reimplements manually.

### supermarket (4)

- **`pages/supermarket/plugin-detail.vue:96`** — 'MCPs' list: divide-y divide-border/60 wrapper with per-item flex items-center gap-3 py-4 rows, each with a leading size-10 icon box, title, description
  - → **SettingsSection + SettingsRow (leading icon slot, label=display name, description=mcp description/url/command)**
  - reach: supermarket/plugin-detail page, 'MCPs' section (v-if="plugin.mcps?.length"), shown whenever a plugin manifest includes mcps
  - `mt-4 divide-y divide-border/60 ... flex min-w-0 items-center gap-3 py-4`
  - why: Hand-rolled full-bleed divide-y list with per-item py-4 + leading icon box + title/description is the SettingsRow shape (leading media slot + label + description), just using a full-bleed divider and no card wrapper. Two more instances of the identical pattern exist in the same file (skills list) and in skill-detail.vue (files list) — same shape duplicated 3x across 2 files, differing only in the leading icon.
- **`pages/supermarket/plugin-detail.vue:137`** — 'Skills' list: divide-y divide-border/60 wrapper with per-item flex items-center gap-3 py-4 rows, leading size-10 icon box, title, description
  - → **SettingsSection + SettingsRow**
  - reach: supermarket/plugin-detail page, 'Skills' section (v-if="pluginSkills.length")
  - `mt-4 divide-y divide-border/60 ... flex min-w-0 items-center gap-3 py-4`
  - why: Identical shape to the MCPs list above in the same file, and to the files list in skill-detail.vue — three copies of the same row shape hand-rolled with only the icon differing, confirming this is a recurring, driftable pattern.
- **`pages/supermarket/skill-detail.vue:85`** — 'Files' list: divide-y divide-border/60 wrapper with per-item flex items-center gap-3 py-4 rows, leading size-10 icon box (FileText) + filename
  - → **SettingsSection + SettingsRow**
  - reach: supermarket/skill-detail page, 'Files' section (v-if="skill.files?.length")
  - `mt-4 divide-y divide-border/60 ... flex items-center gap-3 py-4`
  - why: Same divide-y/py-4/leading-icon-box shape as plugin-detail.vue's two lists — a third hand-written copy of the identical row geometry across the two detail pages.
- **`pages/supermarket/components/install-plugin-dialog.vue:82`** — Config-variable form field: space-y-1.5 wrapper containing a hand-written <label class="text-xs font-medium"> + <Input class="h-8 text-xs">, looped per plugin.variables entry
  - → **FieldStack (label prop + #label slot for the required-asterisk if needed, wrapping the Input)**
  - reach: InstallPluginDialog, 'variables.length' branch — shown whenever the selected plugin declares config variables (API keys/secrets), the common case for real plugins
  - `space-y-1.5 ... text-xs font-medium ... h-8 text-xs`
  - why: Textbook Label-over-control vertical field (space-y-1.5, label, then control) — the exact shape FieldStack owns. The same file already imports and uses FieldStack four lines above (for BotSelect), so this is an internal inconsistency: one field in the dialog is migrated, the config-variable fields right below it are not.

### onboarding (11)

- **`pages/onboarding/steps/Step2Appearance.vue:39`** — Label('mb-2 block text-sm font-medium') directly above a Select control, no wrapping div
  - → **FieldStack (label prop, wrap Select as default slot)**
  - reach: Step2 'Appearance' step body, language field
  - `mb-2 block text-sm font-medium`
  - why: Exact label-above-control relationship FieldStack owns (space-y-1.5 label→control rhythm). Written with mb-2 block margin instead of composing FieldStack — same shape, different code. Repeated identically at lines 67 and 98 in the same file (theme, color scheme), and 6 more times across Step3Provider.vue and Step4Bot.vue with the byte-identical class string, which is the clearest same-shape-different-code signature in the domain.
- **`pages/onboarding/steps/Step2Appearance.vue:67`** — Label('mb-2 block text-sm font-medium') above the light/dark theme Button pair
  - → **FieldStack**
  - reach: Step2 body, theme field
  - `mb-2 block text-sm font-medium`
  - why: Same as line 39 — duplicate of the same hand-rolled field header pattern within one file.
- **`pages/onboarding/steps/Step2Appearance.vue:98`** — Label('mb-2 block text-sm font-medium') above the ColorSchemeCard grid
  - → **FieldStack**
  - reach: Step2 body, color scheme field
  - `mb-2 block text-sm font-medium`
  - why: Same pattern a third time in this file; the grid itself (ColorSchemeCard tiles) is a one-off compound and correctly stays local — only the Label-over-control header is the MISS.
- **`pages/onboarding/steps/Step3Provider.vue:492`** — Label('mb-2 block text-sm font-medium') above provider-name Input
  - → **FieldStack**
  - reach: Step3 'form' mode (custom/preset provider form)
  - `mb-2 block text-sm font-medium`
  - why: Same label-over-control shape as Step2; this file repeats it 6 times (name, client type, api key, base url, ACP setup-mode label, ACP managed-field label in the v-for), all with the identical literal class string.
- **`pages/onboarding/steps/Step3Provider.vue:505`** — Label('mb-2 block text-sm font-medium') above client-type Select
  - → **FieldStack**
  - reach: Step3 'form' mode, custom-provider-only client type field
  - `mb-2 block text-sm font-medium`
  - why: Duplicate of the same header pattern.
- **`pages/onboarding/steps/Step3Provider.vue:530`** — Label('mb-2 block text-sm font-medium') above API key Input (type=password)
  - → **FieldStack**
  - reach: Step3 'form' mode
  - `mb-2 block text-sm font-medium`
  - why: Duplicate of the same header pattern.
- **`pages/onboarding/steps/Step3Provider.vue:547`** — Label('mb-2 block text-sm font-medium') above base-url Input
  - → **FieldStack**
  - reach: Step3 'form' mode
  - `mb-2 block text-sm font-medium`
  - why: Duplicate of the same header pattern.
- **`pages/onboarding/steps/Step3Provider.vue:749`** — Label('mb-2 block text-sm font-medium') above the ACP setup-mode button group
  - → **FieldStack (label prop; the 3-button toggle group itself is a one-off compound and stays local inside the default slot)**
  - reach: Step3 'acp' mode
  - `mb-2 block text-sm font-medium`
  - why: Same header shape; only the label-to-control wrapper is duplicated code, the button group content is legitimately custom.
- **`pages/onboarding/steps/Step3Provider.vue:773`** — Label('mb-2 block text-sm font-medium') above each ACP managed-field control, inside a v-for
  - → **FieldStack**
  - reach: Step3 'acp' mode, managed-fields loop
  - `mb-2 block text-sm font-medium`
  - why: Same header pattern repeated per-field in the v-for — this is the highest-multiplicity instance of the miss (one per dynamic ACP field).
- **`pages/onboarding/steps/Step4Bot.vue:449`** — Label (no class override, just default) above display-name Input, plus an inline required-asterisk span
  - → **FieldStack (label prop with a #label slot override for the required-asterisk span, per the skill's documented FieldStack #label use-case)**
  - reach: Step4 bot-creation form, avatar+name row
  - `class="mb-2" (Label default, no font-medium override)`
  - why: Same label-above-control relationship; the asterisk-for-required decoration is exactly the '#label slot lets a caller pair the label text with inline meta' case FieldStack's own doc comment calls out.
- **`pages/onboarding/steps/Step4Bot.vue:525`** — Label + CircleHelp Tooltip row ('mb-2 flex items-center gap-2') above the workspace-backend Select
  - → **FieldStack (#label slot for the label+tooltip row, matching the documented 'label with inline meta' use case; default slot for the Select)**
  - reach: Step4 bot-creation form, workspace backend field (only when allowLocalWorkspaceCreate)
  - `mb-2 flex items-center gap-2`
  - why: Same label-above-control relationship as line 491's chat-model field (identical markup shape, not separately listed to avoid double-counting the same pattern), just with a Tooltip appended to the label row — FieldStack's #label slot exists precisely for this.

### profile-account (11)

- **`pages/usage/index.vue:102`** — 4-cell stat-tile grid (caption label + tabular value), hand-rolled instead of MetricReadout
  - → **MetricReadout (framed, caller owns the grid)**
  - reach: Usage page root, 'Overview' section, always visible once a bot + data are selected
  - `grid grid-cols-2 gap-px overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-border sm:grid-cols-4 / cell: bg-card px-4 py-3.5`
  - why: This is exactly MetricReadout's canonical shape: 'caption label + tabular value' tile, caller-owned grid. The four cells (lines 103-134) each hand-repeat `p class="text-xs text-muted-foreground"` + `p class="mt-1 text-xl font-semibold tabular-nums"` instead of composing `<MetricReadout :label :value framed />`. Geometry differs slightly (px-4 py-3.5 vs MetricReadout's p-3, gap-px divider-grid vs discrete tiles) purely because it was re-derived independently, not because the relationship differs — same job (a metric tile in a grid) as bot-overview's usage tiles, which the owner's own doc comment cites as its reference case.
- **`pages/usage/index.vue:86`** — Bare centered muted-text empty state replacing the entire page body
  - → **Empty + EmptyHeader/EmptyTitle (as used in bot-email.vue:61-67)**
  - reach: Usage page root, shown when no bot is selected yet
  - `px-2 py-12 text-center text-sm text-muted-foreground`
  - why: This is not a small in-list 'no results' aside (the skill's stay-local exception) — it fully replaces the page's primary content area, the same job Empty performs at bot-email.vue's 'no bindings' state (which uses Empty/EmptyTitle/EmptyDescription with a py-12 override). A bare <p> here is a hand-rolled duplicate of that already-solved shape.
- **`pages/usage/index.vue:198`** — Bare centered muted-text empty state for 'no usage data in range'
  - → **Empty + EmptyHeader/EmptyTitle**
  - reach: Usage page root, shown when a bot+filters are selected but return no chart data
  - `px-2 py-12 text-center text-sm text-muted-foreground`
  - why: Same reasoning as line 86 — this is the page's entire secondary-content substitute (nothing else renders in its place), matching Empty's job, not a trivial aside line.
- **`pages/people/index.vue:173`** — Label-over-control form field (username)
  - → **FieldStack (wrapped in FormStack)**
  - reach: People page → 'New member' Dialog body
  - `grid gap-2`
  - why: Textbook FieldStack shape: a <Label :for> above a control. The dialog hand-rolls this 8 times (see sibling misses at lines 183, 192, 204, 213, 224, 287, 296) instead of composing FieldStack/FormStack, which is the documented house form pattern (New Task dialog) this skill exists to unify. No validation exists here, but FieldStack's plain (non-vee-validate) mode is exactly for this case.
- **`pages/people/index.vue:183`** — Label-over-control form field (display name)
  - → **FieldStack**
  - reach: People page → 'New member' Dialog body
  - `grid gap-2`
  - why: Same as line 173; part of the same repeated hand-rolled field run.
- **`pages/people/index.vue:192`** — Label-over-control form field (email)
  - → **FieldStack**
  - reach: People page → 'New member' Dialog body
  - `grid gap-2`
  - why: Same as line 173.
- **`pages/people/index.vue:204`** — Label-over-control form field (password)
  - → **FieldStack**
  - reach: People page → 'New member' Dialog body
  - `grid gap-2`
  - why: Same as line 173.
- **`pages/people/index.vue:213`** — Label-over-control form field (confirm password)
  - → **FieldStack**
  - reach: People page → 'New member' Dialog body
  - `grid gap-2`
  - why: Same as line 173.
- **`pages/people/index.vue:224`** — Label-over-control form field (role select)
  - → **FieldStack**
  - reach: People page → 'New member' Dialog body
  - `grid gap-2 w-fit`
  - why: Same shape as line 173 (Label + Select); the extra w-fit is an unrelated sizing concern layered on top, not a different relationship.
- **`pages/people/index.vue:287`** — Label-over-control form field (new password, reset dialog)
  - → **FieldStack**
  - reach: People page → 'Reset password' Dialog body
  - `grid gap-2`
  - why: Same shape as line 173, in the sibling reset-password dialog.
- **`pages/people/index.vue:296`** — Label-over-control form field (confirm password, reset dialog)
  - → **FieldStack**
  - reach: People page → 'Reset password' Dialog body
  - `grid gap-2`
  - why: Same shape as line 173, in the sibling reset-password dialog.

### provider-domains (5)

- **`pages/email/components/add-email-provider.vue:33`** — Vertical Label-over-control form field inside a dialog body, built from the superseded @memohai/ui FormItem (grid gap-2) + bare <Label :for> instead of FieldStack
  - → **FieldStack (wrap the run in FormStack)**
  - reach: email 'Add provider' dialog (FormDialogShell #body), reached via the email list page's Add button / empty-state Add button
  - `<FormItem> ... <Label :for="'email-provider-name'"> ... <FormControl><Input .../></FormControl> </FormItem>`
  - why: The sibling add-memory-provider.vue (apps/web/src/pages/memory/components/add-memory-provider.vue:24-51) already migrated this exact two-field dialog form to FormStack + FieldStack; add-email-provider.vue is the same shape (name text field + provider-type select) still hand-rolled with FormItem/Label. Two fields, same dialog body pattern, same domain family — genuine same-shape-different-code drift.
- **`pages/email/components/add-email-provider.vue:51`** — Second Label-over-control field (provider type Select) in the same dialog, same FormItem/Label pattern
  - → **FieldStack**
  - reach: email 'Add provider' dialog body
  - `<FormItem> ... <Label :for=" 'email-provider-type'"> ... <FormControl><Select v-bind="componentField">...</Select></FormControl> </FormItem>`
  - why: Same as the field above; both fields in this dialog should be FieldStack children of one FormStack, matching add-memory-provider.vue's structure exactly.
- **`pages/email/components/provider-setting.vue:49`** — A vee-validate field wrapped in the superseded FormItem (grid gap-2) sitting inside a SettingsRow's default (control) slot, instead of a bare FormControl per the skill's validated-field recipe
  - → **Drop FormItem; put <FormControl><Input/></FormControl> (and <FormMessage/> if needed) directly in SettingsRow's default slot**
  - reach: email provider detail pane (DetailPane opened from BackendCard click on the email list), 'Configuration' SettingsSection, first row (Name)
  - `<SettingsRow :label="$t('common.name')"><FormItem class="w-80"><FormControl><Input .../></FormControl><FormMessage /></FormItem></SettingsRow>`
  - why: This file is otherwise already migrated (SettingsSection/SettingsRow/LoadingButton throughout) — it is the one leftover FormItem carried over from the pre-owner form pattern. The skill explicitly names this exact anti-pattern: 'The older @memohai/ui FormItem (grid gap-2) is superseded.' Every other field in the same SettingsSection (the dynamic schema fields at line 63+) already uses a bare control directly inside SettingsRow with no FormItem wrapper, so this one field visibly drifts from its own siblings.
- **`pages/memory/components/provider-setting.vue:38`** — Label-over-control field (name) built from raw div + bare Label, not FieldStack
  - → **FieldStack**
  - reach: memory external-backend detail pane (DetailPane opened from BackendCard click on the 'Advanced' external-provider list), inside SettingsShell
  - `<div class="space-y-2"><Label>{{ $t('memory.name') }}</Label><Input v-model="form.name" .../></div>`
  - why: Domain note says memory provider-setting migrated in group-1 for the speech/transcription/video family, but this file (memory's own external-provider detail, distinct from the already-migrated speech/transcription/video provider-setting.vue files) was not brought along — it never adopted SettingsSection/SettingsRow/FieldStack at all, unlike its speech/transcription/video siblings which use SettingsSection+SettingsRow+FieldStack throughout. Confirm-and-skip did not hold for this specific file.
- **`pages/memory/components/provider-setting.vue:56`** — Dynamic schema field loop: label + optional required-marker + description + control, exactly the FieldStack (label/help/control) shape, hand-rolled with space-y-2
  - → **FieldStack (label + help prop covers the description; keep the required-asterisk in the #label slot)**
  - reach: same memory external-backend detail pane, dynamic provider config schema grid below the name field
  - `<div v-for="(fieldSchema, fieldKey) in providerSchema.fields" class="space-y-2" :class="isWideField(...) ? 'md:col-span-2' : ''"><Label>{{ fieldSchema.title || fieldKey }}<span v-if="fieldSchema.required" class="text-destructive">*</span></Label><p v-if="fieldSchema.description" class="text-xs text-muted-foreground">{{ fieldSchema.description }}</p><Input .../></div>`
  - why: This is the identical dynamic-schema-field grid pattern already migrated in speech/transcription/video's model-config-editor.vue (FieldStack with :help and md:col-span-2 wide-field handling) — same shape, same code family, just not ported to this one file.

### shell-misc-pages (2)

- **`pages/appearance/index.vue:166`** — A label+description+trailing-control settings row (code-highlight theme pickers), hand-rolled with the exact SettingsRow inset/border/padding geometry (`mx-4 border-b border-border py-3 last:border-b-0`) but written as a raw div instead of composing the owner. Body uses a manual label+description block plus a custom two-column preview area below.
  - → **SettingsRow (with #content for the custom label/description + preview body, or keep label/description props and put the two-column shiki preview in the default/#content slot)**
  - reach: SettingsSection 'settings.appearance.typography', last row after the four font-size/family SettingsRow instances — reached directly on page load, no dialog/v-if gating
  - `mx-4 border-b border-border py-3 last:border-b-0`
  - why: Duplicates SettingsRow's exact root class string verbatim (same border-b/mx-4/py-3/last:border-b-0 rhythm as every sibling row in the same section), so it is same-shape-same-size-same-surface — a textbook 同形异码 case, not a different relationship.
- **`pages/appearance/index.vue:229`** — A label+description+trailing-Select settings row (mermaid theme picker) hand-rolled with SettingsRow's exact geometry, containing an inner `flex min-h-[2.25rem] items-center justify-between gap-4` wrapper for the label/select line plus a mermaid diagram preview appended below.
  - → **SettingsRow (#content slot for the label/description + diagram preview, trailing default slot for the Select)**
  - reach: SettingsSection 'settings.appearance.diagrams', the only row in that section — reached directly on page load, no dialog/v-if gating
  - `mx-4 border-b border-border py-3 last:border-b-0`
  - why: Same border/padding/inset class string as SettingsRow's root and as the adjacent typography section's rows; the only reason it isn't already SettingsRow is the extra preview content below the label+control line, which #content already exists to solve.
  - **Correction (2026-07-06): recommendation tried and REVERTED.** Putting the preview inside `#content` bounds it to the content column (left of the Select) — the centered mermaid diagram drifted left of the card's axis (visual regression caught in review). This row is a THREE-piece shape — (label | Select) line + a full-ROW-width preview below — which SettingsRow's two-piece model cannot express. Re-adjudicated stay-local; reason recorded in the file's head comment. The code-highlight row above (a true two-piece `stack="always"` shape) stays migrated.

## DIFF-RELATIONSHIP notes (NOT debt — do not unify)

- [bots] bots/new.vue (create-bot wizard): every field is `<Label class="mb-2">` + control stacked vertically outside any SettingsSection card, separated by `<Separator>` between wizard steps rather than row dividers inside a card. Geometry differs from FieldStack's `space-y-1.5` rhythm and it's not inside a settings surface at all — a full-page onboarding wizard is a different relationship, not a settings form. Not flagged as a MISS, but worth a future owner discussion if a second full-page wizard appears.
- [bots] channel-settings-panel.vue L154-168 hand-rolls the exact same 'Advanced label + chevron-button' row shape as bot-heartbeat/bot-network/bot-hooks (mx-4 flex min-h-[3.75rem] ... border-b ... last:border-transparent) — the census explicitly ruled this file stays-local ('domain ChannelField + manual-toggle-of-sibling'), so it was not re-flagged here, but note it is byte-near-identical to the bot-heartbeat/bot-hooks MISSes reported below. If those get migrated to SettingsRow, this file should be revisited for consistency even though the census closed it.
- [bots] bot-email.vue's per-binding block (avatar+name+unbind button, then a second row of two permission Switches below) is a two-part compound row denser than plain SettingsRow — correctly stays local (census batch-1 migrated file); flagging only so a future 'row with an attached sub-row of toggles' pattern isn't missed if it recurs a third time.
- [home] session-info-panel.vue (popover key-value readout, lines 16-59): divide-y rows with py-2 pairing a muted label + tabular value — same *idea* as MetricReadout but living inside a hover popover on the chat top bar, not a settings card. Different surface + no card frame; correctly local, not a MISS.
- [home] heartbeat-trigger-block.vue (lines 16-32) and schedule-trigger-block.vue (lines 16-39): grid-cols-[auto_1fr] label/value pairs inside a colored chat-message event card (bg-event-heartbeat-soft / bg-event-schedule-soft). Same visual idea as MetricReadout's caption+value cell, but it's chat content, not a settings page — a future 'inline key-value list' primitive could unify these two with each other (they're identical in shape), but they are not settings-owner debt.
- [home] tool-call-detail-*.vue family (18 files): every one renders a `space-y-1.5` stack of label:value lines inside an expandable tool-call detail body. All structurally identical to each other (potential internal dedup opportunity — a shared 'KeyValueList' component for this family specifically) but this is diagnostic tool-output data, the skill's explicit 'one-off compound' exception, not settings-row debt.
- [home] browser-pane.vue address bar (h-11 flex items-center gap-2 px-2 border-b) and terminal-pane.vue disconnected banner: full-bleed toolbar/status bands structurally splitting a workspace pane — correctly full-bleed per the divider rule, not an inset SettingsRow.
- [home] display-pane.vue prepare-progress card (lines 34-74, rounded-lg border bg-card p-5): a live-status compound block (matches skill §13 enumerate-in-between-states guidance) already noted in project memory as a pending video/tab-seam item — not spacing debt, a different concern altogether.
- [home] dockview/* tab-strip and header-action files (group-actions, header-add-actions, terminal-tab, workspace-tab, prefix-header-actions): full-bleed chrome bands and custom SVG tab shapes for the editor-like dock — a declared 'different surface' (sidebar/file-tree/tab-strip family), correctly hand-written; menus inside correctly use DropdownMenu/DropdownMenuItem, not raw buttons.
- [web-search] The identity-card header in provider-setting.vue (lines 6-45) and fetch-provider-setting.vue (lines 6-45) — logo + name + delete/enable controls in a bordered bg-card box — is a genuinely one-off compound header, not a SettingsRow or BackendCard shape (it's non-clickable, has two trailing controls including a nested ConfirmPopover+Switch, which the skill's 'don't nest a button in a button' trap explicitly says must stay local when a row needs both a whole-row click and interactive trailing controls — except here the whole box isn't even clickable, so it's just a correct one-off compound; kept local correctly.
- [web-search] The per-provider config blocks (bing/bocha/brave/cloudflare-markdown/duckduckgo/exa/google/jina/jina-reader/searxng/serper/sogou/tavily/yandex-settings.vue — 14 files) are all already fully composed from SettingsRow with no hand-rolled geometry; these are the clean reference shape for this domain and account for the bulk of alreadyMigratedCount alongside index.vue's use of BackendCard.
- [web-search] index.vue's two 'add' section headers (search providers / fetch providers) are plain flex rows with a title+hint and a trailing Button — not a SettingsRow (no card, no border-b) and not a SettingsSection title (no card wrapper below it holds rows in the settings sense, it holds a BackendCard grid) — this is page-chrome, not an owner shape; correctly hand-written.
- [web-search] The 'unsupportedProvider' / 'nativeManaged' one-line px-4 py-3 text-xs text-muted-foreground fallback divs (provider-setting.vue:108, fetch-provider-setting.vue:50,87) are trivial muted-text one-liners the skill explicitly exempts ('A trivial muted <p> no-results line').
- [providers-models] pages/providers/components/model-item.vue:6 — a flush list row (min-h-[3.25rem] py-2.5 mx-4 border-b) inside ModelList's SettingsSection card. This is explicitly commented as deliberately denser than SettingsRow ('Same row language as the Configuration card above... A flush list row, not a nested card') and matches the skill's documented dense-list-row exception verbatim. Confirmed DIFF-RELATIONSHIP, not debt — but it is itself a repeated dense-row shape (worth flagging as a FUTURE dense-list-row owner candidate if other domains show the same 3.25rem/py-2.5 pattern).
- [providers-models] pages/providers/components/model-list.vue:52-56 — the pagination footer (`flex items-center justify-between border-t border-border px-4 py-3`) is hand-rolled outside SettingsSection's #footer slot specifically because #footer is justify-end only and this needs justify-between (summary span + pager). This exactly matches the skill's documented '#footer is justify-end' trap/exception. Confirmed LOCAL.
- [providers-models] components/add-provider/index.vue:151 — see coverageNote; noted here as a possible future SettingsRow-in-isolation candidate but not asserted as a current miss given single occurrence.
- [providers-models] pages/providers/model-setting.vue and its siblings (speech/video/email/web-search/bots) are the clearest future-owner candidate in this entire sweep — flagged fully under misses above, repeating the pointer here since it is the dominant cross-domain finding.
- [supermarket] plugin-card.vue and skill-card.vue (grid discovery cards: size-9 leading icon + title/external-link + description + trailing Install button, root is a clickable Card, not a <button>) resemble BackendCard's leading-icon+title+trailing shape but are a genuinely different relationship: they live in a responsive card grid (not a settings-list card), carry a richer trailing zone (an Install Button, not just a chevron), and skill-card.vue's whole-card click coexists with a nested interactive Button + external link — the same 'stretched-overlay, nested-interactive-control' pattern the owner skill flags as a reason to stay hand-written. Not a MISS; noting as a possible future BackendCard variant (clickable-card-with-CTA) if this shape recurs elsewhere.
- [supermarket] install-plugin-dialog.vue's plugin-summary box (line 20, rounded-md border border-border p-3 space-y-1) and its nested per-skill mini-rows (line 54, flex items-start gap-2 rounded border border-border/60 bg-muted/20 px-2 py-1.5) form a dense preview-card-within-a-dialog with text-caption/text-[10px] sizing — the 'tighter inline-form language' the skill calls out as a different relationship from settings-page rows, not a SettingsRow/BackendCard duplicate. Correctly local.
- [supermarket] install-skill-dialog.vue's skill-summary box (line 20, same rounded-md border border-border p-3 space-y-1 shape) is the same dense dialog-preview pattern as above — local, not a MISS. This file is otherwise already migrated for its one field (FieldStack for BotSelect).
- [supermarket] index.vue's loading/empty states (py-8 flex/text-center blocks) are trivial muted-text placeholders with no owner shape to compose — correctly local; PageShell and the plugin/skill grids are already the right composition.
  - **2026-07-05 correction:** this verdict predates InlineLoadingRow (added 2026-07-04). The loading half of this pair (the empty half still stands, correctly local) is now a MISS, and has been migrated to `<InlineLoadingRow class="justify-center py-8">` in both the plugins and skills tabs.
- [onboarding] The entire wizard shell (index.vue + all 5 steps) is a first-run full-screen stage, not a settings surface: no PageShell, no SettingsSection card, custom h-[35rem] step panels with hand-choreographed visible/exiting/delay-[Nms] transition classes on every element, and custom prev/next footer buttons (h-[2.625rem] rounded-lg raw <button>, not the Button atom, not a SettingsSection #footer). This is correctly local per the skill's 'a genuinely one-off compound block' and 'different surface' tells — do not force PageShell/SettingsSection/#footer onto it; the whole point of this surface is a distinct, centered, animated wizard chrome.
- [onboarding] Step3Provider's provider/ACP picker buttons (h-16 rounded-lg border ... grid-cols-3, lines 397-435) are a vertical-ish entity-picker tile grid, but they are NOT PersonaTile: PersonaTile is 'w-52 flex-col items-center' (vertical, centered, fixed width); these are full-width grid cells with a horizontal icon+label row (h-16, flex items-center gap-2.5). Geometry differs enough (horizontal internal layout, grid-controlled width) that this reads as a candidate for a FUTURE 'compact picker tile' owner rather than an existing-owner MISS — noting it, not flagging it.
- [onboarding] Step3Provider's error banner (mt-5 rounded-lg border border-destructive/30 bg-destructive/5 p-4, lines 566-616) looks like it could map to CalloutBanner (tone=destructive), but it has a materially richer internal structure than the owner's contract: a title+description+optional error-detail line+two conditional action buttons (retry / manual-add) inside the notice body. CalloutBanner's slots are #icon + one trailing default action, not a multi-button action row — forcing this in would either drop a button or abuse the slot. Left local; a good candidate to raise as a CalloutBanner variant, not silently migrated.
- [onboarding] Step5Complete's 3 feature-teaser cards (rounded-xl border bg-muted/30 px-5 py-6, icon+title+description) and its no-provider warning banner (border-warning-border bg-warning-soft rounded-lg) are plain content tiles/notices on a marketing-style completion screen, not settings rows or metrics — MetricReadout is for a stat/number cell the caller grids, CalloutBanner's tone enum doesn't include a neutral informational card. These are correctly local one-off compounds for this single screen.
- [onboarding] Step1Intro.vue is almost entirely a canvas-less particle/bubble animation (rAF loop, degraded-fps handling) plus one CTA button — there is no settings-shaped surface here at all; nothing to migrate.
- [onboarding] The repeated raw <button> footer controls (prev/skip/next, h-[2.625rem] ...) across all 4 form-bearing steps are themselves a same-shape-different-code candidate, but they are wizard-chrome buttons, not one of the ten current owners (SettingsSection's #footer is justify-end Save/Cancel inside a card, which this wizard doesn't have) — flagged here only as a note, not as a MISS against an existing owner, since forcing #footer onto a card-less full-screen step would be the 'blind-unify' anti-pattern the skill warns against.
- [profile-account] usage/index.vue:5-81 — the 4-field filter bar (bot/date-range/session-type/model) uses `<p class="text-xs text-muted-foreground">` captions + `space-y-1.5` over each control. Judged DIFF-RELATIONSHIP, not a FieldStack miss: it's a dense filter toolbar (no <Label for>, no focus-wiring, no validation, sits in a responsive grid inside a titled SettingsSection) rather than a form field — every confirmed FieldStack usage found elsewhere in the codebase is a dialog/form field with real Label+validation semantics, not a filter row. Could be a future 'filter-field' primitive candidate if this shape recurs.
- [profile-account] usage/index.vue:98-101 and 205-216 — both 'Overview' and 'Records' section headers hand-reconstruct SettingsSection's title-bar markup (`section class="space-y-2.5"` + `div class="flex min-h-7 items-center justify-between ... px-2"` + `h2 class="text-[13px] font-medium text-muted-foreground"`) instead of using `<SettingsSection>` as the wrapper. NOT flagged as a miss: this is a solved, intentional twin already present at bot-email.vue:133-136, which deliberately keeps a `<Table>` outside SettingsSection's bordered card (Table already draws its own frame/border; wrapping it in SettingsSection would be card-in-card). Same relationship, correctly kept local both places.
- [profile-account] people/index.vue — the entire page's central content is a real data `<Table>` (member list with avatar/role/status/actions columns) plus two Dialogs. The Table itself is the explicitly-named 'single real data table' stay-local exception in the owner skill; only its Dialog-body fields (see misses) fall inside owner scope.
- [profile-account] people/index.vue:239-250 — the 'active on create' Label+description+Switch row (`flex items-center justify-between gap-4 border-t pt-4`) sits inside the create-member Dialog body, styled as a mini settings-row-like block with a top border. Not flagged: it is a dialog-form's own field grouping (Label + helper <p> + Switch on one line), a shape SettingsRow doesn't claim (SettingsRow lives in a settings-page card, not a dialog form body) and FieldStack doesn't fit either (label sits beside, not above). Worth watching as a second future-primitive candidate if this exact combination (toggle + description, inline, with a top hairline) recurs in other dialogs.
- [profile-account] profile/components/connected-accounts-section.vue:22-27 (loading spinner placeholder borrowing SettingsRow's mx-4/min-h-[3.75rem]/py-3) and :104-164 (active link-code block with live countdown + copy) are both correctly local per the skill's own named exceptions ('a centered placeholder borrowing a row's min-height' and 'a link-code countdown' respectively) — confirmed correct, not misses.
- [profile-account] about/index.vue:13-67 — the identity card (logo + name + tagline + description + resource-link buttons) is the skill's own named exception ('About is the one exception'); its SettingsRow-based Updates section (lines 72-114) is correctly owner-based. This file carries pre-existing token/color debt (font-[NNN], text-[Npx], text-foreground/NN) called out by memoh-web SKILL.md itself, but that is color/type debt, not spacing-owner debt, and out of this sweep's scope.
- [shell-misc-pages] keyboard-shortcuts/index.vue renders its own `<section class="mx-auto max-w-3xl px-6 pt-10 pb-12">` header block (title + reset-all button + intro paragraph) instead of composing PageShell — this is a page-shell-layer question (not a row/field/section owner-shape), so not flagged as a spacing-owner MISS, but worth a note for whoever next touches page-shell adoption.
- [shell-misc-pages] voice/index.vue similarly hand-rolls its own header/section chrome (`mx-auto max-w-3xl px-6 pt-10 pb-12 space-y-8`, per-capability `space-y-2.5` sub-sections with a title+hint+Add-button row) rather than SettingsSection/PageShell — but this is the list-view half of a list/detail swap pattern (DetailPane + SwapTransition) with two independent add-flows per capability; the header rows already correctly compose BackendCard for each provider entry. Flagging the outer chrome would be re-litigating page-shell, not a settings-row/field miss.
- [shell-misc-pages] login/index.vue, login/components/dot-matrix-bg.vue, and oauth/mcp-callback.vue are correctly hand-written per the skill's own 'genuinely one-off compound block' and 'different surface' tells (auth screen with canvas background animation, OAuth popup-callback status screen) — noted here only so a future pass doesn't mistake their absence from the MISS list for missed coverage.
- [sidebar-shell] sidebar/index.vue nav tabs (h-8, rounded-full, icon-anchored pill) and the pinned Settings row (h-9, gap-[9px], px-[11px]) — a distinct rail-button system, not SettingsRow; explicitly documented in-file as anchored-icon geometry unique to this rail.
- [sidebar-shell] sidebar/session-item.vue rows (min-h-[2.125rem], rounded-[9px]) and sidebar/recents.vue virtualized list (estimateSize 36, pb-[2px] seam) — dense sidebar list rows, a different surface per the skill's 'sidebar rows live in non-settings surfaces with their own row system' rule; not a MISS, and not even in the '3.25rem dense list' family since they're shorter/pill-shaped by design (candidate only if a future 'nav-rail-row' owner is ever built, not the dense-list-row primitive).
- [sidebar-shell] sidebar/panel-schedule.vue group header row (h-8, px-2, mt-2) and files-pane.vue section header (min-h-[1.6875rem], pl-[11px]) — sidebar section labels with inline icon-button toolbars, structurally unlike SettingsSection's title-above-card pattern (no card, no border, lives directly in the scroll list).
- [sidebar-shell] files-pane.vue batch-ops bar (border-b border-border px-2 h-7) — this is exactly the kind of 'chrome band, full-bleed, h-7' the domain note calls out as NOT a settings row; correctly stays local.
- [sidebar-shell] settings-sidebar/index.vue SidebarGroupLabel / NavItem rows — primary settings navigation rail, a different surface from the settings-detail-pane rows that SettingsRow owns.
- [sidebar-shell] master-detail-sidebar-layout/index.vue's nested-card box (border border-border/60 bg-muted/10 rounded-lg) is a sidebar chrome shell, not a SettingsSection card — it wraps a nav list + slots, never a run of label/control rows.
- [sidebar-shell] All five files-pane.vue dialogs (new file, mkdir, rename, delete, batch-delete) and panel-schedule.vue's delete dialog are plain single-Input or single-paragraph confirm dialogs — correctly using bare Dialog+Input / Dialog+<p> with DialogFooter, too trivial to warrant FieldStack (single unlabeled input, not a labeled form field).
- [file-manager] file-tree-node.vue:117 — tree row `min-h-[1.6875rem] cursor-pointer items-center mx-1 mb-px pl-1 pr-1 rounded-sm text-[0.84375rem]` with depth-indent guide spans (line 124-127) and sidebar-accent/hover coloring. This is the skill's explicitly-named exception ("file-tree rows live in non-settings surfaces with their own row systems") — much denser than SettingsRow's 3.75rem geometry, uses sidebar tokens not card tokens, and carries selection/context-menu/drag semantics a settings row doesn't have. Correctly local; not a candidate for SettingsRow or BackendCard.
- [file-manager] file-tree-node.vue:198-210 — loading sub-row for an expanding folder, same tree-row geometry (min-h-[1.6875rem]) plus a LoaderCircle spinner. This is the 'centered placeholder borrowing a row's min-height' exception verbatim — stays local, not Empty.
- [file-manager] file-tree.vue:34-47 — root-level loading (`flex items-center justify-center py-10`) and empty (`px-3 py-6 text-center text-xs text-muted-foreground`) states for the whole tree panel. Shape-wise these resemble the Empty atom (icon/spinner + muted caption, centered), but they're compact panel-embedded states (py-10/py-6) rather than a full page/dialog-level empty state, and carry no icon — closer to the skill's 'trivial muted <p> no-results line' exception. Borderline; noting as a DIFF-RELATIONSHIP rather than a MISS since forcing the Empty atom into a ~40px-tall tree panel would change visual weight, not just spacing code.
  - **2026-07-05 correction:** this verdict predates InlineLoadingRow (added 2026-07-04), which is exactly the left-aligned in-flow shape this DIFF-RELATIONSHIP was reaching for. The loading state (line 34-40) is now a MISS and has been migrated to `<InlineLoadingRow class="justify-center py-10">`; the empty state (line 42-47, no owner fits a bare muted caption) still stands as correctly local.
- [file-manager] file-viewer.vue:753-813 — external-change 'chip' banner (icon + status message + up to 3 ghost buttons, h-6/text-xs mini-buttons) is a one-off compound conflict-resolution surface (VS Code-style), matching the skill's 'genuinely one-off compound block' exception; no CalloutBanner/SettingsRow shape fits a conflict chip with dynamic multi-button sets.
- [file-manager] file-viewer.vue:817-878 — compare-mode toolbar (diff header band with refresh/reload/save/close ghost buttons) is one-off editor chrome for the Monaco diff view, not a settings row or footer band.
- [file-manager] file-viewer.vue:909-946 — three terminal editor-pane states (image-deleted, unsupported-preview with Download action) use `flex h-full flex-col items-center justify-center gap-3` + icon + text + optional Button. These are Empty-atom-shaped (icon, muted text, optional CTA) but rendered inside a full-height Monaco-editor host pane rather than a settings/list surface — plausibly a future Empty-atom composition candidate, but per skill scope (Empty is generally used on settings/list pages) and given no other file-manager surface uses it, treating as local/one-off rather than a hard MISS.
- [settings-owners] expandable-row.vue re-derives SettingsRow's exact header geometry (min-h-[3.75rem] py-3, mx-4 border-b wrapper) instead of composing <SettingsRow>, but this is explicitly documented in the component's own leading comment: SettingsRow's root is not interactive, so nesting its <button> header inside SettingsRow would put a button inside a non-interactive row's markup in an invalid/awkward way; the file states it 'reuses the skeleton to keep the rhythm without that trap.' This is a deliberate, self-justified exception rather than an undocumented miss — flagging it as debt would contradict the skill's own guidance to judge by geometry+context and not force a composition where the relationship (interactive header vs. static row) genuinely differs.
- [settings-owners] backend-card.vue, section.vue, and metric-readout.vue all share the identical card-frame utility (rounded-[var(--radius-menu-shell)] border border-border bg-card) but this is the shared *atom-level* frame token, not a spacing-relationship duplication — each is a distinct owner shape (horizontal clickable object row / section card / single metric tile) with different internal geometry (p-3.5+gap-3 row vs. overflow-hidden shell holding child rows vs. min-h-[4.375rem]+p-3 tile). None hand-rolls another owner's row/section shape.
- [settings-owners] detail-pane.vue composes a real <Button variant="ghost"> for its back action (not a hand-rolled clickable div) and its back-row + width-matching layout is a genuinely one-off compound (per the skill's 'STAY hand-written' list) — it doesn't duplicate any of the ten owners' shapes.
- [settings-owners] swap-transition.vue is a pure Transition/motion wrapper with no spacing-owner shape in it at all — out of scope for this sweep's MISS criteria.
- [atoms-misc] monaco-editor/diff.vue:110-117 — a full-bleed originalTitle/modifiedTitle chrome band (border-b border-border px-3 py-1.5) splitting the diff container, not an inset settings-row divider; correctly full-bleed per the divider rule, not a MISS.
- [atoms-misc] color-scheme-card/index.vue — a vertical preview-swatch picker button (mini chrome mockup + label footer) superficially tile-shaped but not the PersonaTile relationship (no media/status/name slots, it's a rendered preview canvas) and not a BackendCard (vertical stack, not horizontal object row); correctly local.
- [atoms-misc] searchable-select-popover/index.vue — virtualized combobox option rows (menuItemClass, ~32px) are a menu-surface relationship, not a SettingsRow/BackendCard list relationship; correctly local and already using the shared menu-class helpers from @memohai/ui.
- [dev-wall] SectionSpacing.vue:96-135 ('Appearance complex row' specimen) intentionally hand-rolls a settings-row shell (`mx-4 border-b border-border py-3 last:border-b-0` + label/description + a full-width custom body) and labels it in its own note as 'candidate owner, not promoted to a primitive yet'. That comment is now STALE: `SettingsRow` (apps/web/src/components/settings/row.vue) already ships `stack="always"` + the `#content` slot, which is exactly this shape (a label row over a full-width custom/control body). This is not a MISS to fix here (domain note says don't migrate dev-wall demos), but it is worth recording: (1) the dev-wall's own spacing documentation page is out of sync with the current owner vocabulary and could mislead a future reader into thinking this shape still lacks an owner; (2) the real source this specimen mirrors — apps/web/src/pages/appearance/index.vue's 'Code highlighting' row (and structurally similar rows in bot-compaction.vue, bot-heartbeat.vue, bot-email.vue, channel-settings-panel.vue, bot-user-access.vue, video/provider-setting.vue, speech/.../provider-setting.vue, transcription/provider-setting.vue, all found via grep for the same `mx-4 border-b` signature) is a live, unmigrated instance of exactly this owner-shape — but those files belong to other domain sweeps (appearance/bots/video/speech/transcription), not dev-wall, so they are named here only as a pointer, not claimed as this sweep's finding.
- [dev-wall] SectionInputsForms.vue and SectionOverlays.vue render the raw `@memohai/ui` atoms `Field`/`FieldLabel`/`FieldControl`/`FieldDescription`/`FieldError` and `FormItem`/`FormLabel`/`FormControl`/`FormMessage` directly, which look like they 'should' be FieldStack. This is correct as-is: the dev wall's job is to demonstrate the underlying @memohai/ui atom in isolation (per its own header comment 'Real primitives from @memohai/ui'), not to demonstrate app-page composition patterns — FieldStack is an apps/web/src/components/settings/ page-level composition over these same atoms, not a replacement for showing the atom itself. Not a miss.
- [dev-wall] SectionDataDisplay.vue's Item/ItemGroup/Card specimens are demonstrating @memohai/ui atoms as shipped (Card, Item, ItemGroup) — these are generic list/card atoms, not the settings-page SettingsSection/SettingsRow owners, and the file's own notes make the atom-vs-owner distinction explicit ('the real pattern: each row is...'). Not a miss.

## Coverage notes (honesty log)

- **bots**: Read all 51 .vue files in apps/web/src/pages/bots (and pages/bots/components) end-to-end — no file was skipped or only-skimmed given the domain's moderate size (~20K lines total, largest single file 1733 lines). Cross-referenced every finding against docs/design/spacing/owner-vocabulary-census.md, which already documents a prior Phase-3 migration (21 files) and an explicit stays-local list; files matching that record were verified structurally (owners imported and composed) rather than re-litigated. The four MISSes below are either NOT covered by the census's migrated/stays-local lists (bot-heartbeat, bot-hooks) or are net-new hand-rolled rows sitting inside an otherwise-migrated file (bot-network, bot-overview) that the census's file-level pass did not catch because it graded the file, not every row inside it. One judgment call flagged as DIFF-RELATIONSHIP rather than MISS: bots/new.vue's wizard-style Label+Input stacked fields, since full-page creation wizards are a distinct surface from the settings/card language FieldStack targets, and no other wizard page in the codebase composes FieldStack for comparison.
- **home**: Full deep-read of all 79 .vue files under apps/web/src/pages/home (components/ + components/dockview/ + index.vue), including the large chat-pane.vue (2702 lines, read in two passes) and message-item.vue (855 lines) in full. This domain has no settings/form/dialog pages in the owner-vocabulary sense — it is the chat/workspace surface (composer, message rendering, tool-call detail panels, dockview tabs/panels for file/terminal/browser/display, popovers for model-picker and session-info). Zero hand-rolled SettingsRow/SettingsSection/FieldStack/BackendCard/PersonaTile/CalloutBanner look-alikes were found; the only look-alike shapes (key-value readouts) live in chat message cards and tool-call detail bodies, which are explicitly different surfaces/relationships per the skill and are cataloged in diffRelationshipNotes rather than reported as misses. The 5 files using Dialog/Popover that the earlier grep-based audit covered were re-verified clean. No area was skimmed or skipped.
- **web-search**: All 20 .vue files under apps/web/src/pages/web-search (index.vue + 19 in components/) were read in full, not skimmed — the domain is small (~2,276 lines total) so no triage was needed. Two files (add-web-provider.vue) I could not confirm is actually routed from index.vue (index.vue only imports AddSearchProvider/AddFetchProvider separately, not AddWebProvider) — flagged its MISS anyway since the component exists in the directory and duplicates the same shape, but its live reachability from the page is unverified from this directory alone (may be used elsewhere, e.g. a combined-add entry point not in this domain).
- **providers-models**: Domain is small (13 .vue files across pages/providers + 8 sibling component dirs) so every file was read in full, not skimmed. Two lower-confidence items I deliberately did NOT report as firm misses, noted here for completeness: (1) components/add-provider/index.vue:151 has a single-occurrence shadcn-style switch card (`FormItem class="flex flex-row items-center justify-between rounded-lg border p-3 shadow-sm"`) that shares SettingsRow's label+description+trailing-control relationship but is wrapped in its own border and appears nowhere else in the codebase — one occurrence isn't enough to confidently call it drift rather than a deliberate one-off, so I left it out of misses. (2) pages/providers/components/provider-form.vue still composes SettingsRow with the legacy `FormItem`/`FormMessage` pattern rather than FieldStack v2 — this is NOT a miss because SettingsRow+FormItem is label-beside-control (a different relationship than FieldStack's label-above-control), so FieldStack doesn't apply here; this is pre-existing SettingsRow-internal debt on a different axis, out of this sweep's scope. The OAuth device-flow block in provider-form.vue (lines 206-354) was read and correctly matches the skill's explicit 'one-off compound block, hand-write it' exception — confirmed LOCAL, not reported.
- **supermarket**: All 7 files in apps/web/src/pages/supermarket read in full (not skimmed) — small enough for complete deep-read: index.vue, plugin-detail.vue, skill-detail.vue, and all 4 files under components/ (install-plugin-dialog.vue, install-skill-dialog.vue, plugin-card.vue, skill-card.vue). No file was truncated or partially read. Cross-checked BackendCard's actual source to judge plugin-card.vue/skill-card.vue against it rather than assuming.
- **onboarding**: All 6 files fully read end-to-end (script + template), 2371 lines total — no skimming was needed given the domain's small size (~6 files as noted in the task). I did not execute the app or render any step visually; classification is from static markup/class-string reading only, per the read-only reconnaissance mandate. I did not trace every downstream composable (useProviderSetup.ts, useACPSetup.ts, useOnboarding.ts, useStepTransition.ts) line-by-line since they contain no template/markup — only enough to confirm they don't hide additional inline templates or render functions that would add undiscovered surfaces.
- **profile-account**: All 7 .vue files in the assigned scope (profile/index.vue, profile/components/{profile-identity,password-section,connected-accounts-section}.vue, people/index.vue, usage/index.vue, about/index.vue) were read in full, top to bottom, including script blocks — none skimmed. Domain is small (3006 lines total) so no triage was needed.
- **provider-domains**: All 15 .vue files under memory/email/video/speech/transcription/voice/platform were read in full (script+template), not skimmed — this domain is small enough for complete deep-read. Confirmed via direct file comparison that speech/provider-setting.vue, transcription/provider-setting.vue, video/provider-setting.vue, video/index.vue, voice/index.vue, email/index.vue, memory/index.vue, model-config-editor.vue, and add-memory-provider.vue are already fully composed of owners (SettingsSection/SettingsRow/FieldStack/FormStack/BackendCard/DetailPane/PageShell) and were not re-reported. The domain note's premise ('memory/speech/video/transcription provider-setting migrated in group-1; confirm-and-skip') holds for speech/transcription/video's provider-setting.vue but does NOT hold for memory's own provider-setting.vue and builtin-config.vue, which were apparently not part of that migration batch and still hand-roll Label+Input fields. platform/index.vue and platform-card.vue were read and judged out-of-scope for owner migration (see diffRelationshipNotes) rather than skipped.</coverageNote>
<parameter name="diffRelationshipNotes">["platform/index.vue + platform-card.vue: a grid of shadcn Card entity tiles (Card/CardHeader/CardFooter with Switch+Edit+Delete buttons) for platform integrations — this is a genuinely different surface (a dashboard card grid, not a settings list) with no current owner; PersonaTile is vertical-centered and doesn't fit a wide card with a config list body, so this is a candidate for a FUTURE owner if more 'object card with inline config preview + footer actions' surfaces appear, but is not itself debt against an existing owner.", "email/index.vue:136-143 hand-rolled dashed-border 'add' tile (min-h-[4.5rem], border-dashed) sitting beside BackendCard results in a grid — this is a one-off compound (an inline dashed add-affordance card, not a BackendCard, not a PersonaTile 'add' variant since it's horizontal/grid-cell not vertical-centered) — kept local intentionally, not flagged.", "speech/components/model-config-editor.vue:190-193 bare <Label> above the 'Test' subsection (not a Label-over-single-control field, just a section caption before a Textarea/file-input test harness) — LOCAL, trivial section label, not FieldStack.", "memory/components/builtin-config.vue:3-19 h2+p heading above a SegmentedControl (mode picker) — this reads as a section header + multi-choice control, not a Label-over-single-control FieldStack; kept local.", "memory/index.vue:73-118 hand-rolled chevron-toggle 'Advanced' disclosure revealing a sibling block of BackendCards + an Add button that the toggle does not own as a single #expanded body — this is exactly the skill's named anti-pattern exception ('a toggle that reveals a sibling block is not an ExpandableSettingsRow'), so correctly kept as a plain button, not flagged as a miss."]
- **shell-misc-pages**: All 10 .vue files that actually exist under the assigned directories were read in full (100% deep-read, 0 skimmed). Two of the nine requested directories contributed no .vue files: `network/` contains only `api.ts` (no components), and `settings-shell/` does not exist in this checkout at all — so effective coverage is complete for what's on disk, but the domain note's expected `network`/`settings-shell` surfaces could not be swept because they aren't present.
- **sidebar-shell**: Full coverage: all 16 .vue files across all 6 target directories (sidebar, settings-sidebar, settings-shell, chat-list/channel-badge, master-detail-sidebar-layout, main-container) were read in full, including the two largest (files-pane.vue 1134 lines, bot-switcher.vue 439 lines). No file was skimmed-only. Every dialog (session-search-dialog, panel-schedule delete confirm, panel-sessions rename/delete, files-pane new-file/mkdir/rename/delete/batch-delete) was read line-by-line to check its body for owner-shapes — all correctly compose Dialog/DialogHeader/DialogFooter/Button/Input rather than hand-rolling settings rows. No sub-scanning limitations; this domain is small enough (~3663 lines total) for exhaustive reading, not sampling.
- **file-manager**: All 3 files in apps/web/src/components/file-manager fully deep-read line-by-line (file-tree-node.vue 242 lines, file-tree.vue 58 lines, file-viewer.vue 950 lines). No files skimmed or skipped. Domain is small and entirely non-settings surface (file tree + Monaco editor chrome), consistent with the domain note's expectation of DIFF-RELATIONSHIP-heavy findings. Zero hard MISSes: no hand-rolled SettingsRow/SettingsSection/FieldStack/MetricReadout/PersonaTile/CalloutBanner duplicates found. Flagged three borderline shapes (tree-panel empty/loading state, editor-pane terminal states) as DIFF-RELATIONSHIP notes for awareness rather than MISS, since none share settings-surface context or geometry with an existing owner.
- **settings-owners**: All 9 .vue files under apps/web/src/components/settings were read in full: backend-card.vue, detail-pane.vue, expandable-row.vue, field-stack.vue, form-stack.vue, metric-readout.vue, row.vue, section.vue, swap-transition.vue. Per the domain note, these files ARE the owner definitions, so their own internal geometry (row.vue's min-h-[3.75rem]/mx-4/border-b, section.vue's card frame, metric-readout.vue's tile frame) is the source of truth and was not flagged. I specifically checked whether any owner file hand-rolls a DIFFERENT owner's shape instead of composing it (the one thing in-scope to report here) by grepping for the row-geometry and card-frame class fingerprints across all 9 files. No unjustified case found.
- **atoms-misc**: Full coverage: all 24 .vue files under the 22 assigned directories were read in full (no skimming), plus the 2 non-Vue support files (markdown/index.ts, provider-icon/icons.ts) were confirmed to be registration/data-only and not UI surfaces. No misses found — this domain is exactly what the domain note predicted: leaf atoms/icons/wrappers that are correctly single-purpose. callout-banner, page-shell, and persona-tile are themselves owner-component sources (not consumers) so they were checked for internal consistency rather than for owner-composition. form-dialog-shell and confirm-popover already compose Dialog/Popover primitives correctly (counted in alreadyMigratedCount). monaco-editor/index.vue and diff.vue are the skill's own named example of a "stay hand-written" one-off compound (Monaco/JSON editor). searchable-select-popover is a one-off virtualized combobox composite built from @memohai/ui menu-class helpers, not a settings surface. color-scheme-card is a preview-swatch picker card, a genuinely different shape from SettingsSection/BackendCard. No settings-row, field-stack, section-card, metric-tile, or footer-band shapes were hand-rolled anywhere in this directory set.
- **dev-wall**: All 20 .vue files under apps/web/src/pages/dev/components/ were read in full (not skimmed) — this is a small, low-priority DEV-ONLY directory so a full deep-read was feasible in one pass. No files were skipped. The sibling .ts files (lib/registry.ts, lib/token-catalog.ts, lib/variant-specs.ts) are outside the requested .vue scope and were not read line-by-line, only referenced for import resolution — they contain no markup and cannot host a spacing-owner shape.

## Guard sufficiency review

This confirms the false-positive gap I expected: the loose `mx-4`+`border-b`+`py-3` co-occurrence pattern nets 18 files (looser, would catch unrelated card/table/divider markup using those same three classes for unrelated reasons), while the exact literal `min-h-[3.75rem]` nets 10 non-owner files — precisely the SettingsRow-height signature, since `3.75rem` is an arbitrary, unusual value nobody would independently reinvent unless copying the row.

I have everything I need. Here's my assessment.

## Findings

### 1. What the guard catches today

`check-ui-contract.mjs` is a **token/value linter**, not a structure linter. Its unit of analysis is a single Tailwind token/class on a single line, string-matched. Three families:

- **px-scaling** (both `packages/ui` and `apps/web`, ratcheted): `text-[Npx]`, `leading-[Npx]`, `h`/`min-h`/`p*`/`gap`/`space-[Npx]` ≥5px.
- **app-injection** (`apps/web` only, ratcheted): hover/active/group-hover fill classes, raw color literals/gray-scale shades, raw `shadow-*` utilities on component tags.
- **contract rules** (`packages/ui` only, hard): disabled-opacity≠40, arbitrary radius, raw color in `.vue` or in `style.css` component layer, raw-color box-shadow, plus WARN-only heuristics (icon-hover, `border-input`, `ring-offset-*`).

Escape hatches: `ui-allow-px` / `ui-allow-style` same-line comments. Baselines (`ui-px-baseline.json`, `ui-app-baseline.json`) grandfather existing counts per file; only *growth* hard-fails.

### 2. What it structurally cannot catch

It has **no concept of a component shape** — no owner vocabulary, no "this div's class combination replicates a component that already exists" rule. Concretely, I verified this against the SettingsRow debt itself: SettingsRow's canonical geometry is the literal `mx-4 flex min-h-[3.75rem] border-b border-border py-3 last:border-b-0` (`apps/web/src/components/settings/row.vue:3`). I grepped for that exact literal outside the owner and its legitimate sibling `expandable-row.vue`:

```
apps/web/src/pages/bots/components/bot-access.vue        (×2)
apps/web/src/pages/bots/components/bot-network.vue        (×3)
apps/web/src/pages/bots/components/bot-heartbeat.vue
apps/web/src/pages/bots/components/bot-email.vue
apps/web/src/pages/bots/components/bot-plugins.vue
apps/web/src/pages/bots/components/channel-settings-panel.vue
apps/web/src/pages/bots/components/bot-user-access.vue
apps/web/src/pages/bots/components/bot-container.vue
apps/web/src/pages/bots/components/bot-skills.vue
apps/web/src/pages/profile/components/connected-accounts-section.vue
```
10 files, several with multiple hand-rolled instances — a live, textbook case of exactly the 同形异码 the owner skill describes. The guard ran clean on this class of debt (its 9 current hard failures are all px/injection, unrelated). This is the gap made concrete: **it's not a hypothetical**, the debt is sitting in the tree right now, invisible to the guard by design, because the guard operates one class-token at a time and has no notion of "this bag of classes IS a SettingsRow."

### 3. Is the gap by-design, or is there a mechanical rule worth adding?

**By-design, mostly** — and the project's own stance in `packages/ui/AGENTS.md` and the owner skill agrees: "Migrate only genuine owner-shapes... Judge each shape against the 'stay hand-written' tells... Don't blind-unify." Distinguishing "same relationship" from "looks similar but is a denser list item / different surface / one-off" is exactly the judgment call [[feedback_unify_over_pretty]] already codifies (统一 > 好看, but only when the relationship is actually the same — not by class-string similarity). A generic structural linter can't make that call; it can only flag *candidates* for a human/agent to triage.

But there **is** a narrow, mechanically sound slice worth adding, and I already validated its precision empirically:

- **The exact literal `min-h-[3.75rem]`** (not a loose `mx-4`+`border-b`+`py-3` co-occurrence) is a near-perfect fingerprint. I compared both: the loose 3-class co-occurrence pattern nets **18 files** (touches ordinary cards/tables/dividers using those classes for unrelated reasons — too noisy, high false-positive risk). The exact `min-h-[3.75rem]` arbitrary-value literal nets **10 files**, all outside the owner files — because `3.75rem` is an unusual, specifically-tuned magic number nobody would independently reinvent. Nobody writes `min-h-[3.75rem]` by coincidence; it only appears because someone copy-pasted or eyeballed-matched the row's height. That's a very low false-positive surface — nearly all hits are named clone candidates, not coincidental matches.

### Proposed rule (if you want to add exactly one)

- **Pattern:** flag the literal token `min-h-[3.75rem]` appearing in any `apps/web/src/**/*.vue` file that is **not** `components/settings/row.vue` or `components/settings/expandable-row.vue` (the two legitimate owners of that geometry).
- **Where it lives:** a fourth rule family in `check-ui-contract.mjs`, WARN not HARD (mirroring the existing WARN tier used for string-detectable-but-judgment-requiring smells like rule 4's icon-hover heuristic) — message: `possible hand-rolled SettingsRow (min-h-[3.75rem] outside components/settings/row.vue) — compose <SettingsRow> unless this is a genuinely different shape (see .agents/skills/memoh-ui-owners/SKILL.md)`.
- **Escape hatch:** none needed beyond the WARN tier itself — WARN doesn't block CI, so no `ui-allow-*` comment is required; a file that's legitimately a denser/different-surface row just keeps the warning uncommented (WARN output is advisory, same as the existing icon-hover / `border-input` / `ring-offset-*` warnings). If it should be silence-able, a `ui-allow-shape` same-line comment would match the existing convention.
- **False-positive rate:** low, empirically ~0% on today's tree (all 10 hits are genuine clones) — but not provably zero forever, since nothing stops a future *legitimately* denser row from landing on the same 3.75rem number by coincidence. That's exactly why WARN (not HARD) is the right severity: it's a pointer for a human/agent to triage against the "stay hand-written" tells, not an automatic block.
- **Scope limit — don't generalize this into "detect any owner-shape clone."** This works because SettingsRow's height happens to be an arbitrary, distinctive literal. FieldStack (`space-y-1.5`), MetricReadout, CalloutBanner etc. don't have an equally rare fingerprint — `space-y-1.5` is a common, unremarkable Tailwind value that appears constantly for unrelated reasons, so the same technique would be noisy there. This is a one-off opportunistic catch, not a template for a general "owner-shape detector."

### Recommendation

Add the single WARN-tier `min-h-[3.75rem]` rule — it's cheap, precise on current evidence, and directly converts 10 files of invisible debt into visible lint output without blocking anyone. But don't try to extend the guard into a general structural/shape linter: that's the review-judgment layer's job (this skill file + a human/agent doing the "same relationship or not" call), and the project's own docs already say so. The guard's job is "no *new* token-level drift slips in unnoticed"; the owner-vocabulary debt is a different failure mode — cross-file shape duplication — that needs the kind of complete-chain read [[skill-frontend-entropy-audit]] already describes, not a stronger grep.
