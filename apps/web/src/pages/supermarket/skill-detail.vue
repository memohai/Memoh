<template>
  <div class="mx-auto max-w-5xl px-4 py-6 md:px-6 md:py-8">
    <div
      v-if="loading"
      class="flex items-center justify-center py-16 text-xs text-muted-foreground"
    >
      <Spinner class="mr-2" />
      {{ $t('common.loading') }}
    </div>

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
      <div class="mb-6 flex items-center justify-between gap-3">
        <Button
          variant="ghost"
          size="sm"
          class="-ml-2"
          @click="router.push({ name: 'supermarket' })"
        >
          <ArrowLeft class="size-4" />
          {{ $t('common.back') }}
        </Button>
        <Button
          size="sm"
          @click="installDialogOpen = true"
        >
          <Download class="size-4" />
          {{ $t('supermarket.installToBot') }}
        </Button>
      </div>

      <header class="space-y-4">
        <div class="flex items-start gap-4">
          <div class="flex size-16 shrink-0 items-center justify-center rounded-md border bg-background shadow-sm">
            <Zap class="size-8 text-muted-foreground" />
          </div>
          <div class="min-w-0 flex-1">
            <h1 class="break-words text-3xl font-semibold leading-tight">
              {{ skill.name || skill.id }}
            </h1>
          </div>
        </div>

        <div class="flex flex-wrap gap-1.5">
          <Badge
            v-for="tag in skill.metadata?.tags"
            :key="tag"
            variant="secondary"
            size="sm"
          >
            {{ tag }}
          </Badge>
        </div>
      </header>

      <p class="mt-8 max-w-4xl text-base leading-7 text-muted-foreground">
        {{ skill.description || $t('supermarket.noDescription') }}
      </p>

      <section
        v-if="skill.files?.length"
        class="mt-8"
      >
        <h2 class="text-lg font-semibold">
          {{ $t('supermarket.files') }}
          <span class="ml-1 text-muted-foreground">{{ skill.files.length }}</span>
        </h2>
        <div class="mt-4 divide-y divide-border/60">
          <div
            v-for="file in skill.files"
            :key="file"
            class="flex items-center gap-3 py-4"
          >
            <div class="flex size-10 shrink-0 items-center justify-center rounded-md border bg-background">
              <FileText class="size-5 text-muted-foreground" />
            </div>
            <p class="min-w-0 flex-1 break-words text-sm font-medium">
              {{ file }}
            </p>
          </div>
        </div>
      </section>

      <section
        v-if="skill.content"
        class="mt-8"
      >
        <h2 class="text-lg font-semibold">
          {{ $t('supermarket.instructions') }}
        </h2>
        <pre class="mt-4 max-h-[420px] overflow-auto rounded-md border bg-muted/30 p-4 text-xs leading-6 whitespace-pre-wrap">{{ skill.content }}</pre>
      </section>

      <section class="mt-10">
        <h2 class="text-lg font-semibold">
          {{ $t('supermarket.information') }}
        </h2>
        <div class="mt-4 grid gap-x-12 gap-y-5 md:grid-cols-2">
          <InfoItem
            :label="$t('supermarket.category')"
            :value="$t('supermarket.skillsSection')"
          />
          <InfoItem
            :label="$t('supermarket.developer')"
            :value="skill.metadata?.author?.name || $t('common.none')"
          />
          <InfoItem
            :label="$t('supermarket.files')"
            :value="String(skill.files?.length ?? 0)"
          />
          <div
            v-if="skill.metadata?.homepage"
            class="space-y-1"
          >
            <p class="text-sm text-muted-foreground">
              {{ $t('supermarket.links') }}
            </p>
            <a
              :href="skill.metadata.homepage"
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
      @installed="loadSkill"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { ArrowLeft, Download, ExternalLink, FileText, Zap } from 'lucide-vue-next'
import { Badge, Button, Spinner } from '@memohai/ui'
import { getSupermarketSkillsById, type HandlersSupermarketSkillEntry } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import InstallSkillDialog from './components/install-skill-dialog.vue'

const InfoItem = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
  },
  setup(props) {
    return () => h('div', { class: 'space-y-1' }, [
      h('p', { class: 'text-sm text-muted-foreground' }, props.label),
      h('p', { class: 'break-words text-sm font-medium' }, props.value),
    ])
  },
})

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const skill = ref<HandlersSupermarketSkillEntry | null>(null)
const loading = ref(false)
const installDialogOpen = ref(false)

const skillId = computed(() => String(route.params.skillId || ''))

async function loadSkill() {
  if (!skillId.value) return
  loading.value = true
  try {
    const { data } = await getSupermarketSkillsById({
      path: { id: skillId.value },
      throwOnError: true,
    })
    skill.value = data
  } catch (error) {
    skill.value = null
    toast.error(resolveApiErrorMessage(error, t('supermarket.loadError')))
  } finally {
    loading.value = false
  }
}

onMounted(loadSkill)
watch(skillId, loadSkill)
</script>
