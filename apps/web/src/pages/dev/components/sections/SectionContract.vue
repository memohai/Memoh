<script setup lang="ts">
// Button weight CONTRACT bench — ONE weight-rule set, tuned here before touching the
// real buttonVariants / style.css. Real @memohai/ui is NOT edited yet; these are
// local-class previews on the locked neutral base.
//
// ARCHITECTURE: bg/ring/shadow live on ::before (not the button element) so that
// press scale only shrinks the background shell — text never moves or blurs.
// Tailwind v4: `scale` is a CSS property (not `transform`), inline transitions
// use `scale` accordingly. Icon SVGs at 18px for clean 2x Retina rendering
// (stroke 2 in 24×24 viewBox → 1.5px CSS → 3 device px on 2x).
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import {
  ArrowRight, Bold, ChevronDown, Copy, ExternalLink, Heart, Italic,
  Loader2, Plus, RefreshCw, Settings, Star, Strikethrough, Trash2, Underline,
} from 'lucide-vue-next'
import SectionShell from '../components/SectionShell.vue'
import SceneFrame from '../components/SceneFrame.vue'
import { palettes } from '../lib/palettes'

type Tone = 'neutral' | 'brand'
type Shape = 'rect' | 'pill'

const base = computed(() => palettes.find((p) => p.id === 'base') ?? palettes[0])
const dark = ref(false)
const tone = ref<Tone>('neutral')
const shape = ref<Shape>('rect')

const RECT_RADIUS = 8
const isPill = computed(() => shape.value === 'pill')
const btnRadius = computed(() => isPill.value ? '9999px' : RECT_RADIUS + 'px')
const btnStyle = computed(() => `border-radius:${btnRadius.value}`)
const secondaryStyle = computed(() => btnStyle.value)

// ── Variant classes ──────────────────────────────────────────────────────
const primaryClass = computed(() =>
  'btn-primary'
  + (tone.value === 'brand' ? ' btn-primary--brand' : ''),
)
const primaryStyle = computed(() =>
  btnStyle.value
  + (isPill.value
    ? ';--hover-sx:1.005;--hover-sy:1.015'
    : ';--hover-sx:1.008;--hover-sy:1.02'),
)
const SECONDARY = 'btn-secondary bg-transparent text-[var(--fg)]'
const GHOST = 'btn-ghost rounded-md text-[var(--fg)]'
const DESTRUCTIVE = 'text-[var(--fg)] transition-colors duration-150 hover:bg-[#FAE2E1] hover:text-[#c0271e]'
const DESTRUCTIVE_SOLID = 'btn-destructive-solid text-white'
// Three link variants:
//  - LINK        → DEFAULT. Underline FADES in on hover (opacity), text
//                  brightens muted→fg. Calm, works everywhere incl. dense UI.
//  - LINK_STATIC → underline ALWAYS visible (长显); only color brightens on
//                  hover. Most "linky" / discoverable — good for body text.
//  - LINK_DRAW   → "landing / accent" copy. Underline DRAWS in from the left on
//                  hover. Eye-catching, rare — reserve for hero CTAs & marketing.
const LINK = 'link-fade inline-flex items-center gap-1 text-sm font-medium text-[var(--muted-fg)] hover:text-[var(--fg)] transition-colors duration-150 [&_svg]:size-4'
const LINK_STATIC = 'inline-flex items-center gap-1 text-sm font-medium text-[var(--muted-fg)] underline underline-offset-4 decoration-[var(--muted-fg)] hover:text-[var(--fg)] hover:decoration-[var(--fg)] transition-colors duration-150 [&_svg]:size-4'
const LINK_DRAW = 'link-draw inline-flex items-center gap-1 text-sm font-medium text-[var(--muted-fg)] hover:text-[var(--fg)] transition-colors duration-150 [&_svg]:size-4'

const EDIT = 'btn-edit inline-flex items-center justify-center gap-1.5 text-[var(--fg)] cursor-default disabled:pointer-events-none disabled:opacity-40'
const editStyle = 'border-radius:8px'

// ── Size atoms (sm=h-8, default=h-9, lg=h-10) ───────────────────────────
const BTN_SM = 'inline-flex h-8 items-center justify-center gap-1.5 whitespace-nowrap px-3 text-sm font-medium disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-4'
const BTN = 'inline-flex h-9 items-center justify-center gap-2 whitespace-nowrap px-4 text-sm font-medium disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-4'
const BTN_LG = 'inline-flex h-10 items-center justify-center gap-2 whitespace-nowrap px-6 text-sm font-medium disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-4'

// ── Icon button sizes (icon-sm=32, icon=36, icon-lg=40) ─────────────────
const ICON_SM = 'inline-flex size-8 items-center justify-center disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-4'
const ICON = 'inline-flex size-9 items-center justify-center disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-4'
const ICON_LG = 'inline-flex size-10 items-center justify-center disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-4'

// ── Toggle state ─────────────────────────────────────────────────────────
const boldOn = ref(false)
const italicOn = ref(false)
const underlineOn = ref(false)
const strikeOn = ref(false)
const favOn = ref(false)

// color-only toggle variant: same toolbar, but active TINTS the icon (blue)
// instead of painting a persistent rounded-rect background.
const tintBold = ref(true)
const tintItalic = ref(false)
const tintUnderline = ref(false)
const tintStrike = ref(false)

// ── Segmented control (real single-select + sliding indicator) ─────────────
const segments = [
  { id: 'day', label: 'Day' },
  { id: 'week', label: 'Week' },
  { id: 'month', label: 'Month' },
] as const
const activeSeg = ref<typeof segments[number]['id']>('week')
const segGroup = ref<HTMLElement>()
const segIndicator = ref<{ left: number, top: number, width: number, height: number }>({ left: 0, top: 0, width: 0, height: 0 })
function syncIndicator() {
  const el = segGroup.value?.querySelector<HTMLElement>('[data-seg-active="true"]')
  if (!el)
    return
  segIndicator.value = { left: el.offsetLeft, top: el.offsetTop, width: el.offsetWidth, height: el.offsetHeight }
}
watch(activeSeg, () => nextTick(syncIndicator))
onMounted(() => nextTick(syncIndicator))

