<script setup lang="ts">
// Unified ACCENT palette — established NOW, reused everywhere later (colored
// menu items, tags, decorative icons, status, charts, capability badges, …).
//
// DESIGN MODEL: the point isn't raw vividness but SEMANTIC LAYERING — each hue
// ships a 4-step ramp and an interaction model:
//
//   accent     — saturated icon / text color (stays constant across states)
//   soft       — rest background tint        (lightest)
//   softHover  — hover background            (one step deeper)
//   softActive — selected/pressed background (deeper still) + border
//
// Rule the user surfaced earlier (2-layer vs 3-layer): for a COLORED item the
// TEXT/ICON color never changes on hover/select — only the background deepens
// across the soft → softHover → softActive ramp. That's the 3-layer model.
//
// Every hue is mapped into @theme inline (style.css), so this board
// renders through real bg-accent-{hue}[-variant] / text-accent-{hue} utility
// classes — no inline :style, no component-local <style> block. Tailwind's
// static scanner can't see template-interpolated class names
// (`bg-accent-${hue}` never appears as literal text anywhere), so
// HUE_CLASSES below spells out every class string in full — a
// Record<key, literal-classes> lookup, the same shape used elsewhere in the
// app for per-variant Tailwind class tables.
//
// The hover:/active: fills below trip the UI-contract "ad-hoc interaction
// fill" rule, which exists to stop a PAGE from duplicating chrome an owner
// component already encapsulates. Here there is no such owner: this board's
// menu-item/pill previews exist only to demonstrate the ramp itself (the
// hover/press states ARE the subject being shown), so each is marked
// ui-allow-style rather than invented as a throwaway component.
import {
  Bookmark, Check, Cloud, Flame, Hash, Heart, Leaf, Sparkles, Star, Tag,
} from 'lucide-vue-next'
import { ref } from 'vue'
import SectionShell from '../components/SectionShell.vue'

type HueName = 'gray' | 'brown' | 'orange' | 'yellow' | 'green' | 'teal' | 'blue' | 'purple' | 'pink' | 'red'

interface Hue {
  name: HueName
  icon: typeof Star
}

const hues: Hue[] = [
  { name: 'gray', icon: Hash },
  { name: 'brown', icon: Bookmark },
  { name: 'orange', icon: Flame },
  { name: 'yellow', icon: Star },
  { name: 'green', icon: Leaf },
  { name: 'teal', icon: Check },
  { name: 'blue', icon: Cloud },
  { name: 'purple', icon: Sparkles },
  { name: 'pink', icon: Tag },
  { name: 'red', icon: Heart },
]

interface HueClassSet {
  /** accent — saturated icon / text color, constant across states. */
  text: string
  /** accent — same hue as a background fill (ramp swatch + status dot). */
  bg: string
  /** soft — rest background tint. */
  soft: string
  /** soft-hover — static ramp swatch. */
  softHover: string
  /** soft-active — static ramp swatch. */
  softActive: string
  /** border — hairline color utility. */
  border: string
  /** Live hover → soft-hover (menu item rest state + pill). */
  hoverSoftHover: string
  /** Live press → soft-active (pill only; menu item uses click-to-select instead). */
  activeSoftActive: string
  /** Live selected → soft-active, no hover variant (menu item; selected ignores hover). */
  selectedSoftActive: string
}

