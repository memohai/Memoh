<template>
  <section class="max-w-7xl mx-auto px-4 pt-2 pb-10 md:px-6 md:pt-4 md:pb-12">
    <div class="max-w-3xl mx-auto space-y-6">
      <div>
        <h1 class="text-lg font-semibold">
          {{ t('aiDevelopmentEngine.title') }}
        </h1>
        <p class="mt-1 text-xs text-muted-foreground">
          {{ t('aiDevelopmentEngine.description') }}
        </p>
      </div>

      <Card>
        <CardHeader>
          <div class="flex items-center gap-2">
            <Cpu class="size-4 text-muted-foreground" />
            <CardTitle class="text-sm">
              {{ t('aiDevelopmentEngine.connectionStatus') }}
            </CardTitle>
          </div>
          <CardDescription class="text-xs">
            {{ displayName }}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div
            v-if="loading"
            class="flex items-center gap-2 text-xs text-muted-foreground"
          >
            <Spinner class="size-3.5" />
            {{ t('aiDevelopmentEngine.loading') }}
          </div>
          <div
            v-else-if="loadError"
            class="text-xs text-destructive"
          >
            {{ loadError }}
          </div>
          <div
            v-else
            class="grid gap-3 sm:grid-cols-3"
          >
            <div class="rounded-md border p-3">
              <div class="text-xs text-muted-foreground">
                {{ t('aiDevelopmentEngine.connectionStatus') }}
              </div>
              <div class="mt-2 flex items-center gap-2">
                <CircleOff class="size-3.5 text-muted-foreground" />
                <span class="text-sm font-medium">{{ connectionStatusLabel }}</span>
              </div>
            </div>
            <div class="rounded-md border p-3">
              <div class="text-xs text-muted-foreground">
                {{ t('aiDevelopmentEngine.authorizationStatus') }}
              </div>
              <div class="mt-2 flex items-center gap-2">
                <CircleOff class="size-3.5 text-muted-foreground" />
                <span class="text-sm font-medium">{{ authStatusLabel }}</span>
              </div>
            </div>
            <div class="rounded-md border p-3">
              <div class="text-xs text-muted-foreground">
                {{ t('aiDevelopmentEngine.displayName') }}
              </div>
              <div class="mt-2 text-sm font-medium">
                {{ displayName }}
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle class="text-sm">
            {{ t('aiDevelopmentEngine.capabilities') }}
          </CardTitle>
          <CardDescription class="text-xs">
            {{ t('aiDevelopmentEngine.capabilitiesDescription') }}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div class="overflow-hidden rounded-md border">
            <div
              v-for="capability in capabilityItems"
              :key="capability.key"
              class="flex min-h-11 items-center justify-between gap-3 border-b px-3 py-2 last:border-b-0"
            >
              <span class="text-sm">{{ capability.name }}</span>
              <Badge
                variant="secondary"
                class="shrink-0"
              >
                {{ capability.enabled ? enabledLabel : disabledLabel }}
              </Badge>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { CircleOff, Cpu } from 'lucide-vue-next'
import {
  Badge,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Spinner,
} from '@memohai/ui'
import { client } from '@memohai/sdk/client'

interface AIDevelopmentEngineStatus {
  status: string
  authStatus: string
  displayName: string
}

interface AIDevelopmentEngineCapability {
  key: string
  name: string
  enabled: boolean
}

interface AIDevelopmentEngineCapabilities {
  displayName: string
  capabilities: AIDevelopmentEngineCapability[]
}

const { t } = useI18n()

const status = ref<AIDevelopmentEngineStatus | null>(null)
const capabilities = ref<AIDevelopmentEngineCapabilities | null>(null)
const loading = ref(true)
const loadError = ref('')
const disabledLabel = '未启用'
const enabledLabel = '已启用'
const notConfiguredLabel = '未配置'

const fallbackCapabilities: AIDevelopmentEngineCapability[] = [
  { key: 'read_project', name: '读取项目', enabled: false },
  { key: 'modify_files', name: '修改文件', enabled: false },
  { key: 'execute_commands', name: '执行命令', enabled: false },
  { key: 'browser_operations', name: '浏览器操作', enabled: false },
  { key: 'git_operations', name: 'Git 操作', enabled: false },
]

const displayName = computed(() =>
  status.value?.displayName
  ?? capabilities.value?.displayName
  ?? t('aiDevelopmentEngine.title'),
)

const connectionStatusLabel = computed(() =>
  status.value?.status === 'disabled'
    ? disabledLabel
    : (status.value?.status ?? disabledLabel),
)

const authStatusLabel = computed(() =>
  status.value?.authStatus === 'not_configured'
    ? notConfiguredLabel
    : (status.value?.authStatus ?? notConfiguredLabel),
)

const capabilityItems = computed(() =>
  capabilities.value?.capabilities?.length
    ? capabilities.value.capabilities
    : fallbackCapabilities,
)

onMounted(async () => {
  try {
    const [statusResponse, capabilitiesResponse] = await Promise.all([
      client.get<AIDevelopmentEngineStatus>({
        url: '/ai-development-engine/status',
        throwOnError: true,
      }),
      client.get<AIDevelopmentEngineCapabilities>({
        url: '/ai-development-engine/capabilities',
        throwOnError: true,
      }),
    ])

    status.value = statusResponse.data
    capabilities.value = capabilitiesResponse.data
  }
  catch {
    loadError.value = t('aiDevelopmentEngine.loadFailed')
  }
  finally {
    loading.value = false
  }
})
</script>
