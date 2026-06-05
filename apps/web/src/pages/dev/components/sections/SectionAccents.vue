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
// `border` is the visible hairline; `deep` is the darkest text tone (fg on a
// solid fill, or heavy colored text). Promote to packages/ui/src/style.css as
// --accent-{hue}-{role} once locked.
import {
  Bookmark, Check, Cloud, Flame, Hash, Heart, Leaf, Sparkles, Star, Tag,
} from 'lucide-vue-next'
import { ref } from 'vue'
import SectionShell from '../components/SectionShell.vue'

interface Accent {
  name: string
  accent: string
  soft: string
  softHover: string
  softActive: string
  border: string
  deep: string
  icon: typeof Star
}

// Light-theme values, mapped to our role names.
//   accent     — saturated, readable on white
//   soft       — rest tint   softHover — hover   softActive — selected/pressed
//   border     — hairline    deep      — darkest text tone
const accents: Accent[] = [
  { name: 'gray', accent: '#5f5e59', soft: '#f9f8f7', softHover: '#f0efed', softActive: '#e6e5e3', border: '#d4d3cf', deep: '#494846', icon: Hash },
  { name: 'brown', accent: '#9f765a', soft: '#faf8f6', softHover: '#f5ede9', softActive: '#ebdfd7', border: '#e0cdc0', deep: '#584437', icon: Bookmark },
  { name: 'orange', accent: '#d9730d', soft: '#fcf7f4', softHover: '#fbebde', softActive: '#f3ddcb', border: '#eaccb2', deep: '#6a4222', icon: Flame },
  { name: 'yellow', accent: '#cb912f', soft: '#fcfaef', softHover: '#f9f3dc', softActive: '#f2e3b7', border: '#e8d497', deep: '#655121', icon: Star },
  { name: 'green', accent: '#448361', soft: '#f6f9f7', softHover: '#e8f1ec', softActive: '#d7e6dd', border: '#bed9c9', deep: '#2a533c', icon: Leaf },
  { name: 'teal', accent: '#2c8b9e', soft: '#f3fafb', softHover: '#e0f3f7', softActive: '#cae9f0', border: '#b0dbe4', deep: '#18505b', icon: Check },
  { name: 'blue', accent: '#2383e2', soft: '#f3f9fd', softHover: '#e5f2fc', softActive: '#cee3f7', border: '#b6d4f3', deep: '#264a72', icon: Cloud },
  { name: 'purple', accent: '#9065b0', soft: '#faf7fc', softHover: '#f3ebf9', softActive: '#e8dbf2', border: '#dbc8e8', deep: '#553b69', icon: Sparkles },
  { name: 'pink', accent: '#c14c8a', soft: '#fcf7f9', softHover: '#fae9f1', softActive: '#f4d8e4', border: '#eac4d5', deep: '#68354e', icon: Tag },
  { name: 'red', accent: '#cd3c3a', soft: '#fdf6f6', softHover: '#fce9e7', softActive: '#f7d9d5', border: '#f0c5be', deep: '#6d3531', icon: Heart },
]

// blue is the one hue we also ship as a saturated FILL (white text).
const blueFill = { rest: '#2383e2', hover: '#0077d4', press: '#006bc7' }

// live selected state for the menu-list demo
const selectedHue = ref('blue')
</script>