// ── Non-linear press amplitude (SHELVED, revive as a `v-press-scale` directive
//    if we want width-adaptive scaling across many widths). The full-width
//    Continue opts OUT of press-scale entirely (btn-cta = color-press), so the
//    curve has no consumer right now. Formula for when we bring it back:
//      fraction s(W) = base · (NORM_W / W)^k   (measure offsetWidth per element)
//      k = 0 → constant fraction (wide = too heavy) · k = 1 → constant px
//      (wide = no feel) · k ≈ 0.6 in between. base ≈ 0.022 after the −30% trim.
// press-shrink for the white thumb only fires when the ALREADY-selected
// segment is pressed (non-selected items shrink via their own ::before:active).
const segPressed = ref(false)
function onSegDown(id: typeof segments[number]['id']) {
  segPressed.value = id === activeSeg.value
}

// ── Loading state — click a button to enter a ~1.6s busy state. The point of
// the demo is ZERO layout shift while it spins (see markup for the two no-shift
// techniques: hidden-label overlay, and leading-icon swap).
const loading = ref<Record<string, boolean>>({})
function runLoad(key: string) {
  if (loading.value[key])
    return
  loading.value[key] = true
  setTimeout(() => { loading.value[key] = false }, 1600)
}

const toneOpts: { id: Tone; label: string }[] = [
  { id: 'neutral', label: 'Neutral' },
  { id: 'brand', label: 'Brand' },
]
const shapeOpts: { id: Shape; label: string }[] = [
  { id: 'rect', label: '8px rect' },
  { id: 'pill', label: 'Pill' },
]
</script>

