# Bot Overview Spacing Cartography

Date: 2026-06-30

Status: pilot slice, not a final spacing contract.

## Slice

- Archetype: bot overview / settings-readout dashboard hybrid.
- Primary file: `apps/web/src/pages/bots/components/bot-overview.vue`.
- Shared primitives read:
  - `apps/web/src/components/page-shell/index.vue`
  - `apps/web/src/components/settings/section.vue`
  - `apps/web/src/components/settings/row.vue`
- Current gold-reference status: no gold page assumed. This slice is useful because it contains many recurring relationships, not because every value is final.

## Summary

`bot-overview.vue` already has a recognizable spacing grammar:

- tab page shell: centered `max-w-3xl`, top/bottom tab padding, page title, section stack;
- section label rhythm: label inset and label-to-surface gap;
- settings-row rhythm: inset rows, row minimum height, row padding, label-description gap, action gap;
- status banner rhythm: full-width notice with icon, content, and trailing affordance;
- metric readout rhythm: section label plus sibling metric tiles;
- chart/content block rhythm inside a settings surface.

The relationship confidence is high for the page, section, settings row, banner, and metric families. Value confidence is mostly medium because the page was not tuned as a final gold reference and some values are copied from local recipes.

## Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| tab page content width | `mx-auto max-w-3xl` in `bot-overview.vue:10`; also `PageShell` tab variant | keep bot tab content readable and aligned with settings pages | `page.maxWidth` | `PageShell` | high | medium | adopt relationship | The page hand-rolls what `PageShell variant="tab"` now owns. |
| tab page top/bottom padding | `pt-6 pb-8` in `bot-overview.vue:10`; `PageShell` tab variant uses the same | compensate for parent bot-detail tab container padding | `page.paddingTop.tab`, `page.paddingBottom.tab` | `PageShell` | high | medium | adopt relationship | Value should remain provisional until tab shell is reviewed across bot tabs. |
| page title to body | title `mb-6` in `bot-overview.vue:11`; `PageShell` header wrapper uses `mb-6` | separate page title from first body block | `page.headerToBody` | `PageShell` | high | medium | adopt relationship | Good first example of high relationship confidence / provisional value confidence. |
| page title inset | `px-2` on `h1` in `bot-overview.vue:11`; `PageShell` uses `pl-2` | align title text with section labels and inset rows | `page.titleInsetX` | `PageShell` | high | medium | adopt relationship | Relationship is strong; value should be validated with full-width body variants. |
| major section stack | `space-y-8` on body wrapper in `bot-overview.vue:15` | separate unrelated overview blocks | `section.stackGap` | page section stack primitive or `PageShell` body convention | high | medium | adopt relationship | Appears in settings pages and bot tabs. |
| section label to surface | `space-y-2.5` in reminder/runtime custom sections and `SettingsSection` | visually attach label to the surface below without crowding | `section.labelToSurface` | `SettingsSection` / section-like primitive | high | medium | adopt relationship | Used for both card surfaces and metric tile groups. |
| section label inset | `px-2` on `h2` or title row in custom sections; `SettingsSection` title row uses `px-2` | align labels with page title and row content rail | `section.labelInsetX` | `SettingsSection` / section-like primitive | high | medium | adopt relationship | Should be documented as a rail, not as arbitrary padding. |
| section header action/status gap | runtime header `gap-2 px-2` in `bot-overview.vue:168`; `SettingsSection` title row has `gap-4 px-2` | allow label plus badge/action on one baseline | `section.headerInlineGap` | `SettingsSection` or `SectionHeader` primitive | medium | low | defer | Current values diverge (`gap-2` vs `gap-4`) depending on badge/action density. |
| status banner frame | issue banner `rounded menu-shell border ... px-4 py-3` in `bot-overview.vue:20` | full-width state notice that reads as its own surface | `banner.paddingX`, `banner.paddingY` | new `StatusBanner` | high | medium-low | adopt relationship, tune value later | Color/token choices are out of scope here but also need normalization. |
| status banner icon/content gap | issue banner `gap-3` in `bot-overview.vue:20` | separate leading icon, message body, and affordance | `banner.contentGap` | new `StatusBanner` | high | medium | adopt relationship | Recurs in other state notices; should be audited across slices. |
| reminder row frame | `mx-4 min-h-[3.75rem] gap-4 py-3 border-b` in `bot-overview.vue:54` | settings-like actionable setup row inside a card | `settings.rowInsetX`, `settings.rowMinHeight`, `settings.rowColumnGap`, `settings.rowPaddingY` | `SettingsRow` or row-like primitive | high | medium | adopt relationship | Hand-written because row content includes custom loop; should likely use an extended row primitive. |
| row label to description | `mt-0.5` in reminder/platform/config rows | make description subordinate to label | `settings.labelToDescription` | `SettingsRow` | high | medium-high | adopt relationship | Very stable relation. |
| platform loading row | `mx-4 min-h-[3.75rem] gap-3 py-3` in `bot-overview.vue:92` | keep loading state height aligned with populated/empty platform row | `settings.rowLoadingGap`, plus row height/padding roles | `SettingsRow` or loading row primitive | medium-high | medium | adopt relationship, defer exact role | Gap differs from action rows because skeleton/icon layout differs. |
| platform empty/action row | `mx-4 min-h-[3.75rem] justify-between gap-4 py-3` in `bot-overview.vue:103` | same row rhythm as reminders, without divider when single row | settings row roles | `SettingsRow` or row-like primitive | high | medium | adopt relationship | Confirms row semantics beyond formal `SettingsRow` component. |
| platform configured row | `mx-4 min-h-[3.75rem] gap-3 border-b py-3` in `bot-overview.vue:127` | compact status row with icon, title, status | `settings.rowMediaGap`, plus row height/padding roles | `SettingsRow` variant or `SettingsMediaRow` | medium-high | medium | defer variant naming | The relationship is row-like, but action gap and media gap differ. |
| row status dot to text | status cluster `gap-1.5` in `bot-overview.vue:138` | keep status dot attached to status text | `status.inlineGap` | local status/badge primitive | medium | medium | component-local/defer | Likely belongs to status chip/dot component rather than global spacing. |
| runtime section label to metrics | runtime section `space-y-2.5` and header `px-2` | metrics are a surface-equivalent, not a settings card | `section.labelToSurface` | section-like primitive | high | medium | adopt relationship | Confirms section rhythm works for non-card content. |
| metric tile group gap | `grid grid-cols-3 gap-3` in `bot-overview.vue:190` | separate sibling readout tiles without card-in-card | `metric.tileGap` | new `MetricReadout` | high | medium | adopt relationship, tune value later | Should be compared with `bot-container.vue` metric tiles. |
| metric tile padding | `px-3 py-2.5` in `bot-overview.vue:195` | compact telemetry card body | `metric.tilePaddingX`, `metric.tilePaddingY` | new `MetricReadout` | high | low-medium | adopt relationship, tune value later | Value may differ between dense runtime metrics and larger dashboard readouts. |
| metric label to value | `mt-0.5` in `bot-overview.vue:200` | bind metric label to primary value | `metric.labelToValue` | `MetricReadout` | high | medium | adopt relationship | Same numeric value as settings label-description, but semantic owner differs. |
| no-metrics note inset | `px-2` in `bot-overview.vue:216` | align note with section label rail | `section.bodyInsetX` or reuse `section.labelInsetX` | section-like primitive | medium | medium | defer | Needs more examples before naming. |
| config settings rows | formal `SettingsSection` + `SettingsRow` in `bot-overview.vue:225` | simple readout settings use standard row primitive | settings row roles | `SettingsSection`, `SettingsRow` | high | medium-high | adopt relationship | This is the cleanest row case in the slice. |
| small inline reasoning badge padding | `px-1.5 py-0.5` in `bot-overview.vue:232` | compact tag attached to model row | badge/tag component geometry | `Badge` / tag primitive | medium | medium | component-local | Do not promote to spacing role yet. |
| usage content block padding | `space-y-4 p-4` inside `SettingsSection` in `bot-overview.vue:247` | non-row card content with stats and chart | `surface.paddingCompact`, `surface.contentGap` | settings content block primitive | high | medium | adopt relationship, tune naming | This repeats in non-row settings cards. Avoid overly vague `surface` naming if possible. |
| usage stat grid gaps | `gap-x-4 gap-y-3` in `bot-overview.vue:248` | compact 2x/4x stat readout inside a card | `metric.inlineGapX`, `metric.inlineGapY` or component-local | `MetricReadout` / usage stats primitive | medium | low-medium | defer | Needs comparison with usage page and container readouts. |
| usage chart fixed height | `h-[200px]` / inline `height: 200px` | stable chart frame, prevent layout collapse | chart size token/local geometry | chart component/local | medium | medium | component-local/defer | This is size, not spacing; do not mix into spacing roles. |

