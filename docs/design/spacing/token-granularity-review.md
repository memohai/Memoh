# Spacing Token Granularity Review

Date: 2026-07-01

Status: calibration note after external best-practice review. This document decides how much of `spacing-contract-v0.md` should become actual tokens.

## Short Answer

The current `spacing-contract-v0.md` has too many names **if every name becomes a public design token**.

It is reasonable **if most names stay as relationship documentation or component-owned contracts**, and only a small subset becomes public tokens or component APIs.

Memoh should not ship 80+ semantic spacing tokens. It should ship:

1. a small primitive scale;
2. a small set of public layout/component primitives;
3. component-owned internal aliases;
4. a relationship ledger that explains why components use those values;
5. explicit exceptions.

## External Calibration

Sources reviewed:

- Atlassian spacing: https://atlassian.design/foundations/spacing
- Atlassian primitives / Box: https://atlassian.design/components/primitives/box
- Carbon spacing: https://carbondesignsystem.com/elements/spacing/overview/
- Carbon spacing code: https://carbondesignsystem.com/elements/spacing/code/
- Shopify Polaris space tokens: https://polaris-react.shopify.com/tokens/space
- Shopify Polaris layout tokens: https://polaris-react.shopify.com/design/layout/layout-tokens
- Shopify Polaris BlockStack: https://polaris-react.shopify.com/components/layout-and-structure/block-stack
- Adobe Spectrum spacing: https://spectrum.adobe.com/page/spacing/
- W3C/DTCG design token format: https://www.designtokens.org/TR/2025.10/format/

### What Mature Systems Usually Do

They do **not** create a semantic token for every relationship visible in the product.

They usually combine:

- **Primitive space scale**: compact, enumerable values such as 2px, 4px, 6px, 8px, 12px, 16px, 24px, 32px, 40px, 64px.
- **Usage guidance**: small values for compact internals, medium values for component padding, large values for layout/page rhythm.
- **Layout primitives**: Box, Stack, Inline/Flex, Grid, Bleed, Card, Page.
- **A few component semantic tokens**: card padding, card gap, button group gap, table cell padding.
- **Aliases / references**: semantic component tokens often alias primitive tokens.
- **Exceptions**: centering, auto, gutters, percentages, optical offsets, or responsive jumps are treated as layout mechanisms, not necessarily semantic tokens.

### External Pattern Summary

| System | What it suggests for Memoh |
|---|---|
| Atlassian | Keep a small numeric scale, use primitives such as Box/Flex/Grid/Bleed, and document range usage instead of inventing many relationship names. |
| Carbon | Tokens can work inside and between components, but parent-owned layout spacing is preferred; spacing scale complements grid/type/density. |
| Shopify Polaris | Primitive space tokens and a few component-level tokens can coexist; examples include card padding, card gap, button group gap, table cell padding. |
| Adobe Spectrum | Tokens are design decisions turned into data; the token exists because a decision matters, not because a value exists. |
| W3C/DTCG | Aliases, inheritance, and component-level references are normal. A semantic token may reference a primitive token rather than being an independent value. |

## Memoh Interpretation

`spacing-contract-v0.md` should be read as a **relationship contract**, not a token manifest.

For example:

```txt
settings.rowInsetX
settings.rowMinHeight
settings.rowPaddingY
settings.rowColumnGap
settings.labelToDescription
```

These probably should **not** all be public CSS variables used directly by page authors. The better owner is `SettingsRow`. Page authors should write:

```vue
<SettingsRow ... />
```

not:

```vue
<div class="mx-[var(--spacing-settings-row-inset-x)] ...">
```

The relationship names still matter because they explain what `SettingsRow` owns and what must not drift.

## The Same-Frequency / Different-Code Problem

Memoh Web already has a similar idea: the same product-language decision can appear under different local code shapes. Spacing has the same problem, but with two directions.

### Same Frequency, Different Code

Two implementations look different in code, or use slightly different values, but should probably collapse to one role because they serve the same perceptual rhythm.

Examples:

| Local code A | Local code B | Likely shared relationship | Decision |
|---|---|---|---|
| New Task `space-y-1.5` label/control | older dialog `mb-2` label/control | `form.labelToControl` | same role, tune one value later |
| Page `mb-6` title/body | tab `pt-6` after container padding | page/tab top rhythm | same product rhythm, different environment |
| Settings content `space-y-5` | compact OAuth content `space-y-3` | `settings.contentStackGap` with density variants | same family, not two public tokens yet |
| Root tool detail `mt-1.5` | grouped tool detail `mt-1` | `tool.detailTopGap` with variant | same role family, variant local to owner |

This is why grep is weak: it sees different values and assumes different concepts. We need to classify the felt rhythm.

### Same Code, Different Frequency

The opposite is also dangerous. The same value can mean unrelated things.

Examples:

| Same value | Different relationships | Decision |
|---|---|---|
| `gap-3` | backend card media gap, sidebar identity media gap, list row column gap, entity header media gap | do not merge into one `gap-3` semantic token |
| `gap-2` | toolbar action gap, composer attachment gap, form inline controls, popover footer actions | keep owner-specific |
| `p-4` | settings content padding, dialog editor body padding, card content padding | may alias same primitive, but not necessarily same semantic role |

This is why token names must include owners. A value can repeat because the primitive scale is small, not because the relationship is the same.

## Four-Layer Model For Memoh

### Layer 0: Primitive Scale

Public and small.

```txt
space.0      0
space.0_5    2px
space.1      4px
space.1_5    6px
space.2      8px
space.2_5    10px
space.3      12px
space.3_5    14px
space.4      16px
space.5      20px
space.6      24px
space.8      32px
space.10     40px
space.12     48px
space.16     64px
```