<template>
  <SectionShell
    id="controls-contract"
    label="Controls contract — buttons"
    description="按钮必须来自同一套权重规则，先在这里调，再进入真实页面。真组件未改，这里是锁定 base 上的本地预览。圆角平滑由 lisse(corne.rs) 提供。"
  >
    <!-- toolbar -->
    <div class="mb-4 flex flex-wrap items-center gap-2 text-xs">
      <div class="flex items-center gap-1 rounded-lg border border-border p-0.5">
        <button
          type="button"
          class="rounded-md px-2.5 py-1 transition-colors"
          :class="!dark ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'"
          @click="dark = false"
        >
          Light
        </button>
        <button
          type="button"
          class="rounded-md px-2.5 py-1 transition-colors"
          :class="dark ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'"
          @click="dark = true"
        >
          Dark
        </button>
      </div>

      <div class="flex items-center gap-1.5">
        <span class="text-muted-foreground">Primary tone</span>
        <div class="flex items-center gap-0.5 rounded-lg border border-border p-0.5">
          <button
            v-for="t in toneOpts"
            :key="t.id"
            type="button"
            class="rounded-md px-2 py-1 transition-colors"
            :class="tone === t.id ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'"
            @click="tone = t.id"
          >
            {{ t.label }}
          </button>
        </div>
      </div>

      <div class="flex items-center gap-1.5">
        <span class="text-muted-foreground">Shape</span>
        <div class="flex items-center gap-0.5 rounded-lg border border-border p-0.5">
          <button
            v-for="s in shapeOpts"
            :key="s.id"
            type="button"
            class="rounded-md px-2 py-1 transition-colors"
            :class="shape === s.id ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'"
            @click="shape = s.id"
          >
            {{ s.label }}
          </button>
        </div>
      </div>
    </div>

    <SceneFrame
      :palette="base"
      :dark="dark"
    >
      <div class="contract-scene flex flex-col gap-7 p-6 text-[var(--fg)]">
        <!-- ═══ 1. WEIGHT VARIANTS ═══════════════════════════════════════ -->
        <div class="flex flex-col gap-4">
          <span class="text-[11px] font-medium uppercase tracking-wide text-[var(--muted-fg)]">Weight variants</span>
          <div class="flex flex-wrap items-center gap-3">
            <button
              type="button"
              :class="[BTN, primaryClass]"
              :style="primaryStyle"
            >
              Save
            </button>
            <button
              type="button"
              :class="[BTN, SECONDARY]"
              :style="secondaryStyle"
            >
              <Plus />Add provider
            </button>
            <button
              type="button"
              :class="[BTN, GHOST]"
              :style="btnStyle"
            >
              <RefreshCw />Refresh
            </button>
            <button
              type="button"
              :class="[BTN, DESTRUCTIVE]"
              :style="btnStyle"
            >
              <Trash2 />Delete
            </button>
            <button
              type="button"
              :class="LINK"
            >
              Learn more
            </button>
            <button
              type="button"
              :class="[BTN, SECONDARY]"
              :style="secondaryStyle"
              disabled
            >
              Disabled
            </button>
          </div>
        </div>

        <!-- ═══ 2. SIZE VARIANTS (sm / default / lg) ═════════════════════ -->
        <div class="flex flex-col gap-4 border-t border-[var(--border-role)] pt-6">
          <span class="text-[11px] font-medium uppercase tracking-wide text-[var(--muted-fg)]">Size variants · sm (h-8) / default (h-9) / lg (h-10)</span>
          <div class="flex flex-wrap items-end gap-3">
            <button
              type="button"
              :class="[BTN_SM, primaryClass]"
              :style="primaryStyle"
            >
              Small
            </button>
            <button
              type="button"
              :class="[BTN, primaryClass]"
              :style="primaryStyle"
            >
              Default
            </button>
            <button
              type="button"
              :class="[BTN_LG, primaryClass]"
              :style="primaryStyle"
            >
              Large
            </button>
          </div>
          <div class="flex flex-wrap items-end gap-3">
            <button
              type="button"
              :class="[BTN_SM, SECONDARY]"
              :style="secondaryStyle"
            >
              <Plus />Small
            </button>
            <button
              type="button"
              :class="[BTN, SECONDARY]"
              :style="secondaryStyle"
            >
              <Plus />Default
            </button>
            <button
              type="button"
              :class="[BTN_LG, SECONDARY]"
              :style="secondaryStyle"
            >
              <Plus />Large
            </button>
          </div>
        </div>

        <!-- ═══ 3. ICON BUTTONS — ghost default ═══════════════════════════ -->
        <div class="flex flex-col gap-4 border-t border-[var(--border-role)] pt-6">
          <span class="text-[11px] font-medium uppercase tracking-wide text-[var(--muted-fg)]">Icon buttons · ghost = toolbar default</span>
          <div class="flex flex-wrap items-end gap-6">
            <!-- ghost: the standard toolbar choice -->
            <div class="flex flex-col gap-2">
              <span class="text-[10px] text-[var(--muted-fg)]">Ghost (toolbar)</span>
              <div class="flex items-end gap-1.5">
                <button
                  type="button"
                  :class="[ICON_SM, GHOST]"
                  :style="btnStyle"
                >
                  <Settings />
                </button>
                <button
                  type="button"
                  :class="[ICON, GHOST]"
                  :style="btnStyle"
                >
                  <RefreshCw />
                </button>
                <button
                  type="button"
                  :class="[ICON_LG, GHOST]"
                  :style="btnStyle"
                >
                  <Copy />
                </button>
              </div>
            </div>
            <!-- secondary: standalone actions that need affordance -->
            <div class="flex flex-col gap-2">
              <span class="text-[10px] text-[var(--muted-fg)]">Secondary (standalone)</span>
              <div class="flex items-end gap-1.5">
                <button
                  type="button"
                  :class="[ICON_SM, SECONDARY]"
                  :style="secondaryStyle"
                >
                  <Settings />
                </button>
                <button
                  type="button"
                  :class="[ICON, SECONDARY]"
                  :style="secondaryStyle"
                >
                  <RefreshCw />
                </button>
                <button
                  type="button"
                  :class="[ICON_LG, SECONDARY]"
                  :style="secondaryStyle"
                >
                  <Copy />
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- ═══ 4. ICON POSITION (left / right) ══════════════════════════ -->
        <div class="flex flex-col gap-4 border-t border-[var(--border-role)] pt-6">
          <span class="text-[11px] font-medium uppercase tracking-wide text-[var(--muted-fg)]">Icon position</span>
          <div class="flex flex-wrap items-center gap-3">
            <button
              type="button"
              :class="[BTN, SECONDARY]"
              :style="secondaryStyle"
            >
              <Plus />Add item
            </button>
            <button
              type="button"
              :class="[BTN, SECONDARY]"
              :style="secondaryStyle"
            >
              Next <ArrowRight />
            </button>
            <button
              type="button"
              :class="[BTN, SECONDARY]"
              :style="secondaryStyle"
            >
              Options <ChevronDown />
            </button>
            <button
              type="button"
              :class="LINK_STATIC"
            >
              Read guide
            </button>
            <button
              type="button"
              :class="LINK_DRAW"
            >
              View docs <ExternalLink />
            </button>
          </div>
        </div>

        <!-- ═══ 5. LOADING STATE ═════════════════════════════════════════ -->
        <div class="flex flex-col gap-4 border-t border-[var(--border-role)] pt-6">
          <span class="text-[11px] font-medium uppercase tracking-wide text-[var(--muted-fg)]">Loading state · click to load — watch: NO width jump, color stays (not faded)</span>
          <div class="flex flex-wrap items-center gap-3">
            <!-- (a) TEXT button · no-shift = label stays in flow but hidden, spinner
                 overlays absolutely centered → button keeps its exact width. -->
            <button
              type="button"
              :class="[BTN, primaryClass, 'btn-busy relative', { 'is-loading': loading.save }]"
              :style="primaryStyle"
              :disabled="loading.save"
              :aria-busy="loading.save"
              @click="runLoad('save')"
            >
              <span :class="loading.save ? 'invisible' : ''">Save changes</span>
              <span
                v-if="loading.save"
                class="absolute inset-0 grid place-items-center"
              >
                <Loader2 class="animate-spin" />
              </span>
            </button>

            <!-- (b) ICON-LEADING button · the button's OWN icon spins (no glyph
                 swap → fully continuous: it just starts rotating). Width never
                 moves (same icon box). Loader2 is only for buttons with no icon. -->
            <button
              type="button"
              :class="[BTN, SECONDARY, 'btn-busy', { 'is-loading': loading.sync }]"
              :style="secondaryStyle"
              :disabled="loading.sync"
              :aria-busy="loading.sync"
              @click="runLoad('sync')"
            >
              <RefreshCw :class="loading.sync ? 'animate-spin' : ''" />
              Sync
            </button>

            <!-- (c) ICON-ONLY · the glyph itself spins (no swap) in the size-9 box. -->
            <button
              type="button"
              :class="[ICON, GHOST, 'btn-busy', { 'is-loading': loading.refresh }]"
              :style="btnStyle"
              :disabled="loading.refresh"
              :aria-busy="loading.refresh"
              aria-label="Refresh"
              @click="runLoad('refresh')"
            >
              <RefreshCw :class="loading.refresh ? 'animate-spin' : ''" />
            </button>
          </div>

          <!-- (d) FULL-WIDTH CTA · LEADING spinner that ANIMATES in/out — the slot
               grows 0→1.5rem (pushing the label right via justify-center) while the
               spinner fades + slides in from the left. Spinner stays in the DOM
               (no v-if) so it can transition; is-loading drives it. -->
          <div class="w-72">
            <button
              type="button"
              :class="[BTN, primaryClass, 'btn-cta btn-busy btn-spin w-full', { 'is-loading': loading.continue }]"
              :style="primaryStyle"
              :disabled="loading.continue"
              :aria-busy="loading.continue"
              @click="runLoad('continue')"
            >
              <span class="spin-slot"><Loader2 class="size-4 animate-spin" /></span>
              <span>Continue</span>
            </button>
          </div>
        </div>

        <!-- ═══ 5b. DISABLED STATE — inert: faded, no hover/press, not clickable ═ -->
        <div class="flex flex-col gap-4 border-t border-[var(--border-role)] pt-6">
          <span class="text-[11px] font-medium uppercase tracking-wide text-[var(--muted-fg)]">Disabled state · 40% + no hover/press (try clicking — nothing happens)</span>
          <div class="flex flex-wrap items-center gap-3">
            <button
              type="button"
              :class="[BTN, primaryClass]"
              :style="primaryStyle"
              disabled
            >
              Primary
            </button>
            <button
              type="button"
              :class="[BTN, SECONDARY]"
              :style="secondaryStyle"
              disabled
            >
              Secondary
            </button>
            <button
              type="button"
              :class="[BTN, GHOST]"
              :style="btnStyle"
              disabled
            >
              <RefreshCw />Ghost
            </button>
            <button
              type="button"
              :class="[ICON, GHOST]"
              :style="btnStyle"
              disabled
              aria-label="Settings"
            >
              <Settings />
            </button>
            <div class="w-56">
              <button
                type="button"
                :class="[BTN, primaryClass, 'btn-cta w-full']"
                :style="primaryStyle"
                disabled
              >
                Continue
              </button>
            </div>
          </div>
        </div>

        <!-- ═══ 6. TOGGLE BUTTONS ════════════════════════════════════════ -->
        <div class="flex flex-col gap-4 border-t border-[var(--border-role)] pt-6">
          <span class="text-[11px] font-medium uppercase tracking-wide text-[var(--muted-fg)]">Toggle buttons</span>
          <div class="flex flex-wrap items-center gap-4">
            <!-- formatting toolbar group -->
            <div class="flex items-center gap-0.5 rounded-lg border border-[var(--border-role)] p-0.5">
              <button
                type="button"
                :class="[ICON_SM, 'btn-toggle rounded-md text-[var(--fg)]', { 'is-active': boldOn }]"
                @click="boldOn = !boldOn"
              >
                <Bold />
              </button>
              <button
                type="button"
                :class="[ICON_SM, 'btn-toggle rounded-md text-[var(--fg)]', { 'is-active': italicOn }]"
                @click="italicOn = !italicOn"
              >
                <Italic />
              </button>
              <button
                type="button"
                :class="[ICON_SM, 'btn-toggle rounded-md text-[var(--fg)]', { 'is-active': underlineOn }]"
                @click="underlineOn = !underlineOn"
              >
                <Underline />
              </button>
              <button
                type="button"
                :class="[ICON_SM, 'btn-toggle rounded-md text-[var(--fg)]', { 'is-active': strikeOn }]"
                @click="strikeOn = !strikeOn"
              >
                <Strikethrough />
              </button>
            </div>
            <!-- standalone toggle -->
            <button
              type="button"
              :class="[ICON, 'btn-toggle', { 'is-active': favOn }]"
              :style="{ borderRadius: btnStyle.includes('9999') ? '9999px' : RECT_RADIUS + 'px', color: favOn ? 'var(--ac-red)' : 'var(--fg)' }"
              @click="favOn = !favOn"
            >
              <Heart :fill="favOn ? 'currentColor' : 'none'" />
            </button>
            <button
              type="button"
              :class="[BTN, 'btn-toggle', { 'is-active': favOn }]"
              :style="{ ...Object.fromEntries(btnStyle.split(';').map(s => { const [k,v] = s.split(':'); return [k.trim(), v?.trim()] })), color: favOn ? 'var(--ac-amber)' : 'var(--fg)' }"
              @click="favOn = !favOn"
            >
              <Star :fill="favOn ? 'currentColor' : 'none'" />{{ favOn ? 'Starred' : 'Star' }}
            </button>
          </div>

          <!-- color-only variant: same toolbar, active TINTS the icon (no fill) -->
          <div class="flex flex-wrap items-center gap-4">
            <span class="text-[10px] text-[var(--muted-fg)]">Color-only variant · active tints the icon</span>
            <div class="flex items-center gap-0.5 rounded-lg border border-[var(--border-role)] p-0.5">
              <button
                type="button"
                :class="[ICON_SM, 'btn-tint rounded-md', { 'is-active': tintBold }]"
                @click="tintBold = !tintBold"
              >
                <Bold />
              </button>
              <button
                type="button"
                :class="[ICON_SM, 'btn-tint rounded-md', { 'is-active': tintItalic }]"
                @click="tintItalic = !tintItalic"
              >
                <Italic />
              </button>
              <button
                type="button"
                :class="[ICON_SM, 'btn-tint rounded-md', { 'is-active': tintUnderline }]"
                @click="tintUnderline = !tintUnderline"
              >
                <Underline />
              </button>
              <button
                type="button"
                :class="[ICON_SM, 'btn-tint rounded-md', { 'is-active': tintStrike }]"
                @click="tintStrike = !tintStrike"
              >
                <Strikethrough />
              </button>
            </div>
          </div>
        </div>

        <!-- ═══ 7. SEGMENTED CONTROL (real select + sliding indicator) ════ -->
        <div class="flex flex-col gap-4 border-t border-[var(--border-role)] pt-6">
          <span class="text-[11px] font-medium uppercase tracking-wide text-[var(--muted-fg)]">Segmented control · click to switch (indicator slides)</span>
          <div class="flex flex-wrap items-center gap-4">
            <div
              ref="segGroup"
              class="relative inline-flex items-center gap-0.5 rounded-lg bg-[var(--selected)] p-0.5"
            >
              <!-- selected = sliding white thumb; slides via translate, shrinks on press -->
              <div
                class="seg-thumb pointer-events-none absolute left-0 top-0 rounded-md bg-[var(--surface)]"
                :style="{
                  translate: segIndicator.left + 'px ' + segIndicator.top + 'px',
                  width: segIndicator.width + 'px',
                  height: segIndicator.height + 'px',
                  scale: segPressed ? '0.97' : '1',
                }"
              />
              <button
                v-for="seg in segments"
                :key="seg.id"
                type="button"
                :data-seg-active="activeSeg === seg.id"
                class="seg-item relative z-10 h-7 rounded-md px-3 text-sm tracking-[-0.16px] transition-colors duration-150"
                :class="activeSeg === seg.id ? 'is-active text-[var(--fg)]' : 'text-[var(--muted-fg)] hover:text-[var(--fg)]'"
                @pointerdown="onSegDown(seg.id)"
                @pointerup="segPressed = false"
                @pointerleave="segPressed = false"
                @pointercancel="segPressed = false"
                @click="activeSeg = seg.id"
              >
                {{ seg.label }}
              </button>
            </div>
            <span class="text-[11px] text-[var(--muted-fg)]">→ {{ activeSeg }}</span>
          </div>
        </div>

        <!-- ═══ 8. SETTINGS CONTEXT ══════════════════════════════════════ -->
        <div class="flex flex-col gap-3 border-t border-[var(--border-role)] pt-6">
          <span class="text-[11px] font-medium uppercase tracking-wide text-[var(--muted-fg)]">Same primary in a settings context</span>
          <div
            class="flex items-center justify-between border border-[var(--border-role)] bg-[var(--surface)] px-4 py-3"
            style="border-radius:12px"
          >
            <div class="flex flex-col leading-tight">
              <span class="text-sm font-medium">Billing info</span>
              <span class="text-xs text-[var(--muted-fg)]">Primary should not steal focus in settings</span>
            </div>
            <button
              type="button"
              :class="[BTN, primaryClass]"
              :style="primaryStyle"
            >
              Save changes
            </button>
          </div>
        </div>

        <!-- ═══ 9. EDIT (独立 action button) ═════════════════════════════ -->
        <div class="flex flex-col gap-3 border-t border-[var(--border-role)] pt-6">
          <span class="text-[11px] font-medium uppercase tracking-wide text-[var(--muted-fg)]">Edit · independent action (white@10% + ring + shadow)</span>
          <div class="flex flex-wrap items-center gap-4">
            <button
              type="button"
              :class="[EDIT, 'h-8 px-3 text-[14px] font-medium leading-[20px]']"
              :style="editStyle"
            >
              Edit
            </button>
            <span class="text-xs text-[var(--muted-fg)]">↔ vs secondary</span>
            <button
              type="button"
              :class="[BTN, SECONDARY]"
              :style="secondaryStyle"
            >
              Edit
            </button>
          </div>
        </div>

        <!-- ═══ 10. AUDIT ADDITIONS (the missing pieces) ═════════════════ -->
        <div class="flex flex-col gap-5 border-t border-[var(--border-role)] pt-6">
          <span class="text-[11px] font-medium uppercase tracking-wide text-[var(--muted-fg)]">Audit additions · the missing pieces</span>

          <!-- (a) FOCUS RING — keyboard a11y. Now on EVERY button; Tab to see it -->
          <div class="flex flex-col gap-2">
            <span class="text-[10px] text-[var(--muted-fg)]">① Focus ring · press <kbd class="rounded border border-[var(--border-role)] px-1 text-[10px]">Tab</kbd> to walk the buttons (now on every button on this page)</span>
            <div class="flex flex-wrap items-center gap-3">
              <button
                type="button"
                :class="[BTN, primaryClass]"
                :style="primaryStyle"
              >
                Tab me
              </button>
              <button
                type="button"
                :class="[BTN, SECONDARY]"
                :style="secondaryStyle"
              >
                then me
              </button>
              <button
                type="button"
                :class="[BTN, GHOST]"
                :style="btnStyle"
              >
                and me
              </button>
            </div>
          </div>

          <!-- (b) DESTRUCTIVE SOLID — confirm-dialog danger vs ghost inline -->
          <div class="flex flex-col gap-2">
            <span class="text-[10px] text-[var(--muted-fg)]">② Destructive · <b>solid</b> = confirm-dialog primary danger · ghost = inline row action</span>
            <div class="flex flex-wrap items-center gap-3">
              <button
                type="button"
                :class="[BTN, DESTRUCTIVE_SOLID]"
                :style="btnStyle"
              >
                <Trash2 />Delete account
              </button>
              <button
                type="button"
                :class="[BTN, DESTRUCTIVE]"
                :style="btnStyle"
              >
                <Trash2 />Delete
              </button>
            </div>
          </div>

          <!-- (c) SPLIT BUTTON — primary action + attached dropdown trigger.
               NO scale (halves fight + hide the divider) and NO press-color (it's
               normal-tier, not a big CTA) — just the hover color, no press flash. -->
          <div class="flex flex-col gap-2">
            <span class="text-[10px] text-[var(--muted-fg)]">③ Split button · primary + attached ⌄ trigger (no scale, no press-flash — hover color only)</span>
            <div class="inline-flex h-9 w-fit">
              <button
                type="button"
                :class="[primaryClass, 'btn-noscale inline-flex items-center px-4 text-sm font-medium']"
                style="border-radius:8px 0 0 8px"
              >
                Publish
              </button>
              <span
                class="z-10 w-px self-stretch bg-[oklch(1_0_0_/_0.22)]"
                style="margin:6px 0"
              />
              <button
                type="button"
                :class="[primaryClass, 'btn-noscale inline-flex items-center px-2 [&_svg]:size-4']"
                style="border-radius:0 8px 8px 0"
              >
                <ChevronDown />
              </button>
            </div>
          </div>

          <!-- (d) BLOCK / FULL-WIDTH (btn-cta) — color-switch, no scale (full-width
               lurch): hover lightens to rgb(51), press lightens to rgb(92) INSTANT.
               btn-cta swaps the scale-press for a color-press (split does NOT). -->
          <div class="flex flex-col gap-2">
            <span class="text-[10px] text-[var(--muted-fg)]">④ Block · full-width (btn-cta) · no scale → press LIGHTENS to rgb(92) (hover 51 → press 92)</span>
            <div class="w-72">
              <button
                type="button"
                :class="[BTN, primaryClass, 'btn-cta w-full']"
                :style="primaryStyle"
              >
                Continue
              </button>
            </div>
          </div>

          <!-- (e) asChild / POLYMORPHIC — same button styling on a real <a>. A CTA
               that NAVIGATES must be an anchor (right-click → Open in new tab,
               ⌘-click, SEO, middle-click all work; a <button> breaks all of them).
               In @memohai/ui this is the asChild prop; here it's just the classes
               on an <a>. No type/disabled — anchors aren't form controls. -->
          <div class="flex flex-col gap-2">
            <span class="text-[10px] text-[var(--muted-fg)]">⑤ asChild · button look on a real <code>&lt;a&gt;</code> — right-click → “Open in new tab” works (a button can’t)</span>
            <div class="flex flex-wrap items-center gap-3">
              <a
                href="https://example.com"
                target="_blank"
                rel="noopener noreferrer"
                :class="[BTN, primaryClass, 'no-underline']"
                :style="primaryStyle"
              >
                Open docs<ExternalLink />
              </a>
              <a
                href="https://example.com"
                target="_blank"
                rel="noopener noreferrer"
                :class="[BTN, SECONDARY, 'no-underline']"
                :style="secondaryStyle"
              >
                Open docs<ExternalLink />
              </a>
            </div>
          </div>

          <!-- (f) btn-cozy (OPTIONAL) — our own hover micro-GROW (not a sheen).
               Reserve for cozy in-app / corner utility buttons. Default primary
               (left) doesn't grow; cozy (right) lifts a touch on hover. -->
          <div class="flex flex-col gap-2">
            <span class="text-[10px] text-[var(--muted-fg)]">⑥ btn-cozy · optional hover micro-grow for in-app corner buttons (default vs cozy — hover both)</span>
            <div class="flex flex-wrap items-center gap-3">
              <button
                type="button"
                :class="[BTN, primaryClass]"
                :style="primaryStyle"
              >
                Default
              </button>
              <button
                type="button"
                :class="[BTN, primaryClass, 'btn-cozy']"
                :style="primaryStyle"
              >
                Cozy (grows)
              </button>
            </div>
          </div>
        </div>
      </div>
    </SceneFrame>
  </SectionShell>