## Patterns To Extract

| Primitive | Owns roles | Current examples | Why extract | Priority |
|---|---|---|---|---|
| `PageShell` adoption for bot overview | `page.maxWidth`, `page.paddingTop.tab`, `page.paddingBottom.tab`, `page.headerToBody`, `page.titleInsetX` | `bot-overview.vue:10-15`, `page-shell/index.vue:14-55` | Bot overview hand-rolls a now-existing shell. | medium |
| Extended settings row / media row | settings row roles plus media/action variants | reminder rows, platform loading/empty/configured rows | Row-like blocks repeat but cannot always use simple `SettingsRow`. | high |
| `StatusBanner` | `banner.paddingX`, `banner.paddingY`, `banner.contentGap` | issue banner in overview | State notices should not hand-roll padding, radius, icon gap, and surface. | high after auditing more banners |
| `MetricReadout` | metric tile roles | runtime metric grid; usage stats partly related | Readout spacing is distinct from settings rows and should stay outside `SettingsSection` card-in-card patterns. | medium-high |
| Settings content block | `settings.contentPadding`, `settings.contentGap` or better-named equivalent | usage card body `p-4 space-y-4` | Non-row content inside a settings card repeats and needs a name. | medium |

## Exceptions And Local Geometry

| Item | Current implementation | Reason | Decision |
|---|---|---|---|
| ECharts height | `height: 200px` inline and `h-[200px]` fallback | chart component sizing, not spacing; inline style needed because ECharts overrides height | component-local/defer to chart guidance |
| Reasoning badge padding | `px-1.5 py-0.5` | compact badge geometry | component-local |
| Status dot gap | `gap-1.5` | dot/text micro cluster | component-local or status primitive |

