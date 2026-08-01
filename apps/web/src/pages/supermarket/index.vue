<template>
  <PageShell :title="$t('supermarket.title')">
    <template #actions>
      <Button
        variant="outline"
        as="a"
        href="https://github.com/memohai/supermarket"
        target="_blank"
        rel="noopener noreferrer"
      >
        <Github class="size-4" />
        {{ $t('supermarket.submit') }}
      </Button>
    </template>

    <div class="space-y-6">
      <!-- Search -->
      <div class="relative">
        <Search class="absolute left-3 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground" />
        <Input
          v-model="searchInput"
          :placeholder="$t(capabilitiesStore.connectors
            ? 'supermarket.searchPlaceholderWithConnectors'
            : 'supermarket.searchPlaceholder')"
          class="pl-9"
          @keydown.enter="applySearch"
        />
      </div>

      <InlineLoadingRow
        v-if="!capabilitiesStore.loaded"
        class="justify-center py-8"
      >
        {{ $t('common.loading') }}
      </InlineLoadingRow>

      <Tabs
        v-else
        v-model="activeTab"
        class="w-full"
      >
        <TabsList>
          <TabsTrigger
            v-if="capabilitiesStore.connectors"
            value="connectors"
          >
            {{ $t('supermarket.connectorsSection') }}
          </TabsTrigger>
          <TabsTrigger value="plugins">
            {{ $t('supermarket.pluginSection') }}
          </TabsTrigger>
          <TabsTrigger value="skills">
            {{ $t('supermarket.skillsSection') }}
          </TabsTrigger>
        </TabsList>

        <!-- Plugins Tab -->
        <TabsContent value="plugins">
          <InlineLoadingRow
            v-if="pluginsLoading"
            class="justify-center py-8"
          >
            {{ $t('common.loading') }}
          </InlineLoadingRow>

          <div
            v-else-if="!plugins.length"
            class="py-8 text-center text-xs text-muted-foreground"
          >
            {{ $t('supermarket.noPluginResults') }}
          </div>

          <div
            v-else
            class="grid grid-cols-1 sm:grid-cols-2 gap-4"
          >
            <PluginCard
              v-for="plugin in plugins"
              :key="plugin.id"
              :plugin="plugin"
              @install="openPluginInstall"
            />
          </div>
        </TabsContent>

        <!-- Skills Tab -->
        <TabsContent
          value="skills"
          class="space-y-4"
        >
          <SegmentedControl
            v-if="registryFilterItems.length > 1"
            :model-value="selectedRegistry"
            :items="registryFilterItems"
            :aria-label="$t('supermarket.registryFilter')"
            class="w-full sm:w-fit"
            @update:model-value="onRegistryFilterChange"
          />

          <InlineLoadingRow
            v-if="skillsLoading"
            class="justify-center py-8"
          >
            {{ $t('common.loading') }}
          </InlineLoadingRow>

          <div
            v-else-if="!skills.length"
            class="py-8 text-center text-xs text-muted-foreground"
          >
            {{ $t('supermarket.noSkillResults') }}
          </div>

          <div
            v-else
            class="grid grid-cols-1 gap-4 sm:grid-cols-2"
          >
            <SkillCard
              v-for="skill in skills"
              :key="skill.install_id"
              :skill="skill"
              @install="openSkillInstall"
            />
          </div>

          <div
            v-if="showPagination"
            class="flex justify-end gap-2"
          >
            <Button
              variant="outline"
              size="icon-sm"
              :disabled="page === 1 || skillsLoading"
              :aria-label="$t('supermarket.previousPage')"
              @click="page--"
            >
              <ChevronLeft class="size-4" />
            </Button>
            <Button
              variant="outline"
              size="icon-sm"
              :disabled="!hasNextPage || skillsLoading"
              :aria-label="$t('supermarket.nextPage')"
              @click="page++"
            >
              <ChevronRight class="size-4" />
            </Button>
          </div>
        </TabsContent>

        <TabsContent
          v-if="capabilitiesStore.connectors"
          value="connectors"
        >
          <InlineLoadingRow
            v-if="connectorsQuery.isLoading.value"
            class="justify-center py-8"
          >
            {{ $t('common.loading') }}
          </InlineLoadingRow>

          <div
            v-else-if="searchQuery && !filteredConnectors.length"
            class="py-8 text-center text-xs text-muted-foreground"
          >
            {{ $t('supermarket.noConnectorResults') }}
          </div>

          <Empty
            v-else-if="!filteredConnectors.length"
            class="py-12"
          >
            <EmptyHeader>
              <EmptyTitle>{{ $t('connectors.catalogEmptyTitle') }}</EmptyTitle>
              <EmptyDescription>{{ $t('connectors.catalogEmptyDescription') }}</EmptyDescription>
            </EmptyHeader>
          </Empty>

          <div
            v-else
            class="grid grid-cols-1 gap-4 sm:grid-cols-2"
          >
            <MarketItemCard
              v-for="connector in filteredConnectors"
              :key="connector.type"
              :name="connector.name || connector.type"
              :description="connector.description"
              :homepage="connector.homepage_url"
              @open="openConnectorConnect(connector)"
            >
              <template #leading>
                <ProviderIcon
                  :icon="connector.icon_url || ''"
                  size="20"
                  class="size-5 object-contain"
                >
                  <Plug class="size-4 text-muted-foreground" />
                </ProviderIcon>
              </template>
              <template #actions>
                <Button
                  size="sm"
                  :disabled="connector.status !== 'ready'"
                  @click="openConnectorConnect(connector)"
                >
                  {{ connector.status === 'ready'
                    ? $t('connectors.connect')
                    : $t('connectors.unavailable') }}
                </Button>
              </template>
            </MarketItemCard>
          </div>
        </TabsContent>
      </Tabs>

      <InstallPluginDialog
        v-model:open="pluginDialogOpen"
        :plugin="selectedPlugin"
        @installed="refreshAll"
      />
      <InstallSkillDialog
        v-model:open="skillDialogOpen"
        :skill="selectedSkill"
        @installed="refreshAll"
      />
      <ConnectConnectorDialog
        v-model:open="connectorDialogOpen"
        :connector="selectedConnector"
        :default-bot-id="defaultBotId"
        @connected="openBotConnectors"
      />
    </div>
  </PageShell>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useQuery } from '@pinia/colada'