</template>

<style scoped>
/* ── Secondary: bg/ring/states on ::before so text NEVER scales ────────── */
.btn-secondary {
  position: relative;
  isolation: isolate;
}
.btn-secondary::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  background-color: transparent;
  box-shadow: inset 0 0 0 1px oklch(0 0 0 / 0.16);
  z-index: -1;
  transition:
    scale 0.255s linear(0 0%, 0.3505 11.11%, 0.7432 22.22%, 0.9336 33.33%, 0.9951 44.44%, 1.0062 55.56%, 1.0045 66.67%, 1.0019 77.78%, 1.0005 88.89%, 1 100%),
    background-color 0.11s ease-out,
    box-shadow 0.11s ease-out;
}
.btn-secondary:hover::before {
  background-color: oklch(0 0 0 / 0.12);
  box-shadow: none;
}
.btn-secondary:active::before {
  scale: 0.974; /* press shrink 0.026 (trim ~19%, was 0.968) */
  box-shadow: none;
}

/* ── Edit: bg/ring/shadow/states on ::before ───────────────────────────── */
.btn-edit {
  position: relative;
  isolation: isolate;
}
.btn-edit::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  background-color: rgba(255, 255, 255, 0.1);
  box-shadow:
    inset 0 0 0 0.75px oklch(0 0 0 / 0.12),
    0 1px 2px 0 oklch(0 0 0 / 0.05);
  z-index: -1;
  transition:
    scale 0.38s linear(0 0%, 0.2459 7.14%, 0.6526 14.29%, 0.9468 21.43%, 1.0764 28.57%, 1.0915 35.71%, 1.0585 42.86%, 1.0219 50%, 0.9993 57.14%, 0.9914 64.29%, 0.9921 71.43%, 0.9957 78.57%, 0.9988 85.71%, 1.0004 92.86%, 1 100%),
    background-color 0.09s ease-out;
}
.btn-edit:hover::before {
  background-color: oklch(0 0 0 / 0.03);
}
.btn-edit:active::before {
  scale: 0.961; /* press shrink ~0.039 (matched ~19% trim, was 0.952) */
  background-color: oklch(0 0 0 / 0.06);
}

