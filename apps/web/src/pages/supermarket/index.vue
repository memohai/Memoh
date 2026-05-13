<template>
  <div class="px-4 pt-2 pb-10 md:px-6 md:pt-4 md:pb-12 max-w-6xl mx-auto space-y-6">
    <!-- Header: Title + Submit Button -->
    <div class="flex items-center justify-between">
      <h1 class="text-lg font-semibold">
        {{ $t('supermarket.title') }}
      </h1>
      <Button
        variant="outline"
        size="sm"
        as="a"
        href="https://github.com/memohai/supermarket"
        target="_blank"
        rel="noopener noreferrer"
      >
        <Github class="size-4" />
        {{ $t('supermarket.submit') }}
      </Button>
    </div>

    <!-- Search -->
    <div class="space-y-4">
      <div class="flex items-center gap-2">
        <div class="relative flex-1">
          <Search class="absolute left-3 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground" />
          <Input
            v-model="searchInput"
            :placeholder="$t('supermarket.searchPlaceholder')"
            class="pl-9"
            @keydown.enter="applySearch"
          />
        </div>
      </div>

      <!-- Active tag filter -->
      <div
        v-if="activeTag"
        class="flex items-center gap-2"
      >
        <Badge
          variant="secondary"
          class="gap-1"
        >
          {{ $t('supermarket.filterByTag', { tag: activeTag }) }}
          <button
            class="ml-1 hover:text-destructive"
            @click="clearTag"
          >
            <X class="size-3" />
          </button>
        </Badge>
      </div>
    </div>

    <!-- Tabs: Skills / MCP -->
    <Tabs
      default-value="skills"
      class="w-full"
    >
      <TabsList>
        <TabsTrigger value="skills">
          {{ $t('supermarket.skillsSection') }}
        </TabsTrigger>
        <TabsTrigger value="mcp">
          {{ $t('supermarket.mcpSection') }}
        </TabsTrigger>
      </TabsList>

      <!-- Skills Tab -->
      <TabsContent value="skills">
        <div
          v-if="skillsLoading"
          class="flex items-center justify-center py-8 text-xs text-muted-foreground"
        >
          <Spinner class="mr-2" />
          {{ $t('common.loading') }}
        </div>

        <div
          v-else-if="!skills.length"
          class="py-8 text-center text-xs text-muted-foreground"
        >
          {{ $t('supermarket.noSkillResults') }}
        </div>

        <div
          v-else
          class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4"
        >
          <SkillCard
            v-for="skill in skills"
            :key="skill.id"
            :skill="skill"
            @tag-click="setTag"
            @install="openSkillInstall"
          />
        </div>
      </TabsContent>

      <!-- MCP Tab -->
      <TabsContent value="mcp">
        <div
          v-if="mcpLoading"
          class="flex items-center justify-center py-8 text-xs text-muted-foreground"
        >
          <Spinner class="mr-2" />
          {{ $t('common.loading') }}
        </div>

        <div
          v-else-if="!mcps.length"
          class="py-8 text-center text-xs text-muted-foreground"
        >
          {{ $t('supermarket.noMcpResults') }}
        </div>

        <div
          v-else
          class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4"
        >
          <McpCard
            v-for="mcp in mcps"
            :key="mcp.id"
            :mcp="mcp"
            @tag-click="setTag"
            @install="openMcpInstall"
          />
        </div>
      </TabsContent>
    </Tabs>

    <!-- Install Dialogs -->
    <InstallMcpDialog
      v-model:open="mcpDialogOpen"
      :mcp="selectedMcp"
    />
    <InstallSkillDialog
      v-model:open="skillDialogOpen"
      :skill="selectedSkill"
      @installed="refreshAll"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Search, X, Github } from 'lucide-vue-next'
import { Input, Badge, Spinner, Button, Tabs, TabsList, TabsTrigger, TabsContent } from '@memohai/ui'
import {
  getSupermarketMcps,
  getSupermarketSkills,
  type HandlersSupermarketMcpEntry,
  type HandlersSupermarketSkillEntry,
} from '@memohai/sdk'
import { toast } from 'vue-sonner'
import { resolveApiErrorMessage } from '@/utils/api-error'
import McpCard from './components/mcp-card.vue'
import SkillCard from './components/skill-card.vue'
import InstallMcpDialog from './components/install-mcp-dialog.vue'
import InstallSkillDialog from './components/install-skill-dialog.vue'

const { t } = useI18n()

const searchInput = ref('')
const searchQuery = ref('')
const activeTag = ref('')

const mcps = ref<HandlersSupermarketMcpEntry[]>([])
const skills = ref<HandlersSupermarketSkillEntry[]>([])
const mcpLoading = ref(false)
const skillsLoading = ref(false)

const mcpDialogOpen = ref(false)
const skillDialogOpen = ref(false)
const selectedMcp = ref<HandlersSupermarketMcpEntry | null>(null)
const selectedSkill = ref<HandlersSupermarketSkillEntry | null>(null)

function applySearch() {
  searchQuery.value = searchInput.value.trim()
}

let searchDebounce: ReturnType<typeof setTimeout> | undefined
watch(searchInput, () => {
  clearTimeout(searchDebounce)
  searchDebounce = setTimeout(() => {
    searchQuery.value = searchInput.value.trim()
  }, 300)
})

function setTag(tag: string) {
  activeTag.value = tag
}

function clearTag() {
  activeTag.value = ''
}

function openMcpInstall(mcp: HandlersSupermarketMcpEntry) {
  selectedMcp.value = mcp
  mcpDialogOpen.value = true
}

function openSkillInstall(skill: HandlersSupermarketSkillEntry) {
  selectedSkill.value = skill
  skillDialogOpen.value = true
}

async function loadMcps() {
  mcpLoading.value = true
  try {
    const { data } = await getSupermarketMcps({
      query: {
        q: searchQuery.value || undefined,
        tag: activeTag.value || undefined,
        limit: 50,
      },
      throwOnError: true,
    })
    mcps.value = data.data ?? []
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('supermarket.loadError')))
  } finally {
    mcpLoading.value = false
  }
}

async function loadSkills() {
  skillsLoading.value = true
  try {
    const { data } = await getSupermarketSkills({
      query: {
        q: searchQuery.value || undefined,
        tag: activeTag.value || undefined,
        limit: 50,
      },
      throwOnError: true,
    })
    skills.value = data.data ?? []
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('supermarket.loadError')))
  } finally {
    skillsLoading.value = false
  }
}

function refreshAll() {
  loadMcps()
  loadSkills()
}

watch([searchQuery, activeTag], () => {
  loadMcps()
  loadSkills()
}, { immediate: true })
</script>
