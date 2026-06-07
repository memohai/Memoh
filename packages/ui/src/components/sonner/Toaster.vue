<script lang="ts" setup>
import type { Component } from 'vue'
import { CircleCheckIcon, CircleXIcon, InfoIcon, TriangleAlertIcon, XIcon } from 'lucide-vue-next'
import { computed, onBeforeUnmount, onMounted } from 'vue'
import { Button } from '#/components/button'
import { cn } from '#/lib/utils'
import { dismiss, pauseAll, resumeAll, toast as toastApi, toasts, type ToastVariant } from './toast'

export type ToasterPosition =
  | 'top-right'
  | 'top-left'
  | 'top-center'
  | 'bottom-right'
  | 'bottom-left'
  | 'bottom-center'

export interface ToasterProps {
  /** Screen corner the column docks to. We default to top-right: out of the
   *  centre of vision, so a card can carry more text without blocking work. */
  position?: ToasterPosition
  class?: string
}

const props = withDefaults(defineProps<ToasterProps>(), {
  position: 'top-right',
})

// Re-expose the imperative API on the component instance for the rare template
// ref consumer; the canonical entry stays the `toast` export from the package.
defineExpose({ toast: toastApi, dismiss })

const semanticIcon: Partial<Record<ToastVariant, Component>> = {
  success: CircleCheckIcon,
  error: CircleXIcon,
  warning: TriangleAlertIcon,
  info: InfoIcon,
}

const isTop = computed(() => props.position.startsWith('top'))

// Newest sits closest to the screen edge: on top of the list for a top dock, at
// the bottom for a bottom dock. The store is chronological; we only flip the view.
const ordered = computed(() => (isTop.value ? [...toasts].reverse() : toasts))

function onVisibilityChange() {
  if (document.hidden) pauseAll()
  else resumeAll()
}

onMounted(() => document.addEventListener('visibilitychange', onVisibilityChange))
onBeforeUnmount(() => document.removeEventListener('visibilitychange', onVisibilityChange))
</script>

<template>
  <Teleport to="body">
    <ol
      :class="cn('memoh-toaster', `memoh-toaster--${props.position}`, props.class)"
      role="region"
      aria-label="Notifications"
      tabindex="-1"
    >
      <TransitionGroup name="memoh-toast">
        <!--
          Two layout variants — selected purely from data, no CSS hacks needed:

          single  : no description, action optional  → flat row, everything center-aligned
          rich    : has description                  → icon+× pin to first line (flex-start);
                                                       body is a column: title → desc → action?
        -->
        <li
          v-for="t in ordered"
          :key="t.id"
          :class="cn('memoh-toast', t.description ? 'memoh-toast--rich' : 'memoh-toast--single')"
          :data-variant="t.variant"
          role="status"
          aria-live="polite"
          aria-atomic="true"
          @mouseenter="pauseAll"
          @mouseleave="resumeAll"
          @focusin="pauseAll"
          @focusout="resumeAll"
        >
          <!-- ── SINGLE LINE ─────────────────────────────────────────────────────
               Everything in one flat row, vertically centered.
               icon · title · (action) · ×
          ──────────────────────────────────────────────────────────────────────── -->
          <template v-if="!t.description">
            <component
              :is="semanticIcon[t.variant]"
              v-if="semanticIcon[t.variant]"
              class="memoh-toast__icon size-4"
              aria-hidden="true"
            />
            <span class="memoh-toast__title memoh-toast__title--grow">{{ t.title }}</span>
            <Button
              v-if="t.action"
              variant="outline"
              size="sm"
              class="memoh-toast__action"
              @click="() => { t.action!.onClick(); dismiss(t.id) }"
            >
              {{ t.action.label }}
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              class="memoh-toast__close"
              aria-label="Dismiss notification"
              @click="dismiss(t.id)"
            >
              <XIcon class="size-4" />
            </Button>
          </template>

          <!-- ── RICH (has description) ─────────────────────────────────────────
               icon and × pin to the top (first-line height).
               Action always goes BELOW the desc — never inline with the title.
               icon · title               · ×
                       desc
                       (action)
          ──────────────────────────────────────────────────────────────────────── -->
          <template v-else>
            <component
              :is="semanticIcon[t.variant]"
              v-if="semanticIcon[t.variant]"
              class="memoh-toast__icon size-4 memoh-toast__icon--rich"
              aria-hidden="true"
            />
            <div class="memoh-toast__body">
              <span class="memoh-toast__title">{{ t.title }}</span>
              <p class="memoh-toast__desc">
                {{ t.description }}
              </p>
              <Button
                v-if="t.action"
                variant="outline"
                size="sm"
                class="memoh-toast__action"
                @click="() => { t.action!.onClick(); dismiss(t.id) }"
              >
                {{ t.action.label }}
              </Button>
            </div>
            <Button
              variant="ghost"
              size="icon-sm"
              class="memoh-toast__close memoh-toast__close--rich"
              aria-label="Dismiss notification"
              @click="dismiss(t.id)"
            >
              <XIcon class="size-4" />
            </Button>
          </template>
        </li>
      </TransitionGroup>
    </ol>
  </Teleport>
</template>