/* ── Primary: COLOR-SWITCH on ::before (no sheen) ──────────────────────────
   rest --fg → hover rgb(51) → small press scale(0.96) / wide press rgb(92).
   bg lives on ::before so press-scale never moves the text. */
.btn-primary {
  position: relative;
  isolation: isolate;
  color: var(--surface);
}
.btn-primary--brand { color: var(--brand-foreground, #fff); }

.btn-primary::before {
  content: '';
  position: absolute;
  inset: 0;
  z-index: -1;
  border-radius: inherit;
  background-color: var(--fg);
  scale: 1 1;
  transition:
    scale 0.255s linear(0, .3505, .7432, .9336, .9951, 1.0062, 1.0045, 1.0019, 1.0005, 1),
    background-color 0.15s ease-out;
}
.btn-primary--brand::before { background-color: var(--brand); }

/* hover: fill LIGHTENS to rgb(51) (neutral). A generic lighten-on-engage
   pattern. Brand brightens instead. */
.btn-primary:not(.btn-primary--brand):hover::before {
  background-color: rgb(51, 51, 51);
}
.btn-primary--brand:hover::before {
  filter: brightness(1.08);
}

/* press — SMALL (default): scale DOWN 0.96 (springy via base transition). The
   scale IS the signal; the fill stays at its hover color. */
.btn-primary:active::before {
  scale: 0.96;
}

/* btn-noscale: ONLY kills the press-scale (split halves would fight + hide the
   divider; full-width lurches). No press-color — a no-scale button that is still
   "normal tier" (the split halves) must NOT flash the CTA color. Its press just
   holds the hover color. The color-press is opt-in via btn-cta below. */
.btn-primary.btn-noscale:active::before {
  scale: 1 1;
}

/* btn-cta: the full-width primary CTA — no scale, and press LIGHTENS to rgb(92)
   INSTANT (its only press signal, since scale would lurch). Holds rgb(92) through
   loading too. This is the SEPARATE big-button signal; split stays btn-noscale. */
.btn-primary.btn-cta:active::before {
  scale: 1 1;
}
.btn-primary.btn-cta:not(.btn-primary--brand):active::before {
  background-color: rgb(92, 92, 92);
  transition: background-color 0s;
}
.btn-primary.btn-cta.btn-primary--brand:active::before {
  filter: brightness(1.16);
}

/* ── btn-cozy (OPTIONAL): our OWN hover micro-GROW for cozy / corner buttons ──
   A subtle lift on hover, sitting on top of the standard color-switch; no sheen.
   Reserve for app-内 small utility buttons where a little lift feels nice. */
.btn-primary.btn-cozy:hover::before {
  scale: var(--hover-sx, 1.012) var(--hover-sy, 1.025);
}


/* ── Destructive · SOLID (filled red) — confirm-dialog primary danger ─────
   Mirrors the primary ::before architecture (bg on a scaling shell, text never
   moves) but red. Pair with the existing ghost-red DESTRUCTIVE for inline rows. */
.btn-destructive-solid {
  position: relative;
  isolation: isolate;
  color: #fff;
}
.btn-destructive-solid::before {
  content: '';
  position: absolute;
  inset: 0;
  z-index: -1;
  border-radius: inherit;
  background-color: var(--ac-red);
  transition:
    scale 0.255s linear(0, .3505, .7432, .9336, .9951, 1.0062, 1.0045, 1.0019, 1.0005, 1),
    background-color 0.15s ease;
}
.btn-destructive-solid:hover::before {
  background-color: #b8302e; /* darker red */
}
.btn-destructive-solid:active::before {
  scale: 0.968;
}

/* ── Accent palette — SINGLE SOURCE = the accent board (see SectionAccents.vue).
   Do NOT invent ad-hoc oklch here; keep these in lockstep with the accent board
   so colors never fragment. Promote to style.css later. */
.contract-scene {
  --ac-purple: #9065b0;
  --ac-blue: #2383e2;
  --ac-red: #cd3c3a;
  --ac-amber: #d8a32f;
  --ac-green: #448361;

  /* Gray ladder (rgb 255 → 249 → 243 → 237 → 231), ~0.018 oklch L per step.
     249=--hover, 243=--selected are the system grays; deeper steps add the
     ghost-hover tint and the toggle's pressed/selected-pressed layers. */
  --gray-3: oklch(0.946 0 0); /* ~237 */
  --gray-4: oklch(0.928 0 0); /* ~231 */
}

/* ── Cursor — every interactive control gets the pointer ──────────────────
   Tailwind Preflight (v3+) resets <button> to cursor:default; modern apps put it
   back. The pointer IS part of the hover affordance. Disabled opts out. */
.contract-scene button:not(:disabled),
.contract-scene a {
  cursor: pointer;
}

/* ── Busy ≠ Disabled — a loading button is set [disabled] to block double-clicks,
   but it must NOT look faded/inert. Keep full color + show the spinner as the only
   busy signal. (Overrides BTN's disabled:opacity-40 for the busy case only.) */
.contract-scene .btn-busy:disabled {
  opacity: 1;
}

/* ── Animated leading spinner (full-width CTA) — the spinner ANIMATES in/out, it
   does not hard-swap. The slot's width grows 0 → 1.5rem, which (with the button's
   justify-center) smoothly pushes the label to the right; the spinner itself fades
   + slides in from the left. Exit reverses. We drop BTN's gap so the slot owns all
   the spacing (a 0-width slot would otherwise leave a phantom gap). */
.btn-spin {
  gap: 0;
}
.btn-spin .spin-slot {
  display: inline-grid;
  place-items: center;
  width: 0;
  opacity: 0;
  translate: -5px 0;
  overflow: hidden;
  transition:
    width 0.24s cubic-bezier(0.32, 0.72, 0, 1),
    opacity 0.18s ease,
    translate 0.24s cubic-bezier(0.32, 0.72, 0, 1);
}
.btn-spin.is-loading .spin-slot {
  width: 1.5rem; /* 1rem icon + 0.5rem trailing space to the label */
  opacity: 1;
  translate: 0 0;
}

/* ── Loading = HOLD the most-engaged color (no pop on the click→spin handoff) ──
   A loading button freezes at the deepest color it reached while interacting,
   instead of snapping back to rest. All values are EXISTING (hover/press) colors —
   nothing new, no fragmentation. loading→done eases back via the base transition.
     · wide color-press button → its press color rgb(92)
     · normal primary          → its hover color rgb(51)
     · secondary / ghost       → their hover grays */
.btn-primary.btn-cta:not(.btn-primary--brand).is-loading::before {
  background-color: rgb(92, 92, 92);
}
.btn-primary:not(.btn-cta):not(.btn-primary--brand).is-loading::before {
  background-color: rgb(51, 51, 51);
}
.btn-secondary.is-loading::before {
  background-color: oklch(0 0 0 / 0.12);
  box-shadow: none;
}
.btn-ghost.is-loading::before {
  background-color: var(--gray-3);
}

/* ── Focus ring — keyboard a11y on EVERY interactive button ──────────────
   :focus-visible = keyboard only (mouse clicks stay clean). outline follows the
   element's border-radius automatically, and offset works now that clip-path /
   squircle is gone. This is the #1 "real library" gate. */
.contract-scene button:focus-visible {
  outline: 2px solid var(--ac-blue);
  outline-offset: 2px;
}

/* ── Ghost button: hover tint (gray-3 / 237), press SHRINKS like secondary ──
   Same action-button tier as primary/secondary — the press signal is a scale-
   down on a bg-only ::before (text never moves), NOT an extra gray step. */
.btn-ghost {
  position: relative;
  isolation: isolate;
}
.btn-ghost::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  background-color: transparent;
  z-index: -1;
  transition:
    scale 0.255s linear(0, .3505, .7432, .9336, .9951, 1.0062, 1.0045, 1.0019, 1.0005, 1),
    background-color 150ms ease;
}
.btn-ghost:hover::before {
  background-color: var(--gray-3);
}
.btn-ghost:active::before {
  scale: 0.974; /* press shrink 0.026 (trim ~19%, was 0.968) */
  background-color: var(--gray-3);
}

/* ── Link · DEFAULT (fade): underline opacity 0 → 1 on hover ──────────────
   Underline is a ::after bar so we can fade just the line (opacity on the
   element would dim the text too). Inline box (no h-9) → line hugs the text. */
.link-fade {
  position: relative;
  padding-bottom: 3px; /* underline offset below the text */
}
.link-fade::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 1.5px;
  background-color: currentColor;
  opacity: 0;
  transition: opacity 0.18s ease;
}
.link-fade:hover::after {
  opacity: 1;
}

