import type { Component } from 'vue'
import { setCustomComponents } from 'markstream-vue'
import MdCheckbox from './md-checkbox.vue'
import MdFootnoteReference from './md-footnote-reference.vue'
import MdFootnoteAnchor from './md-footnote-anchor.vue'

// Custom markstream node components shared by every markdown surface (chat +
// file preview). They replace markstream's built-in glyphs with design-system
// equivalents — the library Checkbox for task markers, and link-language
// footnote markers (dotted underline + up-right arrow) — without touching the
// renderer's id/scroll wiring.
const sharedComponents: Record<string, Component> = {
  checkbox: MdCheckbox,
  footnote_reference: MdFootnoteReference,
  footnote_anchor: MdFootnoteAnchor,
}

const registered = new Set<string>()

// Register the shared components (plus any surface-specific extras, e.g. the
// chat code block) in ONE call per `customId`, so the result is correct
// regardless of whether markstream merges or replaces a scope's mapping.
export function registerSharedMarkdownComponents(
  customId: string,
  extra?: Record<string, Component>,
): void {
  if (registered.has(customId)) return
  registered.add(customId)
  setCustomComponents(customId, { ...sharedComponents, ...extra })
}