import { ChevronLeft, ChevronRight, Github, Plug, Search } from 'lucide-vue-next'
import {
  Button,
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
  InlineLoadingRow,
  Input,
  PageShell,
  SegmentedControl,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  toast,
  type SegmentedItem,
} from '@felinic/ui'
import {
  getConnectorsCatalog,
  getSupermarketPlugins,
  getSupermarketRegistries,
  getSupermarketSkills,
  type ConnectitConnector,
  type HandlersSupermarketCatalogSkill,
  type HandlersSupermarketPluginEntry,
  type HandlersSupermarketRegistry,
} from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import PluginCard from './components/plugin-card.vue'
import SkillCard from './components/skill-card.vue'
import InstallPluginDialog from './components/install-plugin-dialog.vue'
import InstallSkillDialog from './components/install-skill-dialog.vue'
import ConnectConnectorDialog from './components/connect-connector-dialog.vue'
import MarketItemCard from './components/market-item-card.vue'
import ProviderIcon from '@/components/provider-icon/index.vue'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'
import { useCapabilitiesStore } from '@/store/capabilities'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const capabilitiesStore = useCapabilitiesStore()
const tabParam = useSyncedQueryParam('tab', '')
const pageSize = 50
const allRegistriesValue = 'all'
// Settings pages are KeepAlive-cached and share the `tab` query key, so a
// foreign value (e.g. bot detail's ?tab=memory) can land in the synced param
// while this page is deactivated. Render anything outside this page's own
// tabs as the capability-dependent default; writes go back through the synced param.
const activeTab = computed({
  get: () => {
    const valid = capabilitiesStore.connectors
      ? ['connectors', 'plugins', 'skills']
      : ['plugins', 'skills']
    const fallback = capabilitiesStore.connectors ? 'connectors' : 'plugins'
    return valid.includes(tabParam.value) ? tabParam.value : fallback
  },
  set: (value: string) => {
    tabParam.value = value
  },
})

const searchInput = ref('')
const searchQuery = ref('')
const page = ref(1)
const total = ref(0)
const selectedRegistry = ref(allRegistriesValue)
const plugins = ref<HandlersSupermarketPluginEntry[]>([])
const skills = ref<HandlersSupermarketCatalogSkill[]>([])
const registries = ref<HandlersSupermarketRegistry[]>([])
const pluginsLoading = ref(false)
const skillsLoading = ref(false)

const pluginDialogOpen = ref(false)
const skillDialogOpen = ref(false)
const selectedPlugin = ref<HandlersSupermarketPluginEntry | null>(null)
const selectedSkill = ref<HandlersSupermarketCatalogSkill | null>(null)
const connectorDialogOpen = ref(false)
const selectedConnector = ref<ConnectitConnector | null>(null)

