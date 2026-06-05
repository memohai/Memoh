<script setup lang="ts">
// Scale tuning bench (bottom-up). Each ROLE gets its own slider — tune the real
// component in context until it looks right, no file editing. The readout lists
// every current value so near-equal ones can be consolidated into a named scale
// afterward (scale is EXTRACTED from what looks good, not imposed up front).
// Judged on the locked pure-white base. Defaults seeded from lib/shape.ts.
import { computed, reactive, ref } from 'vue'
import { ArrowUp } from 'lucide-vue-next'
import SectionShell from '../components/SectionShell.vue'
import SceneFrame from '../components/SceneFrame.vue'
import { palettes } from '../lib/palettes'

const dark = ref(false)
const base = computed(() => palettes.find((p) => p.id === 'base') ?? palettes[0])

// Per-role radius (px). Tune each in context; consolidate later.
const r = reactive({
  badge: 6,
  button: 8,
  input: 14,
  menu: 8,
  card: 12,
  dialog: 16,
})
// Per-level elevation intensity (alpha multiplier; 1 = the tuned baseline).
const e = reactive({ card: 1, dropdown: 1, dialog: 1 })

function elevCard(i: number) {
  return `0 1px 2px 0 oklch(0 0 0 / ${(0.04 * i).toFixed(3)})`
}
function elevDropdown(i: number) {
  return `0 1px 2px 0 oklch(0 0 0 / ${(0.04 * i).toFixed(3)}), 0 6px 18px -6px oklch(0 0 0 / ${(0.08 * i).toFixed(3)})`
}
function elevDialog(i: number) {
  return `0 2px 6px -1px oklch(0 0 0 / ${(0.05 * i).toFixed(3)}), 0 14px 36px -8px oklch(0 0 0 / ${(0.1 * i).toFixed(3)})`
}

const radiusReadout = computed(() =>
  `radius  badge ${r.badge} · button ${r.button} · input ${r.input} · menu ${r.menu} · card ${r.card} · dialog ${r.dialog}`,
)
const elevReadout = computed(() =>
  `elev    card ×${e.card} · dropdown ×${e.dropdown} · dialog ×${e.dialog}`,
)

const radiusRows = [
  { key: 'badge', label: 'Badge / tag' },
  { key: 'button', label: 'Button' },
  { key: 'input', label: 'Input' },
  { key: 'menu', label: 'Menu item / row' },
  { key: 'card', label: 'Card' },
  { key: 'dialog', label: 'Dialog' },
] as const
</script>

