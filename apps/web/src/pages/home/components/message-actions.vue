<template>
  <!-- One reserved row under every turn. The height is always present so the
       layout never jumps; only visibility toggles. While the turn is still
       streaming the row stays fully hidden (no hover reveal) — actions on an
       in-flight answer are meaningless. The latest FINISHED turn keeps it
       visible; every other turn reveals it on pointer/focus within the turn's
       hover scope (group/msg, set on the message content wrapper).

       Alignment differs by role on purpose:
       - assistant (`start`): the hover hit-area overflows the text's left edge
         a little (`-ml-1.5`), but NOT by the full button padding — the glyph then
         sits a few px RIGHT of the text's left edge, which reads as visually
         settled rather than a glyph hard-pinned to the margin.
       - user (`end`): the hover hit-area's RIGHT edge meets the bubble's right
         edge (`justify-end`, no negative margin), so the cluster lines up with
         the bubble it belongs to. -->
  <div
    class="chat-message-meta flex h-8 items-center gap-0.5 transition-opacity duration-150 motion-reduce:transition-none"
    :class="[
      align === 'end' ? 'justify-end' : 'justify-start -ml-1.5',
      streaming
        ? 'opacity-0 pointer-events-none'
        : persistent
          ? 'opacity-100'
          : 'opacity-0 pointer-events-none group-hover/msg:opacity-100 group-hover/msg:pointer-events-auto focus-within:opacity-100 focus-within:pointer-events-auto',
    ]"
  >
    <!-- The tooltip is owned entirely by its trigger (the icon button): moving
         the pointer onto the tooltip itself must NOT keep it alive. Without
         this, the hover row could fade out while a stranded tooltip lingered
         over an already-gone button. -->
    <TooltipProvider
      :delay-duration="0"
      :disable-hoverable-content="true"
    >
      <!-- Copy — shared by both roles. The clipboard glyph is mirrored on X so
           the two stacked squares read top-left over bottom-right, matching the
           composer's copy affordance. -->
      <Tooltip>
        <TooltipTrigger as-child>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            :class="actionIconClass"
            :aria-label="copyLabel"
            @click="$emit('copy')"
          >
            <CheckDrawIcon
              v-if="copied"
              class="size-[18px]"
              :stroke-width="1.75"
            />
            <CopyConnectedIcon
              v-else
              class="size-[18px] -scale-x-100"
              :stroke-width="1.75"
            />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="bottom">
          {{ copyLabel }}
        </TooltipContent>
      </Tooltip>

      <Tooltip v-if="canRewriteAction">
        <TooltipTrigger as-child>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            :class="actionIconClass"
            :aria-label="t('chat.actions.edit')"
            @click="$emit('rewrite')"
          >
            <PencilLine class="size-[18px]" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="bottom">
          {{ t('chat.actions.edit') }}
        </TooltipContent>
      </Tooltip>

      <Tooltip v-if="canForkAction">
        <TooltipTrigger as-child>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            :class="actionIconClass"
            :aria-label="t('chat.actions.fork')"
            @click="$emit('fork')"
          >
            <GitFork class="size-[18px]" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="bottom">
          {{ t('chat.actions.fork') }}
        </TooltipContent>
      </Tooltip>

      <!-- ASSISTANT role: "more" menu — timestamp only for now. Share, retry,
           read-aloud, and user edit are intentionally withheld until wired. -->
      <template v-if="role === 'assistant'">
        <DropdownMenu>
          <DropdownMenuTrigger as-child>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              :class="actionIconClass"
              :aria-label="t('chat.actions.more')"
            >
              <DotsIcon class="size-[18px]" />
            </Button>
          </DropdownMenuTrigger>
          <!-- Opens UPWARD: the action bar sits right above the composer, so a
               downward menu would land on top of the input. -->
          <DropdownMenuContent
            side="top"
            align="start"
            class="min-w-[12rem]"
          >
            <DropdownMenuLabel
              class="text-label font-normal text-muted-foreground"
              :title="fullTime"
            >
              {{ menuTime }}
            </DropdownMenuLabel>
          </DropdownMenuContent>
        </DropdownMenu>
      </template>
    </TooltipProvider>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import CopyConnectedIcon from './copy-connected-icon.vue'
import CheckDrawIcon from './check-draw-icon.vue'
import DotsIcon from './dots-icon.vue'
import { GitFork, PencilLine } from 'lucide-vue-next'
import {
  Button,
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuLabel,
} from '@memohai/ui'

const props = defineProps<{
  copyText: string
  role: 'user' | 'assistant'
  menuTime?: string
  fullTime?: string
  persistent?: boolean
  streaming?: boolean
  align?: 'start' | 'end'
  copied?: boolean
  canFork?: boolean
  canRewrite?: boolean
}>()

const { t } = useI18n()

defineEmits<{
  copy: []
  fork: []
  rewrite: []
}>()

// Rest tint sits just shy of the muted text — nudged ~15% toward the body
// foreground so the icons read a touch more present than plain muted-foreground,
// without going to full foreground. color-mix keeps it correct in both themes.
const actionIconClass
  = 'text-[color-mix(in_oklab,var(--muted-foreground),var(--foreground)_15%)] hover:text-foreground'

const copied = computed(() => props.copied === true)
const canForkAction = computed(() => props.canFork === true)
const canRewriteAction = computed(() => props.canRewrite === true)
const copyLabel = computed(() => (copied.value ? t('chat.actions.copied') : t('chat.actions.copy')))
</script>
