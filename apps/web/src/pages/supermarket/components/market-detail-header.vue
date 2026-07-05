<template>
  <!-- 超市详情页(plugin/skill)共用的页头:返回/安装操作行 + 图标盒 + 标题 + 标签。
       抽取前两页各复制一份同构结构;图标盒统一带 overflow-hidden(plugin 版原有,
       skill 版漏掉——外链图标图片可能非方形,需裁进圆角盒)。
       标签容器与原实现一致地始终渲染(即使为空),保持 space-y-4 的节奏不变。 -->
  <div>
    <div class="mb-6 flex items-center justify-between gap-3">
      <Button
        variant="ghost"
        size="sm"
        class="-ml-2"
        @click="$emit('back')"
      >
        <ArrowLeft class="size-4" />
        {{ $t('common.back') }}
      </Button>
      <Button
        size="sm"
        @click="$emit('install')"
      >
        <Download class="size-4" />
        {{ $t('supermarket.installToBot') }}
      </Button>
    </div>

    <header class="space-y-4">
      <div class="flex items-start gap-4">
        <div :class="iconBoxClass">
          <slot name="icon" />
        </div>
        <div class="min-w-0 flex-1">
          <h1 class="break-words text-3xl font-semibold leading-tight">
            {{ name }}
          </h1>
        </div>
      </div>

      <div class="flex flex-wrap gap-1.5">
        <Badge
          v-for="tag in tags"
          :key="tag"
          variant="secondary"
          size="sm"
        >
          {{ tag }}
        </Badge>
      </div>
    </header>
  </div>
</template>

<script setup lang="ts">
import { ArrowLeft, Download } from 'lucide-vue-next'
import { Badge, Button } from '@memohai/ui'

// 抽取前两页图标盒原样带的浅投影,随形状一起搬入 owner
const iconBoxClass = 'flex size-16 shrink-0 items-center justify-center overflow-hidden rounded-md border bg-background shadow-sm' /* ui-allow-style */

defineProps<{
  name?: string
  tags?: string[]
}>()

defineEmits<{
  back: []
  install: []
}>()
</script>
