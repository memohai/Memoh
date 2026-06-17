<template>
  <div class="pointer-events-none absolute inset-x-0 top-0 z-30 h-9">
    <div
      class="pointer-events-auto absolute left-0 top-0 flex h-9 items-center gap-0.5 pr-2 transition-[padding,width] duration-300 ease-[cubic-bezier(0.32,0.72,0,1)] [-webkit-app-region:drag]"
      :class="shouldReserveTrafficLight ? 'pl-[76px]' : 'pl-2'"
    >
      <Button
        variant="ghost"
        size="icon-sm"
        class="pointer-events-auto size-7 rounded-full text-muted-foreground hover:text-foreground [-webkit-app-region:no-drag]"
        :title="workbenchOpen ? t('chat.topBar.hideWorkbench') : t('chat.topBar.showWorkbench')"
        :aria-label="workbenchOpen ? t('chat.topBar.hideWorkbench') : t('chat.topBar.showWorkbench')"
        :aria-pressed="workbenchOpen"
        @click="workspaceTabs.toggleWorkbench()"
      >
        <PanelLeftClose
          v-if="workbenchOpen"
          :stroke-width="1.75"
          class="size-4"
        />
        <PanelLeftOpen
          v-else
          :stroke-width="1.75"
          class="size-4"
        />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        class="pointer-events-auto size-7 rounded-full text-muted-foreground hover:text-foreground [-webkit-app-region:no-drag]"
        :title="t('chat.topBar.goBack')"
        :aria-label="t('chat.topBar.goBack')"
        @click="router.go(-1)"
      >
        <ChevronLeft
          :stroke-width="1.75"
          class="size-4"
        />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        class="pointer-events-auto size-7 rounded-full text-muted-foreground hover:text-foreground [-webkit-app-region:no-drag]"
        :title="t('chat.topBar.goForward')"
        :aria-label="t('chat.topBar.goForward')"
        @click="router.go(1)"
      >
        <ChevronRight
          :stroke-width="1.75"
          class="size-4"
        />
      </Button>
    </div>

    <div
      class="pointer-events-auto absolute right-0 top-0 flex h-9 items-center justify-end pr-2 [-webkit-app-region:drag]"
    >
      <Button
        variant="ghost"
        size="icon-sm"
        class="pointer-events-auto size-7 rounded-full text-muted-foreground hover:text-foreground [-webkit-app-region:no-drag]"
        :title="branchSidebarOpen ? t('chat.topBar.hideBranchSidebar') : t('chat.topBar.showBranchSidebar')"
        :aria-label="branchSidebarOpen ? t('chat.topBar.hideBranchSidebar') : t('chat.topBar.showBranchSidebar')"
        :aria-pressed="branchSidebarOpen"
        @click="workspaceTabs.toggleBranchSidebar()"
      >
        <PanelRightClose
          v-if="branchSidebarOpen"
          :stroke-width="1.75"
          class="size-4"
        />
        <PanelRightOpen
          v-else
          :stroke-width="1.75"
          class="size-4"
        />
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  ChevronLeft,
  ChevronRight,
  PanelLeftClose,
  PanelLeftOpen,
  PanelRightClose,
  PanelRightOpen,
} from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'

const props = defineProps<{
  macTrafficReserve?: boolean
}>()

const { t } = useI18n()
const router = useRouter()
const workspaceTabs = useWorkspaceTabsStore()
const { branchSidebarOpen, workbenchOpen } = storeToRefs(workspaceTabs)

const shouldReserveTrafficLight = computed(() =>
  props.macTrafficReserve === true && !workbenchOpen.value,
)
</script>