## Candidate Role Matrix

| Candidate role | Overview | Dialog | Backend list | Chat | Relationship confidence | Value confidence | Decision | Owner |
|---|---:|---:|---:|---:|---|---|---|---|
| `page.maxWidth` | yes | no | likely | no | high | medium | adopt relationship | `PageShell` |
| `page.paddingTop.tab` | yes | no | no | no | high | medium | adopt relationship | `PageShell` |
| `page.paddingBottom.tab` | yes | no | no | no | high | medium | adopt relationship | `PageShell` |
| `page.headerToBody` | yes | no | likely | no | high | medium | adopt relationship | `PageShell` |
| `page.titleInsetX` | yes | no | likely | no | high | medium | adopt relationship | `PageShell` |
| `section.stackGap` | yes | no | likely | no | high | medium | adopt relationship | page body convention |
| `section.labelToSurface` | yes | no | likely | no | high | medium | adopt relationship | `SettingsSection` / section primitive |
| `section.labelInsetX` | yes | no | likely | no | high | medium | adopt relationship | `SettingsSection` / section primitive |
| `settings.rowInsetX` | yes | no | no | no | high | medium | adopt relationship | `SettingsRow` |
| `settings.rowMinHeight` | yes | no | no | no | high | medium | adopt relationship | `SettingsRow` |
| `settings.rowPaddingY` | yes | no | no | no | high | medium | adopt relationship | `SettingsRow` |
| `settings.rowColumnGap` | yes | no | no | no | high | medium | adopt relationship | `SettingsRow` |
| `settings.labelToDescription` | yes | maybe | no | no | high | medium-high | adopt relationship | `SettingsRow` |
| `banner.paddingX` | yes | maybe | maybe | maybe | high | low-medium | adopt relationship, tune later | `StatusBanner` |
| `banner.paddingY` | yes | maybe | maybe | maybe | high | low-medium | adopt relationship, tune later | `StatusBanner` |
| `banner.contentGap` | yes | maybe | maybe | maybe | high | medium | adopt relationship | `StatusBanner` |
| `metric.tileGap` | yes | no | maybe | no | high | medium | adopt relationship, tune later | `MetricReadout` |
| `metric.tilePaddingX` | yes | no | maybe | no | high | low-medium | adopt relationship, tune later | `MetricReadout` |
| `metric.tilePaddingY` | yes | no | maybe | no | high | low-medium | adopt relationship, tune later | `MetricReadout` |
| `metric.labelToValue` | yes | no | maybe | no | high | medium | adopt relationship | `MetricReadout` |
| `settings.contentPadding` | yes | likely | maybe | no | high | medium | adopt relationship, naming needs care | settings content block |
| `settings.contentGap` | yes | likely | maybe | no | high | medium | adopt relationship, naming needs care | settings content block |

## First Slice Conclusions

1. The strongest relationships are page shell, section rhythm, settings row rhythm, banner rhythm, and metric readout rhythm.
2. Bot Overview confirms that Memoh should adopt relationships before finalizing values: most values are plausible, but not gold-reference proven.
3. `PageShell`, `SettingsSection`, and `SettingsRow` are already semantic carriers; the debt is mostly hand-written row variants and missing primitives for banners, metric readouts, and non-row card content.
4. The metric/readout family should stay distinct from settings rows. It can share section label rhythm without being wrapped in `SettingsSection`.
5. The next slices should test this matrix against a form dialog, a backend list/empty state, and chat/tool detail before any broad migration.

## Open Questions

- Should `bot-overview.vue` move to `PageShell variant="tab"` now, or wait until the spacing contract is reviewed?
- Should row-like media/action/loading variants become `SettingsRow` props, or a separate `SettingsMediaRow` / `SettingsActionRow` family?
- Should `surface.paddingCompact` be named generically, or should Memoh prefer a more specific `settings.contentPadding` role?
- Do banner roles belong in `@memohai/ui` as an Alert variant, or in `apps/web` as `StatusBanner`?
- Should metric tiles support dense and standard density variants from the start?
