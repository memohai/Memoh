<template>
  <section class="px-4 pt-2 pb-10 lg:px-6 md:pt-4 md:pb-12">
    <div class="mb-6 flex items-center justify-between flex-wrap gap-3">
      <h1 class="text-lg font-semibold">
        {{ t('teams.title') }}
      </h1>
      <Button @click="openCreate">
        <Plus class="mr-1.5" />
        {{ t('teams.create') }}
      </Button>
    </div>

    <div
      v-if="teams.length > 0"
      class="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3"
    >
      <Card
        v-for="team in teams"
        :key="team.id"
        class="cursor-pointer hover:bg-accent/30 transition-colors"
        @click="goToDetail(team.id)"
      >
        <CardHeader>
          <div class="flex items-start gap-3">
            <Avatar class="size-10 shrink-0 rounded-full">
              <AvatarImage
                v-if="team.avatar_url"
                :src="team.avatar_url"
                :alt="team.name"
              />
              <AvatarFallback class="text-sm">
                <Users
                  v-if="!teamInitials(team)"
                  class="size-4"
                />
                <template v-else>
                  {{ teamInitials(team) }}
                </template>
              </AvatarFallback>
            </Avatar>
            <div class="min-w-0 flex-1">
              <CardTitle class="truncate">
                {{ team.name }}
              </CardTitle>
              <CardDescription
                v-if="team.description"
                class="truncate"
              >
                {{ team.description }}
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent class="text-xs text-muted-foreground">
          <div v-if="team.shared_dir_name">
            {{ t('teams.sharedDir') }}: <code>/team/{{ team.shared_dir_name }}</code>
          </div>
          <div>{{ t('teams.createdAt') }}: {{ formatDate(team.created_at) }}</div>
        </CardContent>
      </Card>
    </div>

    <Empty
      v-else-if="!isLoading"
      class="mt-20 flex flex-col items-center justify-center"
    >
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Users />
        </EmptyMedia>
      </EmptyHeader>
      <EmptyTitle>{{ t('teams.emptyTitle') }}</EmptyTitle>
      <EmptyDescription>{{ t('teams.emptyDescription') }}</EmptyDescription>
    </Empty>

    <Dialog v-model:open="showCreate">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ t('teams.create') }}</DialogTitle>
        </DialogHeader>
        <form
          class="space-y-3"
          @submit.prevent="submitCreate"
        >
          <div class="flex items-center gap-3">
            <div class="group/avatar relative size-14 shrink-0 cursor-pointer overflow-hidden rounded-full">
              <Avatar class="size-14 rounded-full">
                <AvatarImage
                  v-if="form.avatar_url.trim()"
                  :src="form.avatar_url.trim()"
                  :alt="form.name"
                />
                <AvatarFallback class="text-lg">
                  <Users
                    v-if="!createInitials"
                    class="size-5"
                  />
                  <template v-else>
                    {{ createInitials }}
                  </template>
                </AvatarFallback>
              </Avatar>
              <button
                type="button"
                class="absolute inset-0 flex items-center justify-center rounded-full bg-black/40 opacity-0 transition-opacity group-hover/avatar:opacity-100"
                :title="t('common.edit')"
                :aria-label="t('common.edit')"
                @click="createAvatarDialogOpen = true"
              >
                <SquarePen class="size-5 text-white" />
              </button>
            </div>
            <div class="min-w-0 flex-1">
              <label class="text-sm">{{ t('teams.name') }}</label>
              <Input
                v-model="form.name"
                required
              />
            </div>
          </div>
          <div>
            <label class="text-sm">{{ t('teams.description') }}</label>
            <Textarea
              v-model="form.description"
              rows="3"
            />
          </div>
          <div>
            <label class="text-sm">{{ t('teams.sharedDir') }}</label>
            <Input
              v-model="form.shared_dir_name"
              :placeholder="t('teams.sharedDirPlaceholder')"
            />
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              @click="showCreate = false"
            >
              {{ t('common.cancel') }}
            </Button>
            <Button
              type="submit"
              :disabled="creating"
            >
              {{ t('common.save') }}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>

    <AvatarEditDialog
      v-model:open="createAvatarDialogOpen"
      v-model:avatar-url="form.avatar_url"
      :fallback-text="createInitials"
      :title="t('teams.editAvatar')"
      :description="t('teams.editAvatarDescription')"
      :placeholder="t('teams.avatarUrlPlaceholder')"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { toast } from 'vue-sonner'
import { Plus, SquarePen, Users } from 'lucide-vue-next'
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  Input,
  Textarea,
} from '@memohai/ui'
import { getTeams, postTeams } from '@memohai/sdk'
import AvatarEditDialog from '@/components/avatar-edit-dialog/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'

const router = useRouter()
const { t } = useI18n()
const queryCache = useQueryCache()

const { data, status } = useQuery({
  key: () => ['teams'],
  query: async () => {
    const { data, error } = await getTeams()
    if (error) throw error
    return data ?? []
  },
})

const teams = computed(() => data.value ?? [])
const isLoading = computed(() => status.value === 'loading')

const showCreate = ref(false)
const createAvatarDialogOpen = ref(false)
const form = reactive({
  name: '',
  description: '',
  shared_dir_name: '',
  avatar_url: '',
})

const createInitials = computed(() => {
  const label = form.name.trim()
  if (!label) return ''
  return label.slice(0, 2).toUpperCase()
})

function openCreate() {
  form.name = ''
  form.description = ''
  form.shared_dir_name = ''
  form.avatar_url = ''
  showCreate.value = true
}

const { mutate: doCreate, status: createStatus } = useMutation({
  mutation: async () => {
    const { data, error } = await postTeams({
      body: {
        name: form.name.trim(),
        description: form.description.trim(),
        avatar_url: form.avatar_url.trim() || undefined,
        shared_dir_name: form.shared_dir_name.trim(),
      },
    })
    if (error) throw error
    return data
  },
  onSuccess: () => {
    toast.success(t('teams.createSuccess'))
    showCreate.value = false
    void queryCache.invalidateQueries({ key: ['teams'] })
  },
  onError: (err) => {
    toast.error(resolveApiErrorMessage(err, t('teams.createFailed')))
  },
})

const creating = computed(() => createStatus.value === 'loading')

function submitCreate() {
  if (!form.name.trim()) return
  doCreate()
}

function goToDetail(id: string | undefined) {
  if (!id) return
  router.push({ name: 'team-detail', params: { teamId: id } })
}

function teamInitials(team: { name?: string }): string {
  const label = (team.name ?? '').trim()
  if (!label) return ''
  return label.slice(0, 2).toUpperCase()
}

function formatDate(value: string | undefined): string {
  if (!value) return ''
  try {
    return new Date(value).toLocaleString()
  }
  catch {
    return value
  }
}
</script>
