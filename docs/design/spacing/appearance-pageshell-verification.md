# Appearance PageShell Verification

Date: 2026-07-01

Source page: `apps/web/src/pages/appearance/index.vue`

Target wall sample: `apps/web/src/pages/dev/components/sections/SectionSpacing.vue`

## Purpose

Verify that the spacing wall's `PageShell` / `SettingsSection` / `SettingsRow` sample is equivalent to the real Appearance page, instead of relying on visual inspection.

This check specifically validates the P0 owner chain:

```txt
PageShell -> SettingsSection -> SettingsRow
```

It does not validate `BackendCard`, dialogs, forms, or future settings content owners.

## Method

Measured both pages in the same browser viewport with Playwright and system Chrome:

- viewport: `2048 x 1200`
- source route: `/settings/appearance`
- wall route: `/dev/components#spacing`
- `/users/me` was mocked to return `metadata.onboarding_completed = true`, so the real Appearance route could render past the onboarding guard.

The script read DOM `getBoundingClientRect()` and `getComputedStyle()` values for:

- `PageShell` root;
- page title/header;
- body stack;
- first `SettingsSection`;
- first section label;
- first card;
- first `SettingsRow`;
- first row label.

## Result

All P0 owner measurements matched exactly.

| Relationship / value | Appearance | Spacing wall | Delta |
|---|---:|---:|---:|
| `PageShell` root class | `mx-auto max-w-3xl px-6 pt-10 pb-12` | same | same |
| root max width | `768px` | `768px` | `0` |
| root padding left | `24px` | `24px` | `0` |
| root padding top | `40px` | `40px` | `0` |
| root padding bottom | `48px` | `48px` | `0` |
| header bottom to body | `24px` | `24px` | `0` |
| `h1` padding left | `8px` | `8px` | `0` |
| body stack class | `space-y-8` | `space-y-8` | same |
| section header padding left | `8px` | `8px` | `0` |
| section label to card | `14.25px` | `14.25px` | `0` |
| row margin left | `16px` | `16px` | `0` |
| row padding top | `12px` | `12px` | `0` |
| row padding bottom | `12px` | `12px` | `0` |
| row min height | `60px` | `60px` | `0` |
| row column gap | `16px` | `16px` | `0` |
| root left to section label | `32px` | `32px` | `0` |
| root left to row content | `41px` | `41px` | `0` |
| card left to row | `17px` | `17px` | `0` |

## Conclusion

`PageShell`, `SettingsSection`, and `SettingsRow` are verified against Appearance.

They are allowed in P0.

`SettingsContent` is not verified against Appearance. Appearance proves a complex-row pattern:

```txt
mx-4 border-b border-border py-3
  label + description
  mt-3 grid gap-3 sm:grid-cols-2
```

That is evidence for a future owner, not proof that a generic `SettingsContent` primitive should exist now.

## Current Decision

- Keep `PageShell`, `SettingsSection`, and `SettingsRow` in P0.
- Keep the spacing wall's Appearance complex-row panel as evidence.
- Do not promote `SettingsContent` until Provider/MCP/Schedule validate the need and shape.
