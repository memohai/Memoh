<template>
  <div class="mx-auto max-w-5xl px-4 py-6 md:px-6 md:py-8">
    <InlineLoadingRow
      v-if="loading"
      class="justify-center py-16"
    >
      {{ $t('common.loading') }}
    </InlineLoadingRow>

    <div
      v-else-if="!skill"
      class="py-16 text-center"
    >
      <p class="text-sm font-medium">
        {{ $t('supermarket.skillNotFound') }}
      </p>
      <Button
        variant="outline"
        size="sm"
        class="mt-4"
        @click="router.push({ name: 'supermarket' })"
      >
        <ArrowLeft class="size-4" />
        {{ $t('supermarket.backToSupermarket') }}
      </Button>
    </div>

    <template v-else>
      <MarketDetailHeader
        :name="displayName"
        :tags="skill.tags"
        @back="router.push({ name: 'supermarket' })"
        @install="installDialogOpen = true"
      >
        <template #icon>
          <SkillIcon
            :icon="skill.icon"
            variant="detail"
          />
        </template>
      </MarketDetailHeader>

      <p class="mt-8 max-w-4xl text-base leading-7 text-muted-foreground">
        {{ skill.description || $t('supermarket.noDescription') }}
      </p>

      <section
        v-if="skill.files?.length"
        class="mt-8"
      >
        <h2 class="mb-4 text-lg font-semibold">
          {{ $t('supermarket.files') }}
          <span class="ml-1 text-muted-foreground">{{ skill.files.length }}</span>
        </h2>
        <SettingsSection>
          <SettingsRow
            v-for="file in skill.files"
            :key="file"
          >
            <template #leading>
              <div class="flex size-10 shrink-0 items-center justify-center rounded-md border bg-background">
                <FileText class="size-5 text-muted-foreground" />
              </div>
            </template>
            <template #content>
              <p class="min-w-0 break-words text-sm font-medium">
                {{ file }}
              </p>
            </template>
          </SettingsRow>
        </SettingsSection>
      </section>

      <section class="mt-10">
        <h2 class="text-lg font-semibold">
          {{ $t('supermarket.information') }}
        </h2>
        <div class="mt-4 grid gap-x-12 gap-y-5 md:grid-cols-2">
          <InfoItem
            :label="$t('supermarket.registry')"
            :value="registryName || skill.registry_id || $t('common.none')"
          />
          <InfoItem
            :label="$t('supermarket.package')"
            :value="skill.package_id || $t('common.none')"
          />
          <InfoItem
            :label="$t('supermarket.category')"
            :value="skill.category_name || skill.category || $t('common.none')"
          />
          <InfoItem
            :label="$t('supermarket.developer')"
            :value="skill.author?.name || $t('common.none')"
          />
          <div
            v-if="skill.homepage"
            class="space-y-1"
          >
            <p class="text-sm text-muted-foreground">
              {{ $t('supermarket.links') }}
            </p>
            <a
              :href="skill.homepage"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex items-center gap-1 text-sm font-medium text-primary hover:underline"
            >
              {{ $t('supermarket.website') }}
              <ExternalLink class="size-3.5" />
            </a>
          </div>
        </div>
      </section>
    </template>

    <InstallSkillDialog
      v-model:open="installDialogOpen"
      :skill="skill"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ArrowLeft, ExternalLink, FileText } from 'lucide-vue-next'
import { Button, InlineLoadingRow, SettingsRow, SettingsSection, toast } from '@felinic/ui'
import {
  getSupermarketRegistries,
  getSupermarketRegistriesByRegistryIdPackagesByPackageIdSkillsBySkillId,
  type HandlersSupermarketCatalogSkill,
} from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import InstallSkillDialog from './components/install-skill-dialog.vue'
import InfoItem from './components/info-item.vue'
import MarketDetailHeader from './components/market-detail-header.vue'
import SkillIcon from './components/skill-icon.vue'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const skill = ref<HandlersSupermarketCatalogSkill | null>(null)
const registryName = ref('')
const loading = ref(false)
const installDialogOpen = ref(false)

const registryId = computed(() => String(route.params.registryId || ''))
const packageId = computed(() => String(route.params.packageId || ''))
const skillId = computed(() => String(route.params.skillId || ''))
const skillIdentity = computed(() => `${registryId.value}/${packageId.value}/${skillId.value}`)
const displayName = computed(() => skill.value
  ? skill.value.name || skill.value.skill_id || ''
  : '')

async function loadSkill() {
  if (!registryId.value || !packageId.value || !skillId.value) return
  loading.value = true
  try {
    const [{ data }, registryResponse] = await Promise.all([
      getSupermarketRegistriesByRegistryIdPackagesByPackageIdSkillsBySkillId({
        path: {
          registry_id: registryId.value,
          package_id: packageId.value,
          skill_id: skillId.value,
        },
        throwOnError: true,
      }),
      getSupermarketRegistries({ throwOnError: true }).catch(() => null),
    ])
    registryName.value = registryResponse?.data.data
      ?.find(registry => registry.id === registryId.value)?.name || registryId.value
    skill.value = data
  } catch (error) {
    registryName.value = registryId.value
    skill.value = null
    toast.error(resolveApiErrorMessage(error, t('supermarket.loadError')))
  } finally {
    loading.value = false
  }
}

onMounted(loadSkill)
watch(skillIdentity, loadSkill)
</script>
