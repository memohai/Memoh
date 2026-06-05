<script setup lang="ts">
// Chat skeleton for the color + SHAPE test (Memoh multi-bot). Reads palette role
// vars (set by SceneFrame) AND a `shape` prop (radius / icon shape / input shape /
// shadow) driven by the Scene control bar — so the geometry language can be
// flipped live and judged on REAL interactive components (Plus opens a real menu,
// send/hovers are live), not isolated swatches.
import { ref } from 'vue'
import {
  ArrowUp, Bot, ChevronRight, Database, FileUp, PanelLeft, Plug, Plus, Search, SquarePen,
} from 'lucide-vue-next'

defineProps<{
  shape: {
    radius: string // text buttons, menu items, rows
    iconRadius: string // sidebar icon buttons (square vs circle)
    inputIconRadius: string // Plus + send INSIDE the input (round when pill)
    inputRadius: string // input dock (rect vs pill)
    cardRadius: string // menu / popover
    shadow: string // overlay shadow
  }
}>()

interface ChatBot {
  name: string
  preview: string
  state?: 'selected' | 'hover'
}
const bots: ChatBot[] = [
  { name: 'Atlas', preview: '问题主要三点:灰阶混了两种家族…', state: 'selected' },
  { name: 'Sora', preview: '草稿已经更新到第三版,你看下' },
  { name: 'Nova', preview: '今天的播客脚本写好了' },
  { name: 'Echo', preview: '三个候选方案的对比表整理完了' },
  { name: 'Koto', preview: '已归档 · 上周' },
]

const addMenu = [
  { icon: FileUp, label: '从文件添加', chevron: false },
  { icon: Database, label: '从知识库添加', chevron: true },
  { icon: Plug, label: '添加连接器', chevron: true },
]

const menuOpen = ref(false)
</script>