/* ── Link · DRAW (landing copy): underline grows in from the LEFT on hover ──
   background-image gradient (not text-decoration) so width can animate 0%→100%
   with the left edge pinned. Reserve for hero / marketing CTAs. */
.link-draw {
  background-image: linear-gradient(currentColor, currentColor);
  background-repeat: no-repeat;
  background-position: 0 100%;
  background-size: 0% 1.5px;
  padding-bottom: 3px;
  transition:
    background-size 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    color 150ms ease;
}
.link-draw:hover {
  background-size: 100% 1.5px;
}

/* ── Toggle: the gray-ladder lives HERE (not on action buttons) ────────
   rest → hover (249) → press (243, quick) → release (eases back); selected
   sits at 243, and pressing a selected item dips to 237. Press-down is snappy
   (40ms), release eases (160ms) — the "modern" press feel. */
.btn-toggle {
  background-color: transparent;
  transition: background-color 160ms ease; /* eased release / mouse-leave */
}
.btn-toggle:hover {
  background-color: var(--hover); /* 249 */
}
.btn-toggle:active {
  background-color: var(--gray-3); /* 237 — press (2 steps below hover, clearly visible) */
  transition-duration: 40ms;
}
.btn-toggle.is-active {
  background-color: var(--selected); /* 243 — selected */
}
.btn-toggle.is-active:active {
  background-color: var(--gray-4); /* 231 — press while selected */
  transition-duration: 40ms;
}