const HUE_CLASSES: Record<HueName, HueClassSet> = {
  gray: { text: 'text-accent-gray', bg: 'bg-accent-gray', soft: 'bg-accent-gray-soft', softHover: 'bg-accent-gray-soft-hover', softActive: 'bg-accent-gray-soft-active', border: 'border-accent-gray-border', hoverSoftHover: 'hover:bg-accent-gray-soft-hover', activeSoftActive: 'active:bg-accent-gray-soft-active', selectedSoftActive: 'bg-accent-gray-soft-active' }, /* ui-allow-style */
  brown: { text: 'text-accent-brown', bg: 'bg-accent-brown', soft: 'bg-accent-brown-soft', softHover: 'bg-accent-brown-soft-hover', softActive: 'bg-accent-brown-soft-active', border: 'border-accent-brown-border', hoverSoftHover: 'hover:bg-accent-brown-soft-hover', activeSoftActive: 'active:bg-accent-brown-soft-active', selectedSoftActive: 'bg-accent-brown-soft-active' }, /* ui-allow-style */
  orange: { text: 'text-accent-orange', bg: 'bg-accent-orange', soft: 'bg-accent-orange-soft', softHover: 'bg-accent-orange-soft-hover', softActive: 'bg-accent-orange-soft-active', border: 'border-accent-orange-border', hoverSoftHover: 'hover:bg-accent-orange-soft-hover', activeSoftActive: 'active:bg-accent-orange-soft-active', selectedSoftActive: 'bg-accent-orange-soft-active' }, /* ui-allow-style */
  yellow: { text: 'text-accent-yellow', bg: 'bg-accent-yellow', soft: 'bg-accent-yellow-soft', softHover: 'bg-accent-yellow-soft-hover', softActive: 'bg-accent-yellow-soft-active', border: 'border-accent-yellow-border', hoverSoftHover: 'hover:bg-accent-yellow-soft-hover', activeSoftActive: 'active:bg-accent-yellow-soft-active', selectedSoftActive: 'bg-accent-yellow-soft-active' }, /* ui-allow-style */
  green: { text: 'text-accent-green', bg: 'bg-accent-green', soft: 'bg-accent-green-soft', softHover: 'bg-accent-green-soft-hover', softActive: 'bg-accent-green-soft-active', border: 'border-accent-green-border', hoverSoftHover: 'hover:bg-accent-green-soft-hover', activeSoftActive: 'active:bg-accent-green-soft-active', selectedSoftActive: 'bg-accent-green-soft-active' }, /* ui-allow-style */
  teal: { text: 'text-accent-teal', bg: 'bg-accent-teal', soft: 'bg-accent-teal-soft', softHover: 'bg-accent-teal-soft-hover', softActive: 'bg-accent-teal-soft-active', border: 'border-accent-teal-border', hoverSoftHover: 'hover:bg-accent-teal-soft-hover', activeSoftActive: 'active:bg-accent-teal-soft-active', selectedSoftActive: 'bg-accent-teal-soft-active' }, /* ui-allow-style */
  blue: { text: 'text-accent-blue', bg: 'bg-accent-blue', soft: 'bg-accent-blue-soft', softHover: 'bg-accent-blue-soft-hover', softActive: 'bg-accent-blue-soft-active', border: 'border-accent-blue-border', hoverSoftHover: 'hover:bg-accent-blue-soft-hover', activeSoftActive: 'active:bg-accent-blue-soft-active', selectedSoftActive: 'bg-accent-blue-soft-active' }, /* ui-allow-style */
  purple: { text: 'text-accent-purple', bg: 'bg-accent-purple', soft: 'bg-accent-purple-soft', softHover: 'bg-accent-purple-soft-hover', softActive: 'bg-accent-purple-soft-active', border: 'border-accent-purple-border', hoverSoftHover: 'hover:bg-accent-purple-soft-hover', activeSoftActive: 'active:bg-accent-purple-soft-active', selectedSoftActive: 'bg-accent-purple-soft-active' }, /* ui-allow-style */
  pink: { text: 'text-accent-pink', bg: 'bg-accent-pink', soft: 'bg-accent-pink-soft', softHover: 'bg-accent-pink-soft-hover', softActive: 'bg-accent-pink-soft-active', border: 'border-accent-pink-border', hoverSoftHover: 'hover:bg-accent-pink-soft-hover', activeSoftActive: 'active:bg-accent-pink-soft-active', selectedSoftActive: 'bg-accent-pink-soft-active' }, /* ui-allow-style */
  red: { text: 'text-accent-red', bg: 'bg-accent-red', soft: 'bg-accent-red-soft', softHover: 'bg-accent-red-soft-hover', softActive: 'bg-accent-red-soft-active', border: 'border-accent-red-border', hoverSoftHover: 'hover:bg-accent-red-soft-hover', activeSoftActive: 'active:bg-accent-red-soft-active', selectedSoftActive: 'bg-accent-red-soft-active' }, /* ui-allow-style */
}

// live selected state for the menu-list demo
const selectedHue = ref<HueName>('blue')

// D-section solid-fill button: same ui-allow-style rationale as HUE_CLASSES —
// the hover/press states are the ramp being demonstrated, not app chrome.
const blueFillButtonClass = 'inline-flex h-8 items-center gap-1.5 rounded-md bg-accent-blue-fill px-3 text-control font-medium text-accent-blue-foreground transition-colors hover:bg-accent-blue-fill-hover active:bg-accent-blue-fill-active' /* ui-allow-style */
</script>

