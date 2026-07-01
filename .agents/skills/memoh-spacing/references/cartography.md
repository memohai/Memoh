# Spacing Cartography Workflow

Use this workflow when mapping Memoh spacing from screenshots, code, or both.

## Goal

Produce a map of spatial relationships, not a grep report. The map should make it possible to decide which relationships deserve roles, which deserve primitives, and which should remain local.

Do not assume a current page is a gold reference. In early Memoh spacing work, relationships can be more reliable than values. Track relationship confidence and value confidence separately.

## Slice Selection

Prefer high-frequency interface slices:

- Bot General / Overview or a bot settings tab.
- Create/edit dialog or form.
- Provider/backend list plus empty/add state.
- Chat message stream plus tool detail.
- Onboarding, bot launcher, or About as non-settings exceptions.
- Component wall sections that teach or fail to teach spacing.

Choose 3 to 5 slices for a first pass. A first contract should be narrow.

## Per-Slice Procedure

1. **Describe the archetype**
   - settings page, form dialog, backend list, chat stream, launcher, onboarding, admin table, sparse page, etc.

2. **Annotate visible relationships**
   - screenshot-led: mark relationships directly on the image;
   - code-led: reconstruct the relationships from DOM/component structure.

3. **Map to code**
   - record the file path and line where the relationship is implemented;
   - record the current class, component prop, CSS variable, or style.

4. **Infer intent**
   - grouping, hierarchy, density, reading rhythm, action grouping, touch target, card continuity, empty-state frame, etc.

5. **Propose candidate role**
   - use a temporary name if unsure;
   - name the relationship, not the number.

6. **Assign owner**
   - existing primitive, new primitive, component-local geometry, exception, or primitive scale.

7. **Score confidence**
   - relationship confidence: high, medium, or low;
   - value confidence: high, medium, or low.

8. **Decide state**
   - `adopt`, `primitive-only`, `component-local`, `exception`, `defer`, or `remove`.

## Slice Ledger Template

Use this exact shape for subagent outputs when possible:

```md
## Slice: <name>

- Archetype:
- Files read:
- Screenshot/source:
- Current owner primitives:

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| page title to first content | `mb-6` in `PageShell` | separate title from body | `page.headerToBody` | `PageShell` | high | medium | adopt relationship | Tune value later |

### Patterns To Extract

| Primitive | Owns roles | Current examples | Why extract | Priority |
|---|---|---|---|---|
| `StatusBanner` | `banner.paddingX`, `banner.paddingY`, `banner.contentGap` | issue banners in bot overview/container/chat | repeated state notice language | medium |

### Exceptions

| Exception | Current implementation | Reason | Revisit trigger |
|---|---|---|---|
| About upper-middle bias | `pb-[12vh]` / translate footer | sparse page composition | if another sparse page appears |

### Open Questions

- ...
```

## Candidate Role Matrix

After slices are complete, combine them:

```md
| Candidate role | Settings | Dialog | Backend list | Chat | Launcher | Relationship confidence | Value confidence | Decision | Owner |
|---|---:|---:|---:|---:|---:|---|---|---|---|
| `page.gutterX` | yes | no | yes | no | no | high | medium | adopt relationship | `PageShell` |
| `form.fieldGap` | no | yes | no | no | no | high | medium | adopt relationship | `FormStack` |
| `chat.turnGap` | no | no | no | yes | no | high | low | adopt relationship, tune later | chat primitive |
| `launcher.tileGap` | no | no | no | no | yes | medium | low | defer | launcher page |
```

Look for roles that explain product relationships across slices. Do not merge unrelated relationships because the numeric value matches.

## Delegating To Subagents

Use subagents when there are multiple independent slices. Give every subagent the same schema and a bounded scope. Do not ask different subagents to invent their own taxonomy.

Good prompt:

```txt
Use the Memoh spacing cartography workflow. Analyze only these files: <list>. Do not edit files. Output the slice ledger with relationships, candidate roles, owners, decisions, and open questions. Focus on spatial relationships, not grep counts.
```

Bad prompt:

```txt
Find all spacing problems.
```

## Leader Synthesis

The leader should:

1. Normalize candidate names.
2. Merge equivalent relationships only when intent and owner match.
3. Split same-valued relationships when archetype differs.
4. Mark each candidate as adopt/local/exception/defer/remove.
5. Separate relationship confidence from value confidence.
6. Propose the smallest first contract.
7. Identify which component-wall examples must be added or changed.
8. Propose migration order.

## Evidence Standard

Use grep and statistics only as discovery tools. A role decision needs qualitative evidence:

- repeated product relationship;
- clear owner;
- visible effect on hierarchy or density;
- future decision cost if not standardized.

Value evidence is stricter: do not claim a value is final just because it is common. A common value can be provisional if the current UI was never tuned as a gold reference.

## First-Pass Stop Condition

Stop first-pass discovery when new slices mostly fall into existing archetypes and no major new relationship families appear. This is semantic saturation. Do not wait for every file in the repo before drafting the first contract.
