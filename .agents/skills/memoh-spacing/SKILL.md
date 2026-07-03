---
name: memoh-spacing
description: "SUPERSEDED by memoh-ui-owners — do not use for building or migrating UI. This skill is retained ONLY as the historical research record (the cartography/role-taxonomy method that produced the owner vocabulary). For any actual work — composing a settings row / form field / section / tile, or migrating a hand-rolled surface — use memoh-ui-owners instead."
---

# Memoh Spacing (SUPERSEDED — research record only)

> **This skill is retired.** The spacing system it set out to define is **built and shipped**:
> ten owner components, the full click-surface + coverage audits, and all migrations are
> complete (see `docs/design/spacing/owner-vocabulary-census.md`). For building or changing
> any Memoh Web surface, use **`memoh-ui-owners`** — that is the living contract. This file is
> kept only because the audits still cite its cartography (`references/cartography.md`,
> `references/role-taxonomy.md`) as the research trail that derived the owners. Read it to
> understand *how the vocabulary was discovered*, not *what to do now*.


Use this skill to build Memoh's spacing system from real product interfaces. The goal is not to tokenize every `gap-4`; the goal is to name reusable spatial decisions and give them owners.

Core principle:

> A spacing role is not a value. It is a reusable decision about a spatial relationship.

Memoh may not have a gold reference page yet. Do not require one before making progress. Adopt relationships separately from values: a role can have high relationship confidence while its current value remains provisional.

## Required Context

For migration or implementation work, read
`/Users/qqqqqf/Documents/Memoh-spacing/docs/design/spacing/spacing-contract-v1.md`
first. It is the authoritative V1 contract. Older spacing documents are evidence and
working notes, not competing contracts.

Before changing or proposing anything in `apps/web` or `packages/ui`, read:

1. `/Users/qqqqqf/Documents/Memoh-spacing/.agents/skills/memoh-web/SKILL.md`
2. `/Users/qqqqqf/Documents/Memoh-spacing/.agents/skills/memoh-web/reference.md`
3. `/Users/qqqqqf/Documents/Memoh-spacing/packages/ui/AGENTS.md`
4. The nearest `AGENTS.md` for any directory being audited or edited.

Use this skill together with `memoh-web` for frontend work. `memoh-web` governs current page/component rules; this skill governs spacing-system discovery and role design.

## Workflow

### 1. Calibrate From Mature Systems

When starting a new spacing-system effort or revisiting role definitions, read `references/best-practices.md`.

Use outside systems only as calibration. Do not copy their token names wholesale. Extract principles:

- keep a limited primitive scale;
- distinguish space from size;
- use small values for dense component internals and larger values for page structure;
- use layout primitives to carry recurring spacing decisions;
- allow optical adjustments, but record them as local geometry or exceptions.

### 2. Pick Interface Slices

Start from a few high-frequency Memoh interface slices, not the whole codebase. Good first slices:

- Bot General / Overview or a bot settings tab;
- a create/edit dialog or form;
- provider/backend list and empty/add states;
- chat message plus tool detail;
- onboarding, bot launcher, or About as special non-settings surfaces.

For screenshot-led work, annotate the visible spatial relationships first, then map to code. For code-led work, read the concrete page/component files and reconstruct the same relationships.

Read `references/cartography.md` for the slice ledger format and subagent workflow.

### 3. Name Relationships Before Values

For each slice, identify relationships:

- pane edge to content;
- page title to body;
- section label to surface;
- row label to description;
- row content to action;
- form label to control;
- card/list item gap;
- empty-state frame padding;
- message turn gap;
- tool detail inset;
- composer padding.

Only after naming the relationship should you record the current class or value.

Track two confidence levels independently:

- **Relationship confidence**: how sure the spatial relationship is real and reusable.
- **Value confidence**: how sure the current value is the right visual default.

Example:

```txt
page.headerToBody
relationship confidence: high
value confidence: medium
current value: mb-6
decision: adopt relationship, tune value later
```

Bad output:

```txt
gap-4 appears many times.
```

Good output:

```txt
settings.rowColumnGap currently maps to gap-4 in SettingsRow and several row-like hand-written blocks.
Owner: SettingsRow or a row-like primitive.
Decision: adopt candidate role.
```

### 4. Decide The Owner

Every adopted role needs an owner. Prefer owners in this order:

1. Existing composition primitive, for example `PageShell`, `SettingsSection`, `SettingsRow`.
2. New composition primitive, for example `StatusBanner`, `FramedEmpty`, `MetricReadout`.
3. Component-local geometry, for one component's internal mechanics.
4. Documented exception, for a deliberate one-off layout.
5. Primitive scale only, for values that are useful but not semantic.

Do not create a free-floating semantic token if no owner can enforce or teach it.

### 5. Use The Decision States

Classify every candidate as one of:

- `adopt`: stable and repeated enough for the first spacing contract.
- `primitive-only`: useful base value, not a product relationship.
- `component-local`: belongs inside one component's geometry.
- `exception`: deliberate one-off or rare page archetype.
- `defer`: promising but not enough evidence yet.
- `remove`: historical spacing debt to migrate away from.

Do not over-adopt. A narrow first contract is better than a large token soup.

### 6. Propose Roles And Primitives Together

A useful spacing proposal normally has both:

- candidate roles, for the design language;
- primitives, for everyday implementation.

Example:

```txt
Role: settings.rowPaddingY
Current value: py-3
Owner: SettingsRow
Use when: vertical padding for rows inside SettingsSection cards
Do not use for: chat turns, tables, dashboard metric cards
Decision: adopt
```

Example:

```txt
Primitive: StatusBanner
Owns: banner.paddingX, banner.paddingY, banner.contentGap
Use when: issue, warning, pending setup, or lifecycle state needs a full-width notice
Decision: extract after auditing 3+ current banners
```

### 7. Avoid These Failure Modes

- Do not make tokens for every pixel value.
- Do not merge unrelated relationships because they share the same number.
- Do not force chat, onboarding, launcher, tables, or sparse About surfaces into settings spacing.
- Do not use grep counts as the final argument. Use them only to find evidence.
- Do not create roles without owners.
- Do not let component-wall examples teach spacing by accidental classes.
- Do not migrate before the role matrix is reviewed.

## Outputs

For analysis tasks, produce:

1. External-practice summary if this effort has not already been calibrated.
2. Slice ledgers for each audited interface.
3. A candidate role matrix.
4. Relationship confidence and value confidence.
5. Adopt/defer/local/exception decisions.
6. Suggested primitives and migration order.

For implementation tasks, also update the relevant docs or component wall and preserve existing user-visible behavior.

## References

- `references/best-practices.md`: outside design-system calibration.
- `references/cartography.md`: interface-slice and subagent mapping workflow.
- `references/role-taxonomy.md`: Memoh candidate roles and naming guidance.