/* ── Segmented control: white thumb (selected) slides + shrinks on press;
   non-selected items get a SMALLER hover block that also shrinks on press.
   Faint shadow only — stays close to flat. ──────────────────────────────── */
.seg-thumb {
  transition:
    translate 0.25s cubic-bezier(0.32, 0.72, 0, 1),
    width 0.25s cubic-bezier(0.32, 0.72, 0, 1),
    scale 0.16s ease;
  box-shadow: 0 1px 2px oklch(0 0 0 / 0.08);
}
.seg-item {
  /* uniform weight across states — selection reads via the white thumb, not a
     weight change; ~25 lighter than the old 500 */
  font-weight: 475;
}
.seg-item::before {
  content: '';
  position: absolute;
  inset: 0; /* SAME footprint as the white thumb … */
  border-radius: inherit; /* … and SAME radius (seg-item rounded-md = thumb) — no mismatch */
  background-color: transparent;
  z-index: -1;
  transition:
    background-color 150ms ease,
    scale 0.2s cubic-bezier(0.32, 0.72, 0, 1);
}
.seg-item:not(.is-active):hover::before {
  background-color: oklch(0 0 0 / 0.05); /* DARKENS the track (not lighten) */
}
.seg-item:not(.is-active):active::before {
  background-color: oklch(0 0 0 / 0.08);
  scale: 0.965; /* gentle press, not a big shrink */
}

/* ── Color-only toggle: tint icon (blue on active), no persistent fill ── */
/* Two-layer: selection is signaled by COLOR, so the gray hover goes straight
   to 243 (no need to reserve a darker "selected" gray). */
.btn-tint {
  color: var(--muted-fg);
  /* ONLY background eases — the icon COLOR swap stays instant (not listed) */
  transition: background-color 160ms ease;
}
.btn-tint:hover {
  background-color: var(--selected); /* 243 */
}
.btn-tint:active {
  background-color: var(--gray-3); /* 237 — press, same feel as btn-toggle */
  transition-duration: 40ms;
}
.btn-tint.is-active {
  color: var(--ac-blue);
}
</style>
