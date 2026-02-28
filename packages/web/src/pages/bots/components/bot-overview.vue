<script setup lang="ts">
import { ref,computed,watch } from 'vue'
import  {
  type BotsBotCheck,
  getBotsByIdChecks,
  getBotsById
} from '@memoh/sdk'
import { useRoute } from 'vue-router'
import { toast } from 'vue-sonner'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { useBotStatusMeta } from '@/composables/useBotStatusMeta'
import { useQuery } from '@pinia/colada'
import { useI18n } from 'vue-i18n'
import { Badge,Button,Spinner } from '@memoh/ui'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'


const checksLoading = ref(false)

const checks = ref<BotCheck[]>([])

const route = useRoute()

const botId = computed(() => route.params.botId as string)

const activeTab = useSyncedQueryParam('tab', 'overview')

watch([activeTab, botId], ([tab]) => {
  if (!botId.value) {
    return
  }
  if (tab === 'overview') {
    void loadChecks(true)
  }
}, { immediate: true })

const { t } = useI18n()
const { data: bot } = useQuery({
  key: () => ['bot', botId.value],
  query: async () => {
    const { data } = await getBotsById({ path: { id: botId.value }, throwOnError: true })
    return data
  },
  enabled: () => !!botId.value,
})


const {
  hasIssue
} = useBotStatusMeta(bot, t)

type BotCheck = BotsBotCheck



async function fetchChecks(id: string): Promise<BotCheck[]> {
  const { data } = await getBotsByIdChecks({ path: { id }, throwOnError: true })
  return data?.items ?? []
}


async function loadChecks(showToast: boolean) {
  checksLoading.value = true
  checks.value = []
  try {
    checks.value = await fetchChecks(botId.value)
  } catch (error) {
    if (showToast) {
      toast.error(resolveErrorMessage(error, t('bots.checks.loadFailed')))
    }
  } finally {
    checksLoading.value = false
  }
}

async function handleRefreshChecks() {
  await loadChecks(true)
}

function resolveErrorMessage(error: unknown, fallback: string): string {
  return resolveApiErrorMessage(error, fallback)
}

const checksSummaryText = computed(() => {
  const issueCount = checks.value.filter((item) => item.status === 'warn' || item.status === 'error').length
  if (issueCount > 0) {
    return t('bots.checks.issueCount', { count: issueCount })
  }
  if (checks.value.length === 0) {
    return t('bots.checks.empty')
  }
  return t('bots.checks.ok')
})


function checkStatusLabel(status: BotCheck['status']): string {
  if (status === 'error') return t('bots.checks.status.error')
  if (status === 'warn') return t('bots.checks.status.warn')
  if (status === 'unknown') return t('bots.checks.status.unknown')
  return t('bots.checks.status.ok')
}

function checkTitleLabel(item: BotCheck): string {
  const titleKey = (item.title_key ?? '').trim()
  if (titleKey) {
    const translated = t(titleKey)
    if (translated !== titleKey) {
      return translated
    }
  }
  return (item.type ?? '').trim() || (item.id ?? '').trim() || '-'
}

function checkStatusVariant(status: BotCheck['status']): 'default' | 'secondary' | 'destructive' {
  if (status === 'error') return 'destructive'
  if (status === 'warn') return 'secondary'
  if (status === 'unknown') return 'secondary'
  return 'default'
}

</script>

<template>
  <div>
    <div class="rounded-md border p-4">
      <div class="flex items-center justify-between gap-2">
        <div>
          <p class="text-sm font-medium">
            {{ $t('bots.checks.title') }}
          </p>
          <p class="text-sm text-muted-foreground">
            {{ $t('bots.checks.subtitle') }}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          :disabled="checksLoading"
          @click="handleRefreshChecks"
        >
          <Spinner
            v-if="checksLoading"
            class="mr-1.5"
          />
          {{ $t('common.refresh') }}
        </Button>
      </div>
      <div class="mt-3 flex items-center gap-2 text-sm">
        <Badge
          :variant="hasIssue ? 'destructive' : 'default'"
          class="text-xs"
        >
          {{ checksSummaryText }}
        </Badge>
      </div>

      <div
        v-if="checksLoading && checks.length === 0"
        class="mt-4 flex items-center gap-2 text-sm text-muted-foreground"
      >
        <Spinner />
        <span>{{ $t('common.loading') }}</span>
      </div>

      <p
        v-else-if="checks.length === 0"
        class="mt-4 text-sm text-muted-foreground"
      >
        {{ $t('bots.checks.empty') }}
      </p>

      <ul
        v-else
        class="mt-4 divide-y"
      >
        <li
          v-for="item in checks"
          :key="item.id"
          class="py-3 first:pt-0 last:pb-0"
        >
          <div class="flex items-center justify-between gap-2">
            <div class="min-w-0">
              <p class="font-mono text-xs">
                {{ checkTitleLabel(item) }}
              </p>
              <p
                v-if="item.subtitle"
                class="mt-0.5 text-xs text-muted-foreground"
              >
                {{ item.subtitle }}
              </p>
            </div>
            <Badge
              :variant="checkStatusVariant(item.status)"
              class="text-[10px]"
            >
              {{ checkStatusLabel(item.status) }}
            </Badge>
          </div>
          <p class="mt-2 text-sm">
            {{ item.summary }}
          </p>
          <p
            v-if="item.detail"
            class="mt-1 text-xs text-muted-foreground break-all"
          >
            {{ item.detail }}
          </p>
        </li>
      </ul>
    </div>
  </div>
</template>