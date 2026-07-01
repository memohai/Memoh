// Ordered manifest of wall sections. Drives BOTH the left-nav anchor list and
// the render order in index.vue, so the two can never drift. Adding a section
// = one entry here + the component import.

import { markRaw, type Component } from 'vue'
import SectionTokens from '../sections/SectionTokens.vue'
import SectionType from '../sections/SectionType.vue'
import SectionAccents from '../sections/SectionAccents.vue'
import SectionSpacing from '../sections/SectionSpacing.vue'
import SectionAtoms from '../sections/SectionAtoms.vue'
import SectionInputsForms from '../sections/SectionInputsForms.vue'
import SectionOverlays from '../sections/SectionOverlays.vue'
import SectionNavigation from '../sections/SectionNavigation.vue'
import SectionDataDisplay from '../sections/SectionDataDisplay.vue'
import SectionFeedback from '../sections/SectionFeedback.vue'
import SectionLayout from '../sections/SectionLayout.vue'

export interface WallSection {
  id: string
  label: string
  component: Component
}

export const wallSections: WallSection[] = [
  { id: 'tokens', label: 'Design tokens', component: markRaw(SectionTokens) },
  { id: 'type', label: 'Typography', component: markRaw(SectionType) },
  { id: 'accents', label: 'Accent palette', component: markRaw(SectionAccents) },
  { id: 'spacing', label: 'Spacing', component: markRaw(SectionSpacing) },
  { id: 'atoms', label: 'Atoms', component: markRaw(SectionAtoms) },
  { id: 'inputs-forms', label: 'Inputs & Forms', component: markRaw(SectionInputsForms) },
  { id: 'overlays', label: 'Overlays', component: markRaw(SectionOverlays) },
  { id: 'navigation', label: 'Navigation', component: markRaw(SectionNavigation) },
  { id: 'data-display', label: 'Data Display', component: markRaw(SectionDataDisplay) },
  { id: 'feedback', label: 'Feedback', component: markRaw(SectionFeedback) },
  { id: 'layout', label: 'Layout', component: markRaw(SectionLayout) },
]