<template>
  <SectionShell
    id="scale"
    label="Scale — tune per role, extract later"
    description="Bottom-up: drag each role's slider until the real component looks right, reusing values where you can. The readout collects them; consolidate into a named scale at the end."
  >
    <div class="mb-3 flex w-fit items-center gap-1 rounded-lg border border-border p-0.5">
      <button
        type="button"
        class="rounded-md px-2.5 py-1 text-xs transition-colors"
        :class="!dark ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'"
        @click="dark = false"
      >
        Light
      </button>
      <button
        type="button"
        class="rounded-md px-2.5 py-1 text-xs transition-colors"
        :class="dark ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'"
        @click="dark = true"
      >
        Dark
      </button>
    </div>

    <!-- live readout for later consolidation -->
    <pre class="mb-3 overflow-x-auto rounded-lg border border-border bg-muted/40 px-3 py-2 text-[11px] leading-relaxed text-muted-foreground">{{ radiusReadout }}
    {{ elevReadout }}</pre>

    <SceneFrame
      :palette="base"
      :dark="dark"
    >
      <div class="flex flex-col gap-7 p-6 text-[var(--fg)]">
        <!-- Radius per role -->
        <div class="flex flex-col gap-4">
          <span class="text-xs font-medium text-[var(--muted-fg)]">Radius — per role (px)</span>
          <div
            v-for="row in radiusRows"
            :key="row.key"
            class="flex items-center gap-6"
          >
            <div class="flex w-52 shrink-0 items-center gap-2">
              <span class="w-28 text-xs">{{ row.label }}</span>
              <input
                v-model.number="r[row.key]"
                type="range"
                min="0"
                max="24"
                step="1"
                class="flex-1 accent-[var(--brand)]"
              >
              <span class="w-9 text-right text-xs tabular-nums text-[var(--muted-fg)]">{{ r[row.key] }}</span>
            </div>

            <div class="flex flex-wrap items-center gap-3">
              <template v-if="row.key === 'badge'">
                <span
                  class="bg-[var(--hover)] px-2 py-0.5 text-xs"
                  :style="{ borderRadius: r.badge + 'px' }"
                >Badge</span>
                <span
                  class="border border-[var(--border-role)] px-2 py-0.5 text-xs"
                  :style="{ borderRadius: r.badge + 'px' }"
                >tag</span>
              </template>

              <template v-else-if="row.key === 'button'">
                <button
                  class="bg-[var(--brand)] px-3.5 py-2 text-sm font-medium text-[var(--brand-foreground)]"
                  :style="{ borderRadius: r.button + 'px' }"
                >
                  Confirm
                </button>
                <button
                  class="border border-[var(--border-role)] bg-[var(--surface)] px-3.5 py-2 text-sm font-medium"
                  :style="{ borderRadius: r.button + 'px' }"
                >
                  Cancel
                </button>
                <button
                  class="px-3.5 py-2 text-sm font-medium hover:bg-[var(--hover)]"
                  :style="{ borderRadius: r.button + 'px' }"
                >
                  Ghost
                </button>
              </template>

              <template v-else-if="row.key === 'input'">
                <div
                  class="flex w-72 items-center gap-2 border border-[var(--border-role)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--muted-fg)]"
                  :style="{ borderRadius: r.input + 'px' }"
                >
                  <span class="flex-1">Message…</span>
                  <span class="text-[10px]">(pill = max)</span>
                </div>
              </template>

              <template v-else-if="row.key === 'menu'">
                <div
                  class="flex w-48 flex-col gap-0.5 border border-[var(--border-role)] bg-[var(--surface)] p-1.5"
                  :style="{ borderRadius: (r.menu + 4) + 'px', boxShadow: elevDropdown(e.dropdown) }"
                >
                  <div
                    class="px-2.5 py-1.5 text-sm transition-colors hover:bg-[var(--hover)]"
                    :style="{ borderRadius: r.menu + 'px' }"
                  >
                    Rename
                  </div>
                  <div
                    class="bg-[var(--selected)] px-2.5 py-1.5 text-sm"
                    :style="{ borderRadius: r.menu + 'px' }"
                  >
                    Pin
                  </div>
                </div>
              </template>

              <template v-else-if="row.key === 'card'">
                <div
                  class="w-60 border border-[var(--border-role)] bg-[var(--surface)] p-4"
                  :style="{ borderRadius: r.card + 'px', boxShadow: elevCard(e.card) }"
                >
                  <div class="text-sm font-semibold">
                    Card title
                  </div>
                  <div class="mt-1 text-xs text-[var(--muted-fg)]">
                    Supporting copy on a card.
                  </div>
                </div>
              </template>

              <template v-else>
                <div
                  class="w-64 border border-[var(--border-role)] bg-[var(--surface)] p-4"
                  :style="{ borderRadius: r.dialog + 'px', boxShadow: elevDialog(e.dialog) }"
                >
                  <div class="text-sm font-semibold">
                    Dialog
                  </div>
                  <div class="mb-3 mt-1 text-xs text-[var(--muted-fg)]">
                    Confirm this action?
                  </div>
                  <div class="flex justify-end gap-2">
                    <button
                      class="px-3 py-1.5 text-xs hover:bg-[var(--hover)]"
                      :style="{ borderRadius: r.button + 'px' }"
                    >
                      Cancel
                    </button>
                    <button
                      class="bg-[var(--brand)] px-3 py-1.5 text-xs text-[var(--brand-foreground)]"
                      :style="{ borderRadius: r.button + 'px' }"
                    >
                      Confirm
                    </button>
                  </div>
                </div>
                <div class="grid size-9 place-items-center rounded-full bg-[var(--brand)] text-[var(--brand-foreground)]">
                  <ArrowUp class="size-5" />
                </div>
              </template>
            </div>
          </div>
        </div>

        <!-- Elevation intensity per surface -->
        <div class="flex flex-col gap-4 border-t border-[var(--border-role)] pt-6">
          <span class="text-xs font-medium text-[var(--muted-fg)]">Elevation — intensity per surface (× alpha)</span>
          <div class="flex flex-wrap gap-8">
            <div class="flex flex-col gap-2">
              <div class="flex items-center gap-2">
                <span class="w-16 text-xs">Card</span>
                <input
                  v-model.number="e.card"
                  type="range"
                  min="0"
                  max="2"
                  step="0.1"
                  class="w-28 accent-[var(--brand)]"
                >
                <span class="text-xs tabular-nums text-[var(--muted-fg)]">×{{ e.card.toFixed(1) }}</span>
              </div>
              <div
                class="grid h-20 w-44 place-items-center border border-[var(--border-role)] bg-[var(--surface)] text-xs text-[var(--muted-fg)]"
                :style="{ boxShadow: elevCard(e.card), borderRadius: r.card + 'px' }"
              >
                card
              </div>
            </div>

            <div class="flex flex-col gap-2">
              <div class="flex items-center gap-2">
                <span class="w-16 text-xs">Dropdown</span>
                <input
                  v-model.number="e.dropdown"
                  type="range"
                  min="0"
                  max="2"
                  step="0.1"
                  class="w-28 accent-[var(--brand)]"
                >
                <span class="text-xs tabular-nums text-[var(--muted-fg)]">×{{ e.dropdown.toFixed(1) }}</span>
              </div>
              <div
                class="grid h-20 w-44 place-items-center border border-[var(--border-role)] bg-[var(--surface)] text-xs text-[var(--muted-fg)]"
                :style="{ boxShadow: elevDropdown(e.dropdown), borderRadius: r.card + 'px' }"
              >
                dropdown
              </div>
            </div>

            <div class="flex flex-col gap-2">
              <div class="flex items-center gap-2">
                <span class="w-16 text-xs">Dialog</span>
                <input
                  v-model.number="e.dialog"
                  type="range"
                  min="0"
                  max="2"
                  step="0.1"
                  class="w-28 accent-[var(--brand)]"
                >
                <span class="text-xs tabular-nums text-[var(--muted-fg)]">×{{ e.dialog.toFixed(1) }}</span>
              </div>
              <div
                class="grid h-20 w-44 place-items-center border border-[var(--border-role)] bg-[var(--surface)] text-xs text-[var(--muted-fg)]"
                :style="{ boxShadow: elevDialog(e.dialog), borderRadius: r.dialog + 'px' }"
              >
                dialog
              </div>
            </div>
          </div>
        </div>
      </div>
    </SceneFrame>
  </SectionShell>
</template>