<template>
  <div class="flex h-[640px] text-[var(--fg)]">
    <!-- Sidebar (surface-sunken) -->
    <aside class="flex w-64 shrink-0 flex-col border-r border-[var(--border-role)] bg-[var(--surface-sunken)]">
      <div class="flex items-center justify-between px-3 py-3">
        <span class="px-1 text-sm font-semibold">Memoh</span>
        <div class="flex items-center gap-1">
          <button
            class="grid size-7 place-items-center text-[var(--muted-fg)] transition-colors hover:bg-[var(--hover)] hover:text-[var(--fg)]"
            :style="{ borderRadius: shape.iconRadius }"
          >
            <SquarePen class="size-4" />
          </button>
          <button
            class="grid size-7 place-items-center text-[var(--muted-fg)] transition-colors hover:bg-[var(--hover)] hover:text-[var(--fg)]"
            :style="{ borderRadius: shape.iconRadius }"
          >
            <PanelLeft class="size-4" />
          </button>
        </div>
      </div>

      <!-- search field: input border on the sunken surface -->
      <div class="px-2.5 pb-2">
        <div
          class="flex items-center gap-2 border border-[var(--border-role)] bg-[var(--surface)] px-2.5 py-1.5"
          :style="{ borderRadius: shape.radius }"
        >
          <Search class="size-3.5 text-[var(--muted-fg)]" />
          <span class="text-xs text-[var(--muted-fg)]">搜索 bot</span>
        </div>
      </div>

      <!-- bot list -->
      <div class="flex flex-1 flex-col gap-0.5 overflow-y-auto px-2 py-1">
        <div
          v-for="bot in bots"
          :key="bot.name"
          class="flex cursor-pointer items-center gap-2.5 px-2 py-2 transition-colors"
          :style="{ borderRadius: shape.radius }"
          :class="{
            'bg-[var(--selected)]': bot.state === 'selected',
            'bg-[var(--hover)]': bot.state === 'hover',
            'hover:bg-[var(--hover)]': !bot.state,
          }"
        >
          <Bot
            class="size-5 shrink-0"
            :class="bot.state === 'selected' ? 'text-[var(--fg)]' : 'text-[var(--muted-fg)]'"
          />
          <div class="min-w-0 flex-1">
            <div
              class="truncate text-sm"
              :class="bot.state === 'selected' ? 'font-semibold' : 'font-medium'"
            >
              {{ bot.name }}
            </div>
            <div class="truncate text-xs text-[var(--muted-fg)]">
              {{ bot.preview }}
            </div>
          </div>
        </div>
      </div>

      <div class="mt-auto flex items-center gap-2.5 border-t border-[var(--border-role)] px-3 py-3">
        <div class="grid size-7 place-items-center rounded-full bg-[var(--brand)] text-xs font-semibold text-[var(--brand-foreground)]">
          清
        </div>
        <div class="flex flex-col leading-tight">
          <span class="text-sm font-medium">清凤</span>
          <span class="text-[11px] text-[var(--muted-fg)]">Pro</span>
        </div>
      </div>
    </aside>

    <!-- Main (surface) -->
    <main class="flex flex-1 flex-col bg-[var(--surface)]">
      <header class="flex items-center gap-2.5 border-b border-[var(--border-role)] px-5 py-3">
        <Bot class="size-5 text-[var(--fg)]" />
        <div class="flex flex-col leading-tight">
          <span class="text-sm font-semibold">Atlas</span>
          <span class="text-[11px] text-[var(--muted-fg)]">在线</span>
        </div>
      </header>

      <div class="flex-1 overflow-y-auto px-6 py-6">
        <div class="mx-auto flex max-w-2xl flex-col gap-5">
          <div class="flex justify-end">
            <div
              class="max-w-[80%] bg-[var(--brand-soft)] px-4 py-2.5 text-sm leading-relaxed text-[var(--fg)]"
              :style="{ borderRadius: shape.cardRadius }"
            >
              帮我把侧边栏的灰阶重新梳理下,现在 hover 和选中态几乎分不开。
            </div>
          </div>
          <div class="flex gap-3">
            <Bot class="mt-0.5 size-5 shrink-0 text-[var(--muted-fg)]" />
            <div class="flex-1 text-sm leading-relaxed text-[var(--fg)]">
              问题主要三点:灰阶混了冷紫和死中性两种家族、可用表面挤在很窄的亮度带里、卡片比背景还亮导致层级反了。
              <span class="text-[var(--muted-fg)]">点左下角的 + 试试,菜单/按钮/输入框都是真的,跟着上面的形状切换实时变。</span>
            </div>
          </div>
        </div>
      </div>

      <!-- input dock — REAL: Plus opens a menu, send is live -->
      <div class="px-6 pb-6">
        <div class="relative mx-auto max-w-2xl">
          <!-- real add-menu popover -->
          <div
            v-if="menuOpen"
            class="absolute bottom-full left-0 mb-2 w-56 border border-[var(--border-role)] bg-[var(--surface)] p-1.5"
            :style="{ borderRadius: shape.cardRadius, boxShadow: shape.shadow }"
          >
            <button
              v-for="m in addMenu"
              :key="m.label"
              class="flex w-full items-center gap-2.5 px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-[var(--hover)]"
              :style="{ borderRadius: shape.radius }"
              @click="menuOpen = false"
            >
              <component
                :is="m.icon"
                class="size-4 text-[var(--muted-fg)]"
              />
              <span class="flex-1">{{ m.label }}</span>
              <ChevronRight
                v-if="m.chevron"
                class="size-4 text-[var(--muted-fg)]"
              />
            </button>
          </div>

          <div
            class="flex items-center gap-2 border border-[var(--border-role)] bg-[var(--surface)] px-2.5 py-2"
            :style="{ borderRadius: shape.inputRadius, boxShadow: shape.shadow }"
          >
            <button
              class="grid size-9 shrink-0 place-items-center text-[var(--fg)] transition-colors hover:bg-[var(--hover)]"
              :class="menuOpen ? 'bg-[var(--selected)]' : ''"
              :style="{ borderRadius: shape.inputIconRadius }"
              @click="menuOpen = !menuOpen"
            >
              <Plus class="size-5" />
            </button>
            <input
              type="text"
              placeholder="给 Atlas 发消息…"
              class="flex-1 bg-transparent text-sm text-[var(--fg)] placeholder:text-[var(--muted-fg)] focus:outline-none"
            >
            <button
              class="grid size-9 shrink-0 place-items-center bg-[var(--brand)] text-[var(--brand-foreground)] transition-colors hover:bg-[var(--brand-hover)]"
              :style="{ borderRadius: shape.inputIconRadius }"
            >
              <ArrowUp class="size-5" />
            </button>
          </div>

          <!-- button language preview -->
          <div class="mt-5 flex items-center gap-2 border-t border-[var(--border-role)] pt-5">
            <span class="mr-1 text-[11px] text-[var(--muted-fg)]">buttons</span>
            <button
              class="bg-[var(--brand)] px-3.5 py-2 text-sm font-medium text-[var(--brand-foreground)] transition-colors hover:bg-[var(--brand-hover)]"
              :style="{ borderRadius: shape.radius }"
            >
              Primary
            </button>
            <button
              class="border border-[var(--border-role)] bg-[var(--surface)] px-3.5 py-2 text-sm font-medium transition-colors hover:bg-[var(--hover)]"
              :style="{ borderRadius: shape.radius }"
            >
              Secondary
            </button>
            <button
              class="px-3.5 py-2 text-sm font-medium transition-colors hover:bg-[var(--hover)]"
              :style="{ borderRadius: shape.radius }"
            >
              Ghost
            </button>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>
