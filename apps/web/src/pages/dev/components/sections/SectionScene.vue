<script setup lang="ts">
// Shape test on the LOCKED neutral base. Picks light/dark + a SHAPE language
// (radius / icon shape / input shape / shadow) flipped live so the geometry can be
// judged by eye on real interactive components (Plus opens a real menu). The base
// palette is locked (lib/palettes.ts) — no palette switcher; just toggles + eyes.
import { computed, ref } from 'vue'
import SectionShell from '../components/SectionShell.vue'
import SceneFrame from '../components/SceneFrame.vue'
import SceneChat from '../components/SceneChat.vue'
import { palettes } from '../lib/palettes'

type IconShape = 'square' | 'circle'
type InputShape = 'rect' | 'pill'
type ShadowLevel = 'none' | 'subtle' | 'float'

const base = computed(() => palettes.find((p) => p.id === 'base') ?? palettes[0])
const dark = ref(false)

// Shape language
const radius = ref('8px')
const iconShape = ref<IconShape>('square')
const inputShape = ref<InputShape>('pill')
const shadow = ref<ShadowLevel>('subtle')

const shape = computed(() => ({
  radius: radius.value,
  iconRadius: iconShape.value === 'circle' ? '9999px' : radius.value,
  // Icons INSIDE the input: a pill container wants round children, so force full
  // when the input is a pill; otherwise follow the global icon-shape toggle.
  inputIconRadius: inputShape.value === 'pill' ? '9999px' : (iconShape.value === 'circle' ? '9999px' : radius.value),
  inputRadius: inputShape.value === 'pill' ? '9999px' : `calc(${radius.value} + 6px)`,
  cardRadius: `calc(${radius.value} + 4px)`,
  // Layered + ultra-low-opacity + negative spread; the element's hairline border
  // keeps the edge crisp so the shadow only lifts (no dirty gray fringe).
  // NOTE: shadow is only roughed-in here — it can't be finalized in isolation;
  // it reveals itself in real composition, tuned at apply-time.
  shadow:
    shadow.value === 'none'
      ? 'none'
      : shadow.value === 'float'
        ? '0 2px 6px -1px oklch(0 0 0 / 0.05), 0 14px 36px -8px oklch(0 0 0 / 0.10)'
        : '0 1px 2px 0 oklch(0 0 0 / 0.04), 0 6px 18px -6px oklch(0 0 0 / 0.08)',
}))

const radii = ['6px', '8px', '10px', '12px']
const iconShapes: { id: IconShape; label: string }[] = [
  { id: 'square', label: '▢ 方' },
  { id: 'circle', label: '● 圆' },
]
const inputShapes: { id: InputShape; label: string }[] = [
  { id: 'rect', label: '矩形' },
  { id: 'pill', label: '胶囊' },
]
const shadows: { id: ShadowLevel; label: string }[] = [
  { id: 'none', label: '无(纯描边)' },
  { id: 'subtle', label: '轻' },
  { id: 'float', label: '浮' },
]
</script>

<template>
  <SectionShell
    id="scene"
    label="Scene — context preview (shape)"
    description="Judge shape on real, interactive components over the locked neutral base (Plus opens a real menu). Toggle below; tune base values in lib/palettes.ts (HMR). Shadow is only roughed-in — finalized later in real composition."
  >
    <!-- Row 1: theme -->
    <div class="mb-2 flex flex-wrap items-center gap-2">
      <div class="flex items-center gap-1 rounded-lg border border-border p-0.5">
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
    </div>

    <!-- Row 2: shape language -->
    <div class="mb-3 flex flex-wrap items-center gap-x-4 gap-y-2 text-xs">
      <div class="flex items-center gap-1.5">
        <span class="text-muted-foreground">圆角</span>
        <div class="flex items-center gap-0.5 rounded-lg border border-border p-0.5">
          <button
            v-for="r in radii"
            :key="r"
            type="button"
            class="rounded-md px-2 py-1 transition-colors"
            :class="radius === r ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'"
            @click="radius = r"
          >
            {{ r }}
          </button>
        </div>
      </div>

      <div class="flex items-center gap-1.5">
        <span class="text-muted-foreground">图标</span>
        <div class="flex items-center gap-0.5 rounded-lg border border-border p-0.5">
          <button
            v-for="s in iconShapes"
            :key="s.id"
            type="button"
            class="rounded-md px-2 py-1 transition-colors"
            :class="iconShape === s.id ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'"
            @click="iconShape = s.id"
          >
            {{ s.label }}
          </button>
        </div>
      </div>

      <div class="flex items-center gap-1.5">
        <span class="text-muted-foreground">输入框</span>
        <div class="flex items-center gap-0.5 rounded-lg border border-border p-0.5">
          <button
            v-for="s in inputShapes"
            :key="s.id"
            type="button"
            class="rounded-md px-2 py-1 transition-colors"
            :class="inputShape === s.id ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'"
            @click="inputShape = s.id"
          >
            {{ s.label }}
          </button>
        </div>
      </div>

      <div class="flex items-center gap-1.5">
        <span class="text-muted-foreground">阴影</span>
        <div class="flex items-center gap-0.5 rounded-lg border border-border p-0.5">
          <button
            v-for="s in shadows"
            :key="s.id"
            type="button"
            class="rounded-md px-2 py-1 transition-colors"
            :class="shadow === s.id ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'"
            @click="shadow = s.id"
          >
            {{ s.label }}
          </button>
        </div>
      </div>
    </div>

    <p class="mb-3 max-w-3xl text-xs leading-relaxed text-muted-foreground">
      {{ base.note }}
    </p>

    <SceneFrame
      :palette="base"
      :dark="dark"
    >
      <SceneChat :shape="shape" />
    </SceneFrame>
  </SectionShell>
</template>