These are values. They do not explain product meaning by themselves.

### Layer 1: Public Layout / Component Primitives

Public and small. These are what page authors should use.

Initial target set:

```txt
PageShell
SettingsSection
SettingsRow
SettingsContent
ExpandableSettingsRow
FormStack
FieldStack
FormDialogShell
BackendCard
AddTile
FramedEmpty
DenseListSection / ObjectListRow
ChatSurface
Composer
ToolProcess
```

These primitives may internally use semantic role aliases, but authors should not import dozens of spacing tokens.

### Layer 2: Component-Owned Aliases

Mostly internal. They can exist as CSS variables, TS constants, or documented component recipes, but they are not a broad page-author API.

Examples:

```txt
--settings-row-inset-x
--settings-row-padding-y
--settings-content-padding
--backend-card-padding
--composer-padding-y
--tool-detail-padding-x
```

Use when a component needs internal consistency, variants, or theming. Avoid exposing these as global "pick any" tokens.

### Layer 3: Relationship Ledger

Documentation only unless a component needs it.

Examples:

```txt
chat.senderNameToBody
sidebar.identityStatusGap
tool.detailDenseStackGap
settings.contentDividerTopGap
gallery.addTileContentGap
```

These are useful names for review, migration, and component-wall education. They do not all need implementation artifacts.

### Layer 4: Exceptions

Documented and deliberate.

Examples:

```txt
composer.welcomeOffset = pt-[38dvh]
chat.threadMaxWidth = 840px
about/sparse vertical bias
onboarding staged panel gaps
output max heights
avatar sizes
control widths
```

Exceptions should not be converted into normal semantic tokens unless they recur with the same product meaning.

## Token Budget Recommendation

For v0, Memoh should aim for this rough budget:

| Category | Target count | Notes |
|---|---:|---|
| primitive space values | 12-16 | public |
| public composition primitives | 10-16 | components, not tokens |
| public semantic spacing tokens | 0-8 | only if multiple primitives need the same role |
| component-owned aliases | 20-40 | internal or semi-private |
| relationship names in docs | unlimited-ish, but reviewed | useful for reasoning, not API |

The important distinction: **a relationship name is cheap in docs, expensive in API**.

## What Should Become Public In V0

### Public Components / Primitives

These should be the main API:

```txt
PageShell
SettingsSection
SettingsRow
SettingsContent
ExpandableSettingsRow
FormStack
FieldStack
FormDialogShell
BackendCard
AddTile
FramedEmpty
```

Maybe later:

```txt
DenseListSection
ObjectListRow
ChatSurface
Composer
ToolProcess
```

### Public Semantic Tokens

Be extremely conservative. Possible public semantic tokens:

```txt
--page-max-width
--chat-thread-max-width
--settings-card-radius / already radius-owned elsewhere
--settings-row-min-height
```

Even these may be better as component internals unless external consumers need them.

### Internal Component Aliases

These are reasonable:

```txt
--page-gutter-x
--page-header-to-body
--section-label-to-surface
--settings-row-inset-x
--settings-row-padding-y
--settings-row-column-gap
--settings-content-padding
--form-field-gap
--form-label-to-control
--backend-card-padding
--backend-card-media-gap
--empty-outer-padding-y
```

But page authors should usually not choose them directly.

## Compression Pass On Current V0

The current contract can be compressed for implementation:

| Current family | Keep as public primitive? | Keep as token? | Notes |
|---|---:|---:|---|
| `page.*` | yes through `PageShell` | maybe 2-3 internals | `PageShell` already owns most. |
| `section.*` | yes through `SettingsSection` | internal only | Do not expose many section tokens. |
| `settings.row*` | yes through `SettingsRow` | internal only | Strongest component contract. |
| `settings.content*` | yes through `SettingsContent` | internal only | Add primitive first. |
| `dialog.*` / `form.*` | yes through `FormStack` / `FieldStack` / shells | internal only | Values still need reconciliation. |
| `sidebar.*` | yes through sidebar primitives | internal only | Desktop/mac offsets may stay local. |
| `gallery.*` / `backendCard.*` | yes through `BackendCard` / `AddTile` | internal only | Very good primitive candidate. |
| `list.row*` | maybe later | internal/defer | Need another list before value tuning. |
| `empty.*` | yes through `FramedEmpty` / `Empty` wrappers | internal only | Rules more important than tokens. |
| `chat.*` | maybe later through `ChatSurface` | mostly internal/defer | Keep separate from settings. |
| `composer.*` | maybe later through `Composer` | mostly internal/defer | Welcome state stays exception. |
| `tool.*` | maybe later through `ToolProcess` | mostly internal/defer | Dense log owner. |

## Revised Principle

Replace:

```txt
Adopt every high-confidence role as a token.
```

with:

```txt
Adopt every high-confidence role as a documented component responsibility.
Promote to a token only when multiple owners must coordinate the same decision.
```

## Decision Rule

Before creating a semantic spacing token, answer yes to at least two of these:

- More than one component owner needs this exact relationship.
- Designers need to tune this relationship globally across surfaces.
- The value changes under density/theme/platform modes.
- It must be visible in Figma/design-token tooling.
- It cannot be enforced by a single component primitive.

If not, keep it as:

- primitive scale value;
- component internal alias;
- prop/variant;
- documentation;
- exception.

## Recommendation For Next Step

Revise `spacing-contract-v0.md` language so readers do not mistake it for a token manifest.

Suggested wording:

```txt
This contract names spacing relationships and owners. Only a small subset should become public tokens.
Most adopted relationships should be enforced by composition primitives.
```

Then build the component wall around primitives, not a giant semantic-token table.