<template>
  <SectionShell
    id="accents"
    label="Accent palette"
    description="统一强调色板，映射到我们的角色命名。核心是语义分层：每个色相 accent / soft / soft-hover / soft-active 四档；彩色项 hover、选中时文字色不变，只有背景在三档间加深（这就是三层色模型）。"
  >
    <!-- ───────────────────────── ramp board ───────────────────────── -->
    <div class="flex flex-col gap-2.5">
      <div class="grid grid-cols-[64px_repeat(4,1fr)_64px] gap-2 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
        <span>Hue</span>
        <span>accent</span>
        <span>soft</span>
        <span>soft-hover</span>
        <span>soft-active</span>
        <span>border</span>
      </div>

      <div
        v-for="h in hues"
        :key="h.name"
        class="grid grid-cols-[64px_repeat(4,1fr)_64px] items-center gap-2"
      >
        <span class="text-xs font-medium text-foreground">{{ h.name }}</span>
        <span :class="['h-8 rounded-md', HUE_CLASSES[h.name].bg]" />
        <span :class="['h-8 rounded-md border border-border-soft', HUE_CLASSES[h.name].soft]" />
        <span :class="['h-8 rounded-md border border-border-soft', HUE_CLASSES[h.name].softHover]" />
        <span :class="['h-8 rounded-md border border-border-soft', HUE_CLASSES[h.name].softActive]" />
        <span :class="['h-8 rounded-md border-2 bg-background', HUE_CLASSES[h.name].border]" />
      </div>
    </div>

    <!-- ───────────────────────── in-use demos ───────────────────────── -->
    <div class="mt-8 flex flex-col gap-7">
      <!-- A · colored menu list (the 3-layer model, live hover + selected) -->
      <div class="flex flex-col gap-2">
        <span class="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
          Colored menu items · rest → hover (live) → selected · text color constant
        </span>
        <div class="flex max-w-xs flex-col gap-0.5 rounded-xl border border-border p-1.5">
          <button
            v-for="h in hues"
            :key="h.name"
            :class="[
              'flex h-8 items-center gap-2 rounded-md px-2.5 text-label font-medium transition-colors',
              HUE_CLASSES[h.name].text,
              selectedHue === h.name ? HUE_CLASSES[h.name].selectedSoftActive : HUE_CLASSES[h.name].hoverSoftHover,
            ]"
            @click="selectedHue = h.name"
          >
            <component
              :is="h.icon"
              :size="16"
              :stroke-width="2.25"
            />
            <span class="capitalize">{{ h.name }}</span>
          </button>
        </div>
      </div>

      <!-- B · pill tags (soft fill + border + accent text), live hover -->
      <div class="flex flex-col gap-2">
        <span class="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Tags · soft fill + border + accent text · hover deepens</span>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="h in hues"
            :key="h.name"
            :class="[
              'inline-flex h-7 items-center gap-1.5 rounded-full border px-2.5 text-body font-medium transition-colors',
              HUE_CLASSES[h.name].text,
              HUE_CLASSES[h.name].soft,
              HUE_CLASSES[h.name].border,
              HUE_CLASSES[h.name].hoverSoftHover,
              HUE_CLASSES[h.name].activeSoftActive,
            ]"
          >
            <component
              :is="h.icon"
              :size="13"
              :stroke-width="2.25"
            />
            {{ h.name }}
          </button>
        </div>
      </div>

      <!-- C · accent icons / status dots -->
      <div class="flex flex-col gap-2">
        <span class="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Accent icons + status dots</span>
        <div class="flex flex-wrap items-center gap-4">
          <span
            v-for="h in hues"
            :key="h.name"
            class="inline-flex items-center gap-1.5 text-xs text-foreground"
          >
            <component
              :is="h.icon"
              :size="16"
              :stroke-width="2.25"
              :class="HUE_CLASSES[h.name].text"
            />
            <span :class="['size-2 rounded-full', HUE_CLASSES[h.name].bg]" />
          </span>
        </div>
      </div>

      <!-- D · blue solid fill button (the one saturated-fill hue) -->
      <div class="flex flex-col gap-2">
        <span class="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Solid fill · blue only (white text) · primary blue</span>
        <div class="flex flex-wrap items-center gap-3">
          <button :class="blueFillButtonClass">
            <Cloud
              :size="15"
              :stroke-width="2.25"
            />
            Primary blue
          </button>
          <code class="text-[10px] text-muted-foreground">rest var(--accent-blue-fill) · hover var(--accent-blue-fill-hover) · press var(--accent-blue-fill-active)</code>
        </div>
      </div>
    </div>
  </SectionShell>
</template>
