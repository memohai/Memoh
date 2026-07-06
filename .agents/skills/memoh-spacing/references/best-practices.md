# Spacing Best Practices

Use this reference to calibrate Memoh's spacing work against mature systems. Do not copy names blindly. Extract the underlying system shape.

## Common Pattern

Mature systems usually separate:

1. **Primitive scale**: a limited set of values.
2. **Usage guidance**: what small, medium, and large values are for.
3. **Semantic or component tokens**: named decisions such as card padding or table cell padding.
4. **Layout primitives**: Stack, Inline, Box, Page, Card, Grid, Bleed, or app-specific equivalents.
5. **Tooling and docs**: component walls, examples, lint rules, migration guides.

Memoh should follow the shape, not the exact naming.

## Atlassian

Source: https://atlassian.design/foundations/spacing

Observed principles:

- Uses an 8px base unit.
- Defines a limited scale from 0px to 80px.
- Names tokens as `space.025`, `space.050`, `space.100`, etc.
- States that space tokens should replace raw pixel or rem values when adding space between components or objects.
- Groups usage ranges:
  - 0px to 8px for compact UI, icon/text gaps, badge/icon-button padding, input padding, title-description gaps.
  - 12px to 24px for larger component padding and spacing between larger component items.
  - 32px to 80px for page-level layout spacing.
- Includes layout guidelines: group by similarity, group by proximity, create hierarchy, introduce rhythm, and use optical adjustment.
- Provides negative tokens but suggests a Bleed primitive before reaching for negative whitespace.

Memoh takeaways:

- Keep primitive scale small.
- Document ranges by use, not just numbers.
- Treat optical adjustment as allowed but named and reviewed.
- Prefer primitives for recurring layout moves.

## Carbon

Source: https://carbondesignsystem.com/elements/spacing/overview/

Observed principles:

- Spacing is negative area between elements and components, commonly controlled by margin and padding.
- Carbon provides tokens and layout components to reduce guesswork.
- Its spacing scale complements its grid and type scale.
- It uses multiples of two, four, and eight.
- Small increments support detail-level spatial relationships; larger increments control density.
- Tokens can be used inside components and between components.
- Stack delegates layout spacing to parent components instead of relying on child margins.
- Carbon explicitly allows non-token methods such as center, auto, and gutter for specific layout cases.
- Deviations from the scale are exceptions and should be avoided whenever possible.
- Tokens are not responsive by themselves, but responsive layouts may jump scale steps at breakpoints.

Memoh takeaways:

- Do not confuse component internals with page-level density.
- Prefer parent-owned layout gap over child margins.
- Distinguish tokenized space from other layout mechanisms such as auto, centering, gutters, and percentages.
- Record exceptions instead of pretending they are reusable roles.

## Shopify Polaris

Source: https://polaris-react.shopify.com/tokens/space

Observed principles:

- Provides primitive space tokens such as `--p-space-0`, `--p-space-025`, `--p-space-050`, `--p-space-100`, up through larger values.
- Includes component-level semantic tokens such as `--p-space-button-group-gap`, `--p-space-card-gap`, `--p-space-card-padding`, and `--p-space-table-cell-padding`.
- The scale includes 1px, 2px, 4px, 6px, 8px, 12px, 16px, 20px, 24px, 32px, 40px, 48px, 64px, and larger.

Memoh takeaways:

- A primitive scale and a few semantic component tokens can coexist.
- Semantic tokens should be few and clearly owned by components or patterns.
- It is acceptable for semantic tokens to alias primitive tokens.

## Adobe Spectrum

Source: https://spectrum.adobe.com/page/spacing/

Observed principles:

- Treats spacing as design tokens.
- Uses a scale with small increments such as 2px, 4px, and 8px.
- Frames design tokens as translated design decisions and a source of truth.

Memoh takeaways:

- Spacing belongs in the design token conversation.
- The token exists because the decision matters, not because a value exists.

## Memoh Rule Of Thumb

Borrow this combined rule:

```txt
Primitive scale values are cheap.
Semantic spacing roles are expensive.
Composition primitives are the preferred way to make roles usable.
Exceptions are allowed, but must be named.
```

Do not start by defining every semantic role. Start with interface slices, then adopt the smallest set of roles that explains repeated product relationships.