const hasNextPage = computed(() => page.value * pageSize < total.value)
const showPagination = computed(() => page.value > 1 || hasNextPage.value)
const registryFilterItems = computed<SegmentedItem[]>(() => [
  { value: allRegistriesValue, label: t('supermarket.allRegistries') },
  ...registries.value
    .filter((registry): registry is HandlersSupermarketRegistry & { id: string } => !!registry.id)
    .map(registry => ({
      value: registry.id,
      label: registry.name || registry.id,
    })),
])

const defaultBotId = computed(() => {
  const value = route.query.botId
  return typeof value === 'string' ? value : ''
})

const connectorsQuery = useQuery({
  key: () => ['connectors-catalog'],
  query: async () => {
    const { data } = await getConnectorsCatalog({ throwOnError: true })
    return data
  },
  enabled: () => capabilitiesStore.connectors,
})

const filteredConnectors = computed(() => {
  const query = searchQuery.value.toLowerCase()
  const connectors = connectorsQuery.data.value ?? []
  if (!query) return connectors
  return connectors.filter(connector =>
    [connector.name, connector.type, connector.description, ...(connector.categories ?? [])]
      .some(value => value?.toLowerCase().includes(query)),
  )
})

watch(connectorsQuery.error, error => {
  if (error) toast.error(resolveApiErrorMessage(error, t('supermarket.loadError')))
})

watch(
  () => [capabilitiesStore.loaded, capabilitiesStore.connectors] as const,
  ([loaded, connectors]) => {
    // Normalize the URL param only while this page owns the current route —
    // firing while KeepAlive-deactivated would rewrite another page's ?tab.
    if (loaded && !connectors && route.name === 'supermarket' && tabParam.value === 'connectors') {
      tabParam.value = 'plugins'
    }
  },
  { immediate: true },
)

onMounted(() => {
  void capabilitiesStore.load()
})

function applySearch() {
  const nextQuery = searchInput.value.trim()
  if (searchQuery.value === nextQuery) {
    page.value = 1
    void refreshAll()
    return
  }
  searchQuery.value = nextQuery
}

let searchDebounce: ReturnType<typeof setTimeout> | undefined
watch(searchInput, () => {
  clearTimeout(searchDebounce)
  searchDebounce = setTimeout(applySearch, 300)
})

function onRegistryFilterChange(value: string | number) {
  const next = String(value)
  if (selectedRegistry.value === next) return
  selectedRegistry.value = next
  if (page.value !== 1) {
    page.value = 1
    return
  }
  void loadSkills()
}

function openPluginInstall(plugin: HandlersSupermarketPluginEntry) {
  selectedPlugin.value = plugin
  pluginDialogOpen.value = true
}

function openSkillInstall(skill: HandlersSupermarketCatalogSkill) {
  selectedSkill.value = skill
  skillDialogOpen.value = true
}

function openConnectorConnect(connector: ConnectitConnector) {
  if (connector.status !== 'ready') return
  selectedConnector.value = connector
  connectorDialogOpen.value = true
}

function openBotConnectors(botId: string) {
  void router.push({
    name: 'bot-detail',
    params: { botName: botId },
    query: { tab: 'connectors' },
  })
}

async function loadPlugins() {
  pluginsLoading.value = true
  try {
    const { data } = await getSupermarketPlugins({
      query: {
        q: searchQuery.value || undefined,
        limit: 50,
      },
      throwOnError: true,
    })
    plugins.value = data.data ?? []
  } catch (error) {
    plugins.value = []
    toast.error(resolveApiErrorMessage(error, t('supermarket.loadError')))
  } finally {
    pluginsLoading.value = false
  }
}

async function loadRegistries() {
  try {
    const { data } = await getSupermarketRegistries({ throwOnError: true })
    registries.value = data.data ?? []
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('supermarket.loadError')))
  }
}

async function loadSkills() {
  skillsLoading.value = true
  try {
    const { data } = await getSupermarketSkills({
      query: {
        q: searchQuery.value || undefined,
        registry: selectedRegistry.value === allRegistriesValue
          ? undefined
          : selectedRegistry.value,
        page: page.value,
        limit: pageSize,
        sort: 'package',
      },
      throwOnError: true,
    })
    skills.value = data.data ?? []
    total.value = data.total ?? 0
  } catch (error) {
    skills.value = []
    total.value = 0
    toast.error(resolveApiErrorMessage(error, t('supermarket.loadError')))
  } finally {
    skillsLoading.value = false
  }
}

function refreshAll() {
  void loadPlugins()
  void loadSkills()
}

watch(searchQuery, () => {
  if (page.value !== 1) {
    page.value = 1
    return
  }
  refreshAll()
})

watch(page, loadSkills)

void loadRegistries()
refreshAll()
</script>