<template>
  <SectionShell
    id="accents"
    label="Accent palette"
    description="统一强调色板，映射到我们的角色命名。核心是语义分层：每个色相 accent / soft / soft-hover / soft-active 四档；彩色项 hover、选中时文字色不变，只有背景在三档间加深（这就是三层色模型）。锁定后提升到 style.css 成为 --accent-{hue}-{role}。"
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
        v-for="a in accents"
        :key="a.name"
        class="grid grid-cols-[64px_repeat(4,1fr)_64px] items-center gap-2"
      >
        <span class="text-xs font-medium text-foreground">{{ a.name }}</span>
        <span
          class="h-8 rounded-md"
          :style="{ backgroundColor: a.accent }"
        />
        <span
          class="h-8 rounded-md border border-border/60"
          :style="{ backgroundColor: a.soft }"
        />
        <span
          class="h-8 rounded-md border border-border/60"
          :style="{ backgroundColor: a.softHover }"
        />
        <span
          class="h-8 rounded-md border border-border/60"
          :style="{ backgroundColor: a.softActive }"
        />
        <span
          class="h-8 rounded-md border-2 bg-background"
          :style="{ borderColor: a.border }"
        />
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
            v-for="a in accents"
            :key="a.name"
            class="acc-item"
            :class="{ 'is-selected': selectedHue === a.name }"
            :style="{
              '--rest': selectedHue === a.name ? a.softActive : 'transparent',
              '--hover': a.softHover,
              '--active': a.softActive,
              '--txt': a.accent,
              '--bd': a.border,
            }"
            @click="selectedHue = a.name"
          >
            <component
              :is="a.icon"
              :size="16"
              :stroke-width="2.25"
            />
            <span class="capitalize">{{ a.name }}</span>
          </button>
        </div>
      </div>

      <!-- B · pill tags (soft fill + border + accent text), live hover -->
      <div class="flex flex-col gap-2">
        <span class="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Tags · soft fill + border + accent text · hover deepens</span>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="a in accents"
            :key="a.name"
            class="acc-pill"
            :style="{
              '--rest': a.soft,
              '--hover': a.softHover,
              '--active': a.softActive,
              '--txt': a.accent,
              '--bd': a.border,
            }"
          >
            <component
              :is="a.icon"
              :size="13"
              :stroke-width="2.25"
            />
            {{ a.name }}
          </button>
        </div>
      </div>

      <!-- C · accent icons / status dots -->
      <div class="flex flex-col gap-2">
        <span class="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Accent icons + status dots</span>
        <div class="flex flex-wrap items-center gap-4">
          <span
            v-for="a in accents"
            :key="a.name"
            class="inline-flex items-center gap-1.5 text-xs text-foreground"
          >
            <component
              :is="a.icon"
              :size="16"
              :stroke-width="2.25"
              :style="{ color: a.accent }"
            />
            <span
              class="size-2 rounded-full"
              :style="{ backgroundColor: a.accent }"
            />
          </span>
        </div>
      </div>

      <!-- D · blue solid fill button (the one saturated-fill hue) -->
      <div class="flex flex-col gap-2">
        <span class="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Solid fill · blue only (white text) · primary blue</span>
        <div class="flex flex-wrap items-center gap-3">
          <button
            class="acc-fill"
            :style="{ '--rest': blueFill.rest, '--hover': blueFill.hover, '--active': blueFill.press }"
          >
            <Cloud
              :size="15"
              :stroke-width="2.25"
            />
            Primary blue
          </button>
          <code class="text-[10px] text-muted-foreground">rest {{ blueFill.rest }} · hover {{ blueFill.hover }} · press {{ blueFill.press }}</code>
        </div>
      </div>
    </div>
  </SectionShell>
</template>

<style scoped>
/* colored menu item — text/icon color constant, bg deepens across the ramp */
.acc-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  height: 2rem;
  padding: 0 0.625rem;
  border-radius: 0.5rem;
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--txt);
  background-color: var(--rest);
  transition: background-color 0.1s ease;
}
.acc-item:hover {
  background-color: var(--hover);
}
.acc-item.is-selected {
  background-color: var(--active);
}

.acc-pill {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  height: 1.75rem;
  padding: 0 0.625rem;
  border-radius: 9999px;
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--txt);
  background-color: var(--rest);
  border: 1px solid var(--bd);
  transition: background-color 0.1s ease;
}
.acc-pill:hover {
  background-color: var(--hover);
}
.acc-pill:active {
  background-color: var(--active);
}

.acc-fill {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  height: 2rem;
  padding: 0 0.75rem;
  border-radius: 0.5rem;
  font-size: 0.875rem;
  font-weight: 500;
  color: #fff;
  background-color: var(--rest);
  transition: background-color 0.1s ease;
}
.acc-fill:hover {
  background-color: var(--hover);
}
.acc-fill:active {
  background-color: var(--active);
}
</style>
