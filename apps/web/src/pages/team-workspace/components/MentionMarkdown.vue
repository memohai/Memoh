<template>
  <div
    ref="hostRef"
    :class="hostClass"
  >
    <MarkdownPreview
      :content="content"
      :class="markdownClass"
    />
  </div>
</template>

<script setup lang="ts">
import type {
  BotsBot,
  HandlersMemberResponse,
} from '@memohai/sdk'
import {
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from 'vue'
import MarkdownPreview from '@/components/markdown-preview/index.vue'

const props = withDefaults(defineProps<{
  content: string
  members: HandlersMemberResponse[]
  bots: BotsBot[]
  hostClass?: string
  markdownClass?: string
}>(), {
  hostClass: '',
  markdownClass: '',
})

const emit = defineEmits<{
  open: [member: HandlersMemberResponse]
}>()

const hostRef = ref<HTMLElement | null>(null)

// 与后端 / agent 的 mention 规则一致：
//  - `@"name with spaces"` 引号包裹
//  - `@unquoted` 单 token（Unicode letter/digit/_/-/.）
// 同时在前置位用 lookbehind 排除上一字符是字母/数字的情况，避免误伤
// `bob@example.com` 等场景。
const MENTION_RE = /(^|[^\p{L}\p{N}])(@"([^"]+)"|@([\p{L}\p{N}_\-.]+))/gu

watch(
  [() => props.content, () => props.members, () => props.bots],
  async () => {
    await nextTick()
    if (hostRef.value) processMentions(hostRef.value)
  },
  { immediate: true, flush: 'post' },
)

onMounted(() => {
  hostRef.value?.addEventListener('click', onHostClick)
  // 渲染完成后再跑一次，覆盖首次 mounted 时 watch.flush 时序差异。
  if (hostRef.value) processMentions(hostRef.value)
})

onBeforeUnmount(() => {
  hostRef.value?.removeEventListener('click', onHostClick)
})

function processMentions(root: HTMLElement) {
  // 收集需要替换的文本节点，跳过 <code>/<pre> 与已经处理过的 .mention-pill 内部。
  const targets: Text[] = []
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      let parent = node.parentElement
      while (parent && parent !== root) {
        const tag = parent.tagName
        if (tag === 'CODE' || tag === 'PRE' || tag === 'A') {
          return NodeFilter.FILTER_REJECT
        }
        if (parent.classList?.contains('mention-pill')) {
          return NodeFilter.FILTER_REJECT
        }
        parent = parent.parentElement
      }
      const text = node.nodeValue ?? ''
      return text.includes('@') ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT
    },
  })
  let cursor = walker.nextNode() as Text | null
  while (cursor) {
    targets.push(cursor)
    cursor = walker.nextNode() as Text | null
  }
  for (const node of targets) {
    replaceTextNode(node)
  }
}

function replaceTextNode(node: Text) {
  const text = node.nodeValue ?? ''
  MENTION_RE.lastIndex = 0
  let match: RegExpExecArray | null
  const ranges: Array<{ start: number, end: number, name: string }> = []
  while ((match = MENTION_RE.exec(text)) !== null) {
    const leading = match[1] ?? ''
    const start = (match.index ?? 0) + leading.length
    const end = start + (match[2]?.length ?? 0)
    const name = match[3] || match[4] || ''
    ranges.push({ start, end, name })
  }
  if (ranges.length === 0) return

  const frag = document.createDocumentFragment()
  let writeCursor = 0
  for (const range of ranges) {
    if (range.start > writeCursor) {
      frag.appendChild(document.createTextNode(text.slice(writeCursor, range.start)))
    }
    frag.appendChild(buildMentionPill(range.name))
    writeCursor = range.end
  }
  if (writeCursor < text.length) {
    frag.appendChild(document.createTextNode(text.slice(writeCursor)))
  }
  node.parentNode?.replaceChild(frag, node)
}

function buildMentionPill(name: string): HTMLElement {
  const member = findMember(name)
  const pill = document.createElement('span')
  // align-middle + h-5 + leading-none 是为了让 pill 在 24px 行高的中文段落里垂直居中，
  // 不再依赖 translate-y 偏移补偿基线漂移。
  const baseClass = 'mention-pill inline-flex items-center align-middle gap-1 rounded-full h-5 px-1.5 text-xs font-medium leading-none'
  if (member) {
    pill.className = `${baseClass} mention-pill--member border bg-muted text-foreground hover:bg-accent cursor-pointer`
    pill.setAttribute('role', 'button')
    pill.setAttribute('tabindex', '0')
    pill.dataset.memberId = member.id ?? ''

    const avatar = document.createElement('span')
    avatar.className = 'mention-pill__avatar inline-flex size-3.5 shrink-0 items-center justify-center overflow-hidden rounded-full bg-accent text-[8px] font-medium text-muted-foreground'
    const src = memberAvatar(member)
    if (src) {
      const img = document.createElement('img')
      img.src = src
      img.alt = memberLabel(member)
      img.className = 'size-full rounded-full object-cover'
      avatar.appendChild(img)
    }
    else {
      avatar.textContent = initials(memberLabel(member))
    }
    pill.appendChild(avatar)

    const label = document.createElement('span')
    label.textContent = `@${memberLabel(member)}`
    pill.appendChild(label)
  }
  else {
    pill.className = `${baseClass} mention-pill--plain bg-primary/10 text-primary`
    pill.textContent = `@${name}`
  }
  return pill
}

function onHostClick(ev: MouseEvent) {
  const target = (ev.target as HTMLElement | null)?.closest('.mention-pill--member') as HTMLElement | null
  if (!target) return
  const memberId = target.dataset.memberId
  if (!memberId) return
  const member = props.members.find((m) => m.id === memberId)
  if (member) emit('open', member)
}

function findMember(name: string) {
  const key = normalizeMentionName(name)
  return props.members.find((member) => {
    const candidates = [
      member.display_name,
      member.bot_id,
      member.user_id,
    ].filter(Boolean).map((value) => normalizeMentionName(value ?? ''))
    return candidates.includes(key)
  })
}

function normalizeMentionName(value: string) {
  return value.trim().toLowerCase()
}

function memberLabel(member: HandlersMemberResponse) {
  return member.display_name || member.bot_id || member.user_id || 'Unknown'
}

function memberAvatar(member: HandlersMemberResponse) {
  if (!member.bot_id) return ''
  return props.bots.find((bot) => bot.id === member.bot_id)?.avatar_url ?? ''
}

function initials(name: string): string {
  return name
    .split(/[\s_-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((word) => word[0])
    .join('')
    .toUpperCase() || '?'
}
</script>
