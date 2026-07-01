# Dependency patches

This directory contains pnpm patches that are applied during install. Treat each
patch as a local upstream delta: keep it small, documented, and removable.

## dockview-core@7.0.2.patch

Why:

`dockview-core` builds the single-tab drag ghost by cloning the live docked tab
DOM, then hard-codes the ghost offset as `offsetX: 30`.

Both defaults fight Memoh's editor chrome:

- The cloned DOM includes strip-only layout pieces such as the close slot,
  active-tab shape, and lane padding. Once that structure is inside the floating
  drag chip, the title cannot be centered reliably with app CSS.
- The 30px horizontal offset places the chip partly to the left of the pointer.
  VS Code's single editor-tab drag image anchors the tab at the pointer's
  top-left (`setDragImage(tab, 0, 0)`), which is the behavior we want for Memoh
  tabs.

Scope:

Only the single-tab ghost path is patched:

- `createGhost()` keeps upstream `offsetY: -10` but changes `offsetX: 30` to
  `offsetX: 0`.
- `_buildGhostElement()` now creates a minimal title-only ghost element with a
  `.dv-tab-ghost-label` child instead of `cloneNode(true)`.

The same single-tab change is applied to every non-minified runtime entry that a
bundler may resolve:

- `dist/package/main.esm.mjs`
- `dist/package/main.cjs.js`
- `dist/esm/dockview/components/tab/tab.js`
- `dist/cjs/dockview/components/tab/tab.js`

The multi-panel/group drag ghosts keep upstream's `offsetX: 30` and default
renderers.

A second, independent change moves BOTH the "+" (left-actions) container AND the
void (header empty-space drop region) INTO `.dv-tabs-container` at construction
time — `this.tabs.element.appendChild(this.leftActionsContainer)` and
`this.tabs.element.appendChild(this.voidContainer.element)` instead of
`this._element.appendChild(...)`. The tab strip grows (`flex:1 1 auto`) to fill the
header; with the "+" and void inside it, the strip's children are `[+, tab1..tabN,
void]`. The "+" stays flush after the last tab, and the void (flex-grow:1) claims
all the strip's grown slack — so the whole blank stretch to the right of the last
tab is the droppable void (dockview paints its default drop wash over it), not a
dead non-droppable gap. Before the move that slack belonged to the tabs-container
itself and the void was pinned to a sliver at the far right, so dropping a tab on
the blank header area gave no feedback. The same move is applied to all four
entries above plus `dist/{esm,cjs}/dockview/components/titlebar/tabsContainer.js`.

The tab strip CSS in `apps/web/src/styles/dockview-theme.css` depends on this DOM
change: the "+" and void are the container's first DOM children (both appended
before any tab), ordered visually last via `order:1`/`order:2`; the "+"'s lead
divider (`::before`) sits on the seam at the last tab's right edge. The inter-tab
seam and first-tab hide rules there use adjacency selectors (not `:first-child`)
precisely because this patch makes the "+" the first DOM child.

Remove when:

`dockview-core` exposes single-tab drag ghost customization/offset options, or
upstream stops cloning the docked tab for the single-tab ghost and no longer
bleeds the chip left of the pointer; and dockview-core lets the "+" actions and
void ride with the tabs (or Memoh stops growing the tab strip with `flex:1 1
auto`), so the relocate patch is no longer needed to keep the "+" flush after the
last tab and the void spanning the blank header stretch.

Upgrade checklist:

1. Upgrade `dockview-vue` / `dockview` / `dockview-core`.
2. Check the installed `dist/package/main.esm.mjs` for `_buildGhostElement()` and
   all `offsetX` call sites.
3. Recreate the smallest version-specific patch with `pnpm patch dockview-core@x.y.z`.
4. Apply the same single-tab ghost change to the package ESM/CJS entries and the
   per-module ESM/CJS `dockview/components/tab/tab.js` entries. Vite can expose
   either path through dependency optimization.
5. Confirm the single-tab ghost no longer uses `cloneNode(true)` in all four
   patched entries.
6. Confirm only the single-tab offset changed; group/multi-panel offsets stay at
   `30`.
7. Clear Vite caches and drag one editor tab plus one multi-panel/group ghost in
   the app.

Useful checks:

```bash
grep -n "offsetX" patches/dockview-core@7.0.2.patch
grep -n "cloneNode\\|dv-tab-ghost-label" patches/dockview-core@7.0.2.patch
grep -n "leftActionsContainer" patches/dockview-core@7.0.2.patch
find node_modules/.pnpm -maxdepth 1 -type d -name 'dockview-core@7.0.2*'
rg -n "offsetX: 0|offsetX: 30|cloneNode|dv-tab-ghost-label|_buildGhostElement|buildMultiPanelsGhost" \
  node_modules/.pnpm/dockview-core@7.0.2*/node_modules/dockview-core/dist/package/main.esm.mjs \
  node_modules/.pnpm/dockview-core@7.0.2*/node_modules/dockview-core/dist/package/main.cjs.js \
  node_modules/.pnpm/dockview-core@7.0.2*/node_modules/dockview-core/dist/esm/dockview/components/tab/tab.js \
  node_modules/.pnpm/dockview-core@7.0.2*/node_modules/dockview-core/dist/cjs/dockview/components/tab/tab.js
```

## dockview-vue@7.0.2.patch

Why:

`dockview-vue@7.0.2` enriches header action params with `api`,
`containerApi`, `group`, `panels`, and `activePanel` when the header action
component mounts. On a group location change, its ESM runtime updates the Vue
component with only `{ location }`, replacing the complete params object. Memoh's
header action components need `group` and `panels`, so the app crashes after that
location-only update.

Scope:

Only the ESM runtime entry used by Vite (`dist/dockview-vue.es.js`) is patched.
`updateLocation(location)` now sends the complete enriched params plus the new
location instead of replacing params with `{ location }`.

Remove when:

`dockview-vue` upstream changes `updateLocation` to preserve the full header
action props, or Memoh no longer uses header action components that depend on
the enriched params.

Upgrade checklist:

1. Check the new `dockview-vue` runtime for `updateLocation(location)`.
2. If it still updates with `{ params: { location } }`, recreate this small patch.
3. Start the app and confirm `header-add-actions.vue` no longer crashes with
   `Cannot read properties of undefined (reading 'panels')`.

Useful checks:

```bash
grep -n "updateLocation" patches/dockview-vue@7.0.2.patch
rg -n "Memoh patch: keep the complete header-action params|params: \\{ location \\}" \
  node_modules/.pnpm/dockview-vue@7.0.2*/node_modules/dockview-vue/dist/dockview-vue.es.js
```
