# Spacing Research Expansion Plan

Date: 2026-06-30
Updated: 2026-07-01

Status: mostly executed. Providers, Chat/tool, MCP, and Tool Approval have been audited; the first contract v0 now exists at `docs/design/spacing/spacing-contract-v0.md`.

## Position

After the first two pilots, these families are already strong enough to keep testing:

- `page.*`: page shell, title/action/body rhythm.
- `section.*` and `settings.*`: settings card groups and rows.
- `form.*` and `dialog.*`: create/edit forms, field rhythm, footer actions.
- `sidebar.*`: persistent navigation sidebars.
- `empty.*`, `list.*`, `banner.*`, `metric.*`: still need more evidence, but already have plausible owners.

The next research batch should not just add more settings pages. It should test different product archetypes so the first spacing contract does not overfit the cleanest settings tabs.

## Recommended Next Batch

| Priority | Slice | Why It Matters | Families Tested | Current Confidence | Decision |
|---:|---|---|---|---|---|
| 1 | Providers list/detail/config/models | Highest-density global configuration surface. It combines backend cards, detail pane, settings rows, form fields, model rows, toolbars, empty states, and add/import dialogs. | `page.*`, `list.*`, `detail.*`, `settings.*`, `form.*`, `toolbar.*`, `empty.*` | high | done: `provider-cartography.md` |
| 2 | Chat message stream + tool-call detail | It prevents settings/form spacing from leaking into chat. Chat has a different rhythm: thread width, turn gaps, tool blocks, attachments, composer. | `chat.*`, `tool.*`, `composer.*` | high | done: `chat-tool-cartography.md` |
| 3 | MCP bot tab | It is a bot-tab backend list with add tile, solid empty state, import dialog, JSON editor surface, and detail editor. Good validation after Providers. | `page.*`, `list.*`, `empty.*`, `addTile.*`, `dialog.*`, `editor.*` | high | done: `mcp-tool-approval-cartography.md` |
| 4 | Tool Approval | It is not beautiful, but it tests complex settings rows: expanded rows, inline editors, nested form fields inside a settings card, and row/action density. | `settings.expanded*`, `form.*`, `toolbar.*` | high | done: `mcp-tool-approval-cartography.md` |
| 5 | Global Settings sidebar + Bots grid | The screenshot shows both. It validates sidebar roles and introduces launcher/gallery grid roles for Bots cards and add tile. | `sidebar.*`, `gallery.*`, `card.*`, `addTile.*` | medium-high | validate contract, not a blocker |
| 6 | Onboarding | Important, but should be a boundary sample, not a first contract source. It has theatrical/flow spacing, staged motion, sparse panels, and hero-like composition. | `onboarding.*`, `flow.*`, exception rules | medium as exception | boundary audit, not a blocker |
| 7 | About / sparse page | Another boundary sample. Useful to define what must remain exceptional: sparse vertical bias, centered composition, low information density. | `sparse.*`, exception rules | medium | optional boundary sample |

## Provider Is The Right Next Deep Dive

The Provider screenshots are especially valuable because they contain several kinds of spacing in one workflow:

- Global settings sidebar: persistent nav rhythm.
- Provider gallery: `BackendCard` grid, add/search/action row, empty frame.
- Detail pane: back row to detail body, detail width, top padding.
- Provider identity header: compact entity card with leading icon, name, trailing actions.
- Configuration card: `SettingsSection` + `SettingsRow` used as form rows.
- Footer action row inside a settings card: test/save buttons, inset divider, action gap.
- Model list: toolbar block, search/action cluster, list rows, pagination footer, empty/no-result states.
- Add Provider dialog: older `FormDialogShell`, custom field stack, preset select, OAuth hint, switch row.

This makes Providers a better next slice than MCP if we need one high-yield representative. MCP should follow because it validates the same backend-list ideas inside bot tabs and adds import/editor behavior.

## Do Not Promote Onboarding Too Early

Onboarding definitely has spacing, but it is a poor first source for the product spacing contract.

Reasons:

- It is a flow/experience surface, not a repeated configuration surface.
- It uses staged animation, large display type, centered panels, fixed step heights, and optical composition.
- Some values are intentionally theatrical, for example large title gaps, fixed button widths, step card grids, and bottom progress dots.
- If we promote those values too early, they will pollute day-to-day settings and admin pages.

The better treatment:

- keep onboarding as a boundary sample;
- define a small `onboarding.*` or `flow.*` family only after reviewing all onboarding steps together;
- mark many values as `exception` or `component-local`;
- use it to prove what the first contract should not cover.

## Proposed Additional Families To Add To The Taxonomy

These are not final roles yet, but the next batch should test them:

### Detail Pane

```txt
detail.backToBodyGap
detail.backRowPaddingTop
detail.backRowGutterX
detail.maxWidth.narrow
detail.maxWidth.standard
detail.sectionStackGap
```

Likely owners: `DetailPane`, `SettingsShell`, `PageShell`.

### Toolbar

```txt
toolbar.controlGap
toolbar.searchWidth
toolbar.padding
toolbar.toListGap
toolbar.actionClusterGap
```

Likely owners: toolbar block primitive, `PageShell` actions slot, list section primitive.

### Add Tile / Gallery

```txt
gallery.cardGap
gallery.cardPadding
gallery.cardMinHeight
gallery.addTileMinHeight
gallery.addTileContentGap
```

Likely owners: `BackendCard`, `BotCard`, `AddTile`, gallery/list primitive.

### Dense List Rows

```txt
list.toolbarPadding
list.rowPaddingX
list.rowPaddingY
list.rowActionGap
list.footerPadding
list.emptyPaddingY
```

Likely owners: model list/table-like primitive.

### Flow / Onboarding

```txt
flow.viewportPadding
flow.panelMaxWidth
flow.titleToDescription
flow.descriptionToAction
flow.footerGap
flow.stepIndicatorGap
```

Default decision: `defer` or `exception` until the onboarding slice is audited as a whole.

## Suggested Execution Order

1. **Completed**: Providers deep dive.
2. **Completed**: Chat/tool detail deep dive.
3. **Completed**: MCP + Tool Approval paired audit.
4. **Completed**: Synthesis pass and `spacing-contract-v0.md`.
5. **Next validation**: sidebar/gallery follow-up using global settings sidebar plus Bots grid screenshot.
6. **Boundary validation**: onboarding audit, mostly to classify exceptions and possible `flow.*` roles.

## First Contract Guardrail

The first contract should include only relationships with clear owners:

- `PageShell`
- `SettingsSection` / `SettingsRow`
- `FormStack` / `FieldStack`
- `DialogContent` / `FormDialogShell`
- `NavItem` / sidebar primitives
- `BackendCard` / `AddTile` if Providers + MCP confirm them
- `FramedEmpty`

It should not include onboarding/sparse-page values except as documented exceptions.
