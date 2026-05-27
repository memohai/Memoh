<template>
  <div class="flex h-full flex-col bg-background">
    <header class="border-b">
      <div class="mx-auto flex max-w-[1120px] items-start gap-3 px-4 py-3 lg:px-6">
        <Button
          variant="ghost"
          size="sm"
          class="mt-0.5 h-8 shrink-0 px-2"
          @click="router.push({ name: 'team-workspace', params: { teamId } })"
        >
          <ArrowLeft class="size-4" />
        </Button>

        <div class="min-w-0 flex-1">
          <div class="flex min-w-0 flex-wrap items-center gap-2">
            <template v-if="editingTitle">
              <Input
                v-model="titleDraft"
                class="h-8 min-w-64 flex-1 text-sm font-semibold"
                @keyup.enter="saveTitle"
              />
              <Button
                size="sm"
                :disabled="savingIssue || !titleDraft.trim()"
                @click="saveTitle"
              >
                <Check class="mr-1.5 size-4" />
                {{ t('common.save') }}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                @click="cancelTitleEdit"
              >
                <X class="mr-1.5 size-4" />
                {{ t('common.cancel') }}
              </Button>
            </template>

            <template v-else>
              <h1 class="min-w-0 break-words text-lg font-semibold leading-7">
                {{ issue?.title ?? t('teams.loading') }}
                <span
                  v-if="issue?.number"
                  class="ml-1 font-normal text-muted-foreground"
                >
                  #{{ issue.number }}
                </span>
              </h1>
              <Badge
                v-if="issue"
                :variant="badgeVariantForStatus(issue.status)"
                class="h-5 shrink-0 px-1.5 text-[10px]"
              >
                {{ statusLabel(issue.status) }}
              </Badge>
              <Button
                v-if="issue"
                variant="ghost"
                size="icon"
                class="size-7 shrink-0"
                :title="t('common.edit')"
                @click="editingTitle = true"
              >
                <Edit3 class="size-3.5" />
              </Button>
            </template>
          </div>

          <div class="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
            <span>{{ team?.name ?? '' }}</span>
            <span v-if="issue?.created_at">· {{ t('teams.createdAt') }} {{ formatDate(issue.created_at) }}</span>
            <span v-if="issue?.updated_at">· {{ t('common.updatedAt') }} {{ formatDate(issue.updated_at) }}</span>
          </div>
        </div>
      </div>
    </header>

    <div class="flex-1 overflow-auto">
      <div class="mx-auto grid max-w-[1120px] gap-4 px-4 py-4 lg:grid-cols-[minmax(0,1fr)_260px] lg:px-6">
        <main class="min-w-0 space-y-3">
          <IssueTimelineItem
            :author-label="issueAuthorLabel"
            :avatar-url="issueAuthorAvatar"
            :author-meta="formatDate(issue?.created_at)"
            :author-type="issue?.created_by_type"
          >
            <template #actions>
              <Button
                v-if="!editingDescription"
                variant="ghost"
                size="sm"
                class="h-7 px-2 text-xs"
                @click="editingDescription = true"
              >
                <Edit3 class="mr-1 size-3.5" />
                {{ t('common.edit') }}
              </Button>
            </template>

            <div
              v-if="editingDescription"
              class="space-y-3"
            >
              <Textarea
                v-model="descriptionDraft"
                :placeholder="t('teams.issueDescriptionPlaceholder')"
                rows="6"
                class="!text-sm leading-6 placeholder:text-sm"
              />
              <div class="flex justify-end gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  @click="cancelDescriptionEdit"
                >
                  {{ t('common.cancel') }}
                </Button>
                <Button
                  size="sm"
                  :disabled="savingIssue"
                  @click="saveDescription"
                >
                  {{ t('common.save') }}
                </Button>
              </div>
            </div>

            <MentionMarkdown
              v-else-if="issue?.description?.trim()"
              :content="issue.description"
              :members="members"
              :bots="bots"
              markdown-class="!h-auto !overflow-visible !bg-transparent [&>.prose]:px-0 [&>.prose]:py-0 [&_.markdown-renderer]:text-sm [&_.markdown-renderer]:leading-6 [&_.markdown-renderer_li]:text-sm [&_.markdown-renderer_li]:leading-6 [&_.markdown-renderer_p]:text-sm [&_.markdown-renderer_p]:leading-6 [&_.prose]:text-sm [&_.prose]:leading-6 [&_.prose_li]:text-sm [&_.prose_li]:leading-6 [&_.prose_p]:text-sm [&_.prose_p]:leading-6"
              @open="openMention"
            />
            <div
              v-else
              class="rounded-md border border-dashed px-3 py-6 text-center text-sm text-muted-foreground"
            >
              {{ t('teams.noDescription') }}
            </div>
          </IssueTimelineItem>

          <div
            v-if="activeHandoffs.length > 0"
            class="flex gap-3"
          >
            <div class="mt-1 flex size-7 shrink-0 items-center justify-center rounded-full border bg-background text-primary">
              <Bot class="size-3.5" />
            </div>
            <div class="min-w-0 flex-1 rounded-md border border-primary/20 bg-primary/5 px-2.5 py-2 text-sm">
              <div class="font-medium">
                {{ t('teams.agentWorking') }}
              </div>
              <p class="mt-1 text-muted-foreground">
                {{ t('teams.agentWorkingHint', { count: activeHandoffs.length }) }}
              </p>
            </div>
          </div>

          <div
            v-if="comments.length === 0"
            class="ml-10 rounded-md border border-dashed px-4 py-5 text-center text-sm text-muted-foreground"
          >
            {{ t('teams.noComments') }}
          </div>

          <IssueTimelineItem
            v-for="comment in comments"
            :key="comment.id"
            :author-label="commentAuthorLabel(comment)"
            :avatar-url="commentAuthorAvatar(comment)"
            :author-meta="formatDate(comment.created_at)"
            :author-type="comment.author_type || 'user'"
          >
            <MentionMarkdown
              :content="comment.content"
              :members="members"
              :bots="bots"
              markdown-class="!h-auto !overflow-visible !bg-transparent [&>.prose]:px-0 [&>.prose]:py-0 [&_.markdown-renderer]:text-sm [&_.markdown-renderer]:leading-6 [&_.markdown-renderer_li]:text-sm [&_.markdown-renderer_li]:leading-6 [&_.markdown-renderer_p]:text-sm [&_.markdown-renderer_p]:leading-6 [&_.prose]:text-sm [&_.prose]:leading-6 [&_.prose_li]:text-sm [&_.prose_li]:leading-6 [&_.prose_p]:text-sm [&_.prose_p]:leading-6"
              @open="openMention"
            />
          </IssueTimelineItem>

          <div class="ml-0 lg:ml-10">
            <IssueMarkdownComposer
              v-model="newComment"
              :placeholder="t('teams.commentPlaceholder')"
              :helper="t('teams.commentMentionHint')"
              :submit-label="t('teams.postComment')"
              :write-label="t('teams.write')"
              :preview-label="t('common.preview')"
              :empty-preview-label="t('teams.previewEmpty')"
              :disabled="posting"
              @submit="submitComment"
            />
          </div>
        </main>

        <aside class="min-w-0">
          <div class="sticky top-3 space-y-3">
            <IssueSidebarSection :title="t('teams.assignee')">
              <template #icon>
                <UserCircle class="size-3.5 text-muted-foreground" />
              </template>
              <Select
                :model-value="assigneeValueFromIssue"
                @update:model-value="(value) => updateAssignee(String(value))"
              >
                <SelectTrigger class="h-8">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">
                    {{ t('teams.unassigned') }}
                  </SelectItem>
                  <SelectItem
                    v-for="member in assignableMembers"
                    :key="member.id"
                    :value="memberValue(member)"
                  >
                    {{ memberLabel(member) }}
                  </SelectItem>
                </SelectContent>
              </Select>
            </IssueSidebarSection>

            <IssueSidebarSection :title="t('teams.statusLabel')">
              <template #icon>
                <CircleDot class="size-3.5 text-muted-foreground" />
              </template>
              <Select
                :model-value="normalizeStatus(issue?.status)"
                @update:model-value="(value) => updateStatus(String(value) as IssueStatus)"
              >
                <SelectTrigger class="h-8">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem
                    v-for="status in ISSUE_COLUMNS"
                    :key="status.value"
                    :value="status.value"
                  >
                    {{ t(status.labelKey) }}
                  </SelectItem>
                </SelectContent>
              </Select>
            </IssueSidebarSection>

            <IssueSidebarSection :title="t('teams.botSessions')">
              <template #icon>
                <MessageSquare class="size-3.5 text-muted-foreground" />
              </template>
              <div
                v-if="botSessions.length === 0"
                class="text-xs text-muted-foreground"
              >
                {{ t('teams.noBotSessions') }}
              </div>
              <ul
                v-else
                class="space-y-2"
              >
                <li
                  v-for="session in botSessions"
                  :key="session.bot_id + ':' + session.session_id"
                  class="flex items-center justify-between gap-2"
                >
                  <span class="min-w-0 truncate text-xs">{{ session.bot_name || session.bot_id }}</span>
                  <Button
                    variant="ghost"
                    size="sm"
                    class="h-7 px-2"
                    @click="openBotSession(session.bot_id, session.session_id)"
                  >
                    <MessageSquare class="size-3.5" />
                  </Button>
                </li>
              </ul>
            </IssueSidebarSection>

            <IssueSidebarSection :title="t('teams.handoffs')">
              <template #icon>
                <GitBranch class="size-3.5 text-muted-foreground" />
              </template>
              <div
                v-if="handoffs.length === 0"
                class="text-xs text-muted-foreground"
              >
                {{ t('teams.noHandoffs') }}
              </div>
              <ul
                v-else
                class="space-y-2"
              >
                <li
                  v-for="handoff in handoffs"
                  :key="handoff.id"
                  class="text-xs"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="min-w-0 truncate font-medium">{{ botName(handoff.to_bot_id) }}</span>
                    <Badge :variant="handoffBadgeVariant(handoff.status)">
                      {{ handoff.status }}
                    </Badge>
                  </div>
                  <p
                    v-if="handoff.failure_reason"
                    class="mt-1 text-muted-foreground"
                  >
                    {{ handoff.failure_reason }}
                  </p>
                </li>
              </ul>
            </IssueSidebarSection>

            <IssueSidebarSection :title="t('teams.metadata')">
              <template #icon>
                <Info class="size-3.5 text-muted-foreground" />
              </template>
              <dl class="space-y-2 text-xs">
                <div class="flex justify-between gap-3">
                  <dt class="text-muted-foreground">
                    {{ t('teams.createdAt') }}
                  </dt>
                  <dd class="text-right">
                    {{ formatDate(issue?.created_at) }}
                  </dd>
                </div>
                <div class="flex justify-between gap-3">
                  <dt class="text-muted-foreground">
                    {{ t('common.updatedAt') }}
                  </dt>
                  <dd class="text-right">
                    {{ formatDate(issue?.updated_at) }}
                  </dd>
                </div>
                <div class="flex justify-between gap-3">
                  <dt class="text-muted-foreground">
                    {{ t('teams.id') }}
                  </dt>
                  <dd class="max-w-40 truncate font-mono">
                    {{ issue?.id }}
                  </dd>
                </div>
              </dl>
            </IssueSidebarSection>

            <IssueSidebarSection :title="t('common.dangerZone')">
              <template #icon>
                <AlertTriangle class="size-3.5 text-muted-foreground" />
              </template>
              <Button
                v-if="issue"
                variant="destructive"
                size="sm"
                class="w-full"
                @click="showDelete = true"
              >
                <Trash2 class="mr-1.5 size-4" />
                {{ t('teams.deleteIssue') }}
              </Button>
            </IssueSidebarSection>
          </div>
        </aside>
      </div>
    </div>

    <Dialog v-model:open="showMemberDialog">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ selectedMember ? memberLabel(selectedMember) : t('teams.member') }}</DialogTitle>
          <DialogDescription>{{ selectedMember?.role || selectedMember?.member_type }}</DialogDescription>
        </DialogHeader>
        <div
          v-if="selectedMember"
          class="space-y-3 text-sm"
        >
          <div class="flex items-center gap-3">
            <Avatar class="size-10">
              <AvatarImage
                v-if="memberAvatar(selectedMember)"
                :src="memberAvatar(selectedMember)"
                :alt="memberLabel(selectedMember)"
              />
              <AvatarFallback>
                {{ initials(memberLabel(selectedMember)) }}
              </AvatarFallback>
            </Avatar>
            <div class="min-w-0">
              <div class="font-medium">
                {{ memberLabel(selectedMember) }}
              </div>
              <div class="text-xs text-muted-foreground">
                {{ selectedMember.member_type }}
              </div>
            </div>
          </div>
          <p
            v-if="selectedMember.instructions"
            class="rounded-md bg-muted p-3 text-muted-foreground"
          >
            {{ selectedMember.instructions }}
          </p>
        </div>
      </DialogContent>
    </Dialog>

    <Dialog v-model:open="showDelete">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ t('teams.deleteIssue') }}</DialogTitle>
          <DialogDescription>{{ t('teams.deleteIssueDescription') }}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="ghost"
            @click="showDelete = false"
          >
            {{ t('common.cancel') }}
          </Button>
          <Button
            variant="destructive"
            :disabled="deletingIssue"
            @click="deleteIssue"
          >
            {{ t('common.delete') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import type {
  BotsBot,
  HandlersCommentResponse,
  HandlersMemberResponse,
  HandlersUpdateIssueRequest,
} from '@memohai/sdk'
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { toast } from 'vue-sonner'
import {
  AlertTriangle,
  ArrowLeft,
  Bot,
  Check,
  CircleDot,
  Edit3,
  GitBranch,
  Info,
  MessageSquare,
  Trash2,
  UserCircle,
  X,
} from 'lucide-vue-next'
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Textarea,
} from '@memohai/ui'
import {
  deleteTeamsByTeamIdIssuesByIssueId,
  getBots,
  getTeamsByTeamId,
  getTeamsByTeamIdIssuesByIssueId,
  getTeamsByTeamIdIssuesByIssueIdComments,
  getTeamsByTeamIdIssuesByIssueIdHandoffs,
  getTeamsByTeamIdMembers,
  postTeamsByTeamIdIssuesByIssueIdAssign,
  postTeamsByTeamIdIssuesByIssueIdComments,
  putTeamsByTeamIdIssuesByIssueId,
} from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import IssueMarkdownComposer from './components/IssueMarkdownComposer.vue'
import IssueSidebarSection from './components/IssueSidebarSection.vue'
import IssueTimelineItem from './components/IssueTimelineItem.vue'
import MentionMarkdown from './components/MentionMarkdown.vue'

type IssueStatus = 'backlog' | 'todo' | 'in_progress' | 'blocked' | 'review' | 'done' | 'cancelled'

const ISSUE_COLUMNS: Array<{ value: IssueStatus, labelKey: string }> = [
  { value: 'backlog', labelKey: 'teams.status.backlog' },
  { value: 'todo', labelKey: 'teams.status.todo' },
  { value: 'in_progress', labelKey: 'teams.status.inProgress' },
  { value: 'blocked', labelKey: 'teams.status.blocked' },
  { value: 'review', labelKey: 'teams.status.review' },
  { value: 'done', labelKey: 'teams.status.done' },
  { value: 'cancelled', labelKey: 'teams.status.cancelled' },
]

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const queryCache = useQueryCache()

const teamId = computed(() => String(route.params.teamId ?? ''))
const issueId = computed(() => String(route.params.issueId ?? ''))

const { data: teamData } = useQuery({
  key: () => ['team', teamId.value],
  query: async () => {
    const { data, error } = await getTeamsByTeamId({ path: { team_id: teamId.value } })
    if (error) throw error
    return data
  },
  enabled: () => !!teamId.value,
})

const { data: issueData } = useQuery({
  key: () => ['team', teamId.value, 'issue', issueId.value],
  query: async () => {
    const { data, error } = await getTeamsByTeamIdIssuesByIssueId({
      path: { team_id: teamId.value, issue_id: issueId.value },
    })
    if (error) throw error
    return data
  },
  enabled: () => !!teamId.value && !!issueId.value,
})

const { data: commentsData, refetch: refetchComments } = useQuery({
  key: () => ['team', teamId.value, 'issue', issueId.value, 'comments'],
  query: async () => {
    const { data, error } = await getTeamsByTeamIdIssuesByIssueIdComments({
      path: { team_id: teamId.value, issue_id: issueId.value },
    })
    if (error) throw error
    return data ?? []
  },
  enabled: () => !!teamId.value && !!issueId.value,
})

const { data: handoffsData } = useQuery({
  key: () => ['team', teamId.value, 'issue', issueId.value, 'handoffs'],
  query: async () => {
    const { data, error } = await getTeamsByTeamIdIssuesByIssueIdHandoffs({
      path: { team_id: teamId.value, issue_id: issueId.value },
    })
    if (error) throw error
    return data ?? []
  },
  enabled: () => !!teamId.value && !!issueId.value,
})

const { data: membersData } = useQuery({
  key: () => ['team', teamId.value, 'members'],
  query: async () => {
    const { data, error } = await getTeamsByTeamIdMembers({ path: { team_id: teamId.value } })
    if (error) throw error
    return data ?? []
  },
  enabled: () => !!teamId.value,
})

const { data: botListData } = useQuery({
  key: () => ['bots-for-issue', teamId.value, issueId.value],
  query: async () => {
    const { data, error } = await getBots()
    if (error) throw error
    return data?.items ?? []
  },
})

const team = computed(() => teamData.value)
const issue = computed(() => issueData.value)
const comments = computed(() => commentsData.value ?? [])
const handoffs = computed(() => handoffsData.value ?? [])
const members = computed(() => membersData.value ?? [])
const bots = computed<BotsBot[]>(() => botListData.value ?? [])
const assignableMembers = computed(() =>
  members.value.filter((member) => member.member_type === 'bot' ? !!member.bot_id : !!member.user_id),
)
const activeHandoffs = computed(() =>
  handoffs.value.filter((handoff) => ['pending', 'dispatched', 'running'].includes(handoff.status ?? '')),
)

const titleDraft = ref('')
const descriptionDraft = ref('')
const editingTitle = ref(false)
const editingDescription = ref(false)
const newComment = ref('')
const showMemberDialog = ref(false)
const selectedMember = ref<HandlersMemberResponse | null>(null)
const showDelete = ref(false)

watch(issue, (next) => {
  titleDraft.value = next?.title ?? ''
  if (!editingDescription.value) {
    descriptionDraft.value = next?.description ?? ''
  }
}, { immediate: true })

const { mutateAsync: updateIssueMutation, isLoading: savingIssue } = useMutation({
  mutation: async (body: HandlersUpdateIssueRequest) => {
    const { data, error } = await putTeamsByTeamIdIssuesByIssueId({
      path: { team_id: teamId.value, issue_id: issueId.value },
      body,
    })
    if (error) throw error
    return data
  },
})

const { mutateAsync: assignIssueMutation } = useMutation({
  mutation: async (value: string) => {
    const { data, error } = await postTeamsByTeamIdIssuesByIssueIdAssign({
      path: { team_id: teamId.value, issue_id: issueId.value },
      body: parseAssigneeValue(value),
    })
    if (error) throw error
    return data
  },
})

const { mutateAsync: postCommentMutation, isLoading: posting } = useMutation({
  mutation: async () => {
    const { data, error } = await postTeamsByTeamIdIssuesByIssueIdComments({
      path: { team_id: teamId.value, issue_id: issueId.value },
      body: { content: newComment.value.trim() },
    })
    if (error) throw error
    return data
  },
})

const { mutateAsync: deleteIssueMutation, isLoading: deletingIssue } = useMutation({
  mutation: async () => {
    const { error } = await deleteTeamsByTeamIdIssuesByIssueId({
      path: { team_id: teamId.value, issue_id: issueId.value },
    })
    if (error) throw error
  },
})

const assigneeValueFromIssue = computed(() => {
  if (issue.value?.assignee_type === 'bot' && issue.value.assignee_bot_id) {
    return `bot:${issue.value.assignee_bot_id}`
  }
  if (issue.value?.assignee_type === 'user' && issue.value.assignee_user_id) {
    return `user:${issue.value.assignee_user_id}`
  }
  return 'none'
})

const issueAuthorLabel = computed(() => {
  if (issue.value?.created_by_bot_id) {
    const member = members.value.find((item) => item.bot_id === issue.value?.created_by_bot_id)
    return member ? memberLabel(member) : botName(issue.value.created_by_bot_id)
  }
  if (issue.value?.created_by_user_id) {
    const member = members.value.find((item) => item.user_id === issue.value?.created_by_user_id)
    return member ? memberLabel(member) : issue.value.created_by_user_id
  }
  return team.value?.name || t('teams.title')
})

const issueAuthorAvatar = computed(() => {
  if (!issue.value?.created_by_bot_id) return ''
  return bots.value.find((bot) => bot.id === issue.value?.created_by_bot_id)?.avatar_url ?? ''
})

const botSessions = computed(() => {
  const nameByID = new Map<string, string>()
  for (const bot of bots.value) {
    if (bot.id) nameByID.set(bot.id, bot.display_name ?? '')
  }
  const seen = new Set<string>()
  const out: Array<{ bot_id: string, bot_name: string, session_id: string }> = []
  for (const handoff of handoffs.value) {
    const botID = handoff.to_bot_id ?? ''
    const sessionID = handoff.target_session_id ?? ''
    if (!botID || !sessionID) continue
    const key = `${botID}:${sessionID}`
    if (seen.has(key)) continue
    seen.add(key)
    out.push({ bot_id: botID, bot_name: nameByID.get(botID) ?? '', session_id: sessionID })
  }
  return out
})

async function saveIssuePatch(body: HandlersUpdateIssueRequest, successMessage?: string) {
  try {
    await updateIssueMutation(body)
    if (successMessage) toast.success(successMessage)
    invalidateIssue()
    return true
  }
  catch (err) {
    toast.error(resolveApiErrorMessage(err, t('teams.issueUpdateFailed')))
    return false
  }
}

function saveTitle() {
  const title = titleDraft.value.trim()
  if (!title) return
  void saveIssuePatch({ title }, t('teams.issueUpdated')).then((ok) => {
    if (ok) editingTitle.value = false
  })
}

function cancelTitleEdit() {
  titleDraft.value = issue.value?.title ?? ''
  editingTitle.value = false
}

function saveDescription() {
  void saveIssuePatch({ description: descriptionDraft.value.trim() }, t('teams.issueUpdated')).then((ok) => {
    if (ok) editingDescription.value = false
  })
}

function cancelDescriptionEdit() {
  descriptionDraft.value = issue.value?.description ?? ''
  editingDescription.value = false
}

function updateStatus(status: IssueStatus) {
  if (!issue.value || issue.value.status === status) return
  void saveIssuePatch({ status }, t('teams.issueUpdated'))
}

async function updateAssignee(value: string) {
  if (value === assigneeValueFromIssue.value) return
  try {
    await assignIssueMutation(value)
    toast.success(t('teams.issueUpdated'))
    invalidateIssue()
  }
  catch (err) {
    toast.error(resolveApiErrorMessage(err, t('teams.issueUpdateFailed')))
  }
}

async function submitComment() {
  if (!newComment.value.trim()) return
  try {
    await postCommentMutation()
    newComment.value = ''
    void refetchComments()
    invalidateIssue()
  }
  catch (err) {
    toast.error(resolveApiErrorMessage(err, t('teams.commentFailed')))
  }
}

async function deleteIssue() {
  try {
    await deleteIssueMutation()
    showDelete.value = false
    void queryCache.invalidateQueries({ key: ['team', teamId.value, 'issues'] })
    router.push({ name: 'team-workspace', params: { teamId: teamId.value } })
  }
  catch (err) {
    toast.error(resolveApiErrorMessage(err, t('teams.issueDeleteFailed')))
  }
}

function invalidateIssue() {
  void queryCache.invalidateQueries({ key: ['team', teamId.value, 'issue', issueId.value] })
  void queryCache.invalidateQueries({ key: ['team', teamId.value, 'issues'] })
}

function openBotSession(botID: string, sessionID: string) {
  if (!botID || !sessionID) return
  router.push({ name: 'chat', params: { botId: botID }, query: { session: sessionID } })
}

function openMention(member: HandlersMemberResponse) {
  if (member.member_type === 'bot' && member.bot_id) {
    router.push({ name: 'bot-detail', params: { botId: member.bot_id } })
    return
  }
  selectedMember.value = member
  showMemberDialog.value = true
}

function memberValue(member: HandlersMemberResponse) {
  if (member.member_type === 'bot') return `bot:${member.bot_id ?? ''}`
  return `user:${member.user_id ?? ''}`
}

function parseAssigneeValue(value: string) {
  if (value.startsWith('bot:')) {
    return { assignee_type: 'bot', assignee_bot_id: value.slice(4) }
  }
  if (value.startsWith('user:')) {
    return { assignee_type: 'user', assignee_user_id: value.slice(5) }
  }
  return {}
}

function memberLabel(member: HandlersMemberResponse) {
  return member.display_name || member.bot_id || member.user_id || t('teams.unknownMember')
}

function memberAvatar(member: HandlersMemberResponse) {
  if (!member.bot_id) return ''
  return bots.value.find((bot) => bot.id === member.bot_id)?.avatar_url ?? ''
}

function commentAuthorLabel(comment: HandlersCommentResponse) {
  if (comment.author_bot_id) {
    const member = members.value.find((item) => item.bot_id === comment.author_bot_id)
    return member ? memberLabel(member) : botName(comment.author_bot_id)
  }
  if (comment.author_user_id) {
    const member = members.value.find((item) => item.user_id === comment.author_user_id)
    return member ? memberLabel(member) : comment.author_user_id
  }
  return comment.author_type || t('teams.unknownMember')
}

function commentAuthorAvatar(comment: HandlersCommentResponse) {
  if (!comment.author_bot_id) return ''
  return bots.value.find((bot) => bot.id === comment.author_bot_id)?.avatar_url ?? ''
}

function botName(botID: string | undefined) {
  if (!botID) return t('teams.unknownMember')
  return bots.value.find((bot) => bot.id === botID)?.display_name || botID
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

function formatDate(value: string | undefined): string {
  if (!value) return ''
  try {
    return new Date(value).toLocaleString()
  }
  catch {
    return value
  }
}

function normalizeStatus(status: string | undefined): IssueStatus {
  if (ISSUE_COLUMNS.some((column) => column.value === status)) {
    return status as IssueStatus
  }
  return 'todo'
}

function statusLabel(status: string | undefined) {
  const column = ISSUE_COLUMNS.find((item) => item.value === status)
  return column ? t(column.labelKey) : status || ''
}

function badgeVariantForStatus(status: string | undefined): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'in_progress':
    case 'review':
      return 'default'
    case 'blocked':
      return 'destructive'
    case 'done':
    case 'cancelled':
      return 'outline'
    default:
      return 'secondary'
  }
}

function handoffBadgeVariant(status: string | undefined): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'failed':
      return 'destructive'
    case 'completed':
    case 'returned':
      return 'outline'
    case 'running':
    case 'dispatched':
      return 'default'
    default:
      return 'secondary'
  }
}
</script>
