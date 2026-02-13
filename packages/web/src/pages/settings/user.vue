<template>
  <section class="h-full max-w-7xl mx-auto p-6">
    <div class="max-w-3xl mx-auto space-y-8">
      <div class="flex items-center gap-4">
        <Avatar class="size-14 shrink-0">
          <AvatarImage
            v-if="profileForm.avatar_url"
            :src="profileForm.avatar_url"
            :alt="displayTitle"
          />
          <AvatarFallback>
            {{ avatarFallback }}
          </AvatarFallback>
        </Avatar>
        <div class="min-w-0">
          <h4 class="font-semibold truncate">
            {{ displayTitle }}
          </h4>
          <p class="text-sm text-muted-foreground truncate">
            {{ displayUserID }}
          </p>
        </div>
      </div>

      <section>
        <h6 class="mb-2 flex items-center">
          <FontAwesomeIcon
            :icon="['fas', 'user']"
            class="mr-2"
          />
          {{ $t('settings.userProfile') }}
        </h6>
        <Separator />
        <div class="mt-4 space-y-4">
          <div class="space-y-2">
            <Label>{{ $t('settings.userID') }}</Label>
            <Input
              :model-value="displayUserID"
              readonly
            />
          </div>
          <div class="space-y-2">
            <Label>{{ $t('auth.username') }}</Label>
            <Input
              :model-value="displayUsername"
              readonly
            />
          </div>
          <div class="space-y-2">
            <Label>{{ $t('settings.displayName') }}</Label>
            <Input v-model="profileForm.display_name" />
          </div>
          <div class="space-y-2">
            <Label>{{ $t('settings.avatarUrl') }}</Label>
            <Input
              v-model="profileForm.avatar_url"
              type="url"
            />
          </div>
          <div class="flex justify-end">
            <Button
              :disabled="savingProfile || loadingInitial"
              @click="onSaveProfile"
            >
              <Spinner v-if="savingProfile" />
              {{ $t('settings.saveProfile') }}
            </Button>
          </div>
        </div>
      </section>

      <section>
        <h6 class="mb-2 flex items-center">
          <FontAwesomeIcon
            :icon="['fas', 'gear']"
            class="mr-2"
          />
          {{ $t('settings.changePassword') }}
        </h6>
        <Separator />
        <div class="mt-4 space-y-4">
          <div class="space-y-2">
            <Label>{{ $t('settings.currentPassword') }}</Label>
            <Input
              v-model="passwordForm.currentPassword"
              type="password"
            />
          </div>
          <div class="space-y-2">
            <Label>{{ $t('settings.newPassword') }}</Label>
            <Input
              v-model="passwordForm.newPassword"
              type="password"
            />
          </div>
          <div class="space-y-2">
            <Label>{{ $t('settings.confirmPassword') }}</Label>
            <Input
              v-model="passwordForm.confirmPassword"
              type="password"
            />
          </div>
          <div class="flex justify-end">
            <Button
              :disabled="savingPassword || loadingInitial"
              @click="onUpdatePassword"
            >
              <Spinner v-if="savingPassword" />
              {{ $t('settings.updatePassword') }}
            </Button>
          </div>
        </div>
      </section>

      <section>
        <h6 class="mb-2 flex items-center">
          <FontAwesomeIcon
            :icon="['fas', 'network-wired']"
            class="mr-2"
          />
          {{ $t('settings.linkedChannels') }}
        </h6>
        <Separator />
        <div class="mt-4 space-y-3">
          <p
            v-if="loadingIdentities"
            class="text-sm text-muted-foreground"
          >
            {{ $t('common.loading') }}
          </p>
          <p
            v-else-if="identities.length === 0"
            class="text-sm text-muted-foreground"
          >
            {{ $t('settings.noLinkedChannels') }}
          </p>
          <template v-else>
            <div
              v-for="identity in identities"
              :key="identity.id"
              class="border rounded-md p-3 space-y-1"
            >
              <div class="flex items-center justify-between gap-3">
                <p class="font-medium truncate">
                  {{ identity.display_name || identity.channel_subject_id }}
                </p>
                <Badge variant="secondary">
                  {{ platformLabel(identity.channel) }}
                </Badge>
              </div>
              <p class="text-xs text-muted-foreground truncate">
                {{ identity.channel_subject_id }}
              </p>
              <p class="text-xs text-muted-foreground truncate">
                {{ identity.id }}
              </p>
            </div>
          </template>
        </div>
      </section>

      <section>
        <h6 class="mb-2 flex items-center">
          <FontAwesomeIcon
            :icon="['fas', 'plug']"
            class="mr-2"
          />
          {{ $t('settings.bindCode') }}
        </h6>
        <Separator />
        <div class="mt-4 space-y-4">
          <div class="flex flex-wrap gap-3 items-end">
            <div class="space-y-2">
              <Label>{{ $t('settings.platform') }}</Label>
              <Select
                :model-value="bindForm.platform || anyPlatformValue"
                @update:model-value="onPlatformChange"
              >
                <SelectTrigger class="w-56">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem :value="anyPlatformValue">
                      {{ $t('settings.platformAny') }}
                    </SelectItem>
                    <SelectItem
                      v-for="platform in platformOptions"
                      :key="platform"
                      :value="platform"
                    >
                      {{ platformLabel(platform) }}
                    </SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-2">
              <Label>{{ $t('settings.bindCodeTTL') }}</Label>
              <Input
                v-model.number="bindForm.ttlSeconds"
                type="number"
                min="60"
                class="w-40"
              />
            </div>
            <Button
              :disabled="generatingBindCode || loadingInitial"
              @click="onGenerateBindCode"
            >
              <Spinner v-if="generatingBindCode" />
              {{ $t('settings.generateBindCode') }}
            </Button>
          </div>
          <div
            v-if="bindCode"
            class="space-y-2"
          >
            <Label>{{ $t('settings.bindCodeValue') }}</Label>
            <div class="flex gap-2">
              <Input
                :model-value="bindCode.token"
                readonly
              />
              <Button
                variant="outline"
                @click="copyBindCode"
              >
                {{ $t('settings.copyBindCode') }}
              </Button>
            </div>
            <p class="text-xs text-muted-foreground">
              {{ $t('settings.bindCodeExpiresAt') }}: {{ formatDate(bindCode.expires_at) }}
            </p>
          </div>
        </div>
      </section>

      <section>
        <Separator class="mb-4" />
        <ConfirmPopover
          :message="$t('auth.logoutConfirm')"
          @confirm="onLogout"
        >
          <template #trigger>
            <Button variant="outline">
              {{ $t('auth.logout') }}
            </Button>
          </template>
        </ConfirmPopover>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
  Badge,
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Separator,
  Spinner,
} from '@memoh/ui'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import {
  getMyAccount,
  issueMyBindCode,
  listMyIdentities,
  updateMyPassword,
  updateMyProfile,
  type ChannelIdentity,
  type IssueBindCodeResponse,
  type UserAccount,
} from '@/composables/api/useUsers'
import { ApiError } from '@/utils/request'
import { useUserStore } from '@/store/user'

const anyPlatformValue = '__all__'

const { t } = useI18n()
const router = useRouter()
const userStore = useUserStore()
const { userInfo, exitLogin, patchUserInfo } = userStore

const account = ref<UserAccount | null>(null)
const identities = ref<ChannelIdentity[]>([])
const bindCode = ref<IssueBindCodeResponse | null>(null)

const loadingInitial = ref(false)
const loadingIdentities = ref(false)
const savingProfile = ref(false)
const savingPassword = ref(false)
const generatingBindCode = ref(false)

const profileForm = reactive({
  display_name: '',
  avatar_url: '',
})

const passwordForm = reactive({
  currentPassword: '',
  newPassword: '',
  confirmPassword: '',
})

const bindForm = reactive({
  platform: '',
  ttlSeconds: 3600,
})

const displayUserID = computed(() => account.value?.id || userInfo.id || '')
const displayUsername = computed(() => account.value?.username || userInfo.username || '')
const displayTitle = computed(() => {
  return profileForm.display_name.trim() || displayUsername.value || displayUserID.value || 'User'
})
const avatarFallback = computed(() => {
  const source = displayTitle.value.trim()
  return source.slice(0, 2).toUpperCase() || 'U'
})

function platformLabel(platformKey: string): string {
  if (!platformKey?.trim()) return platformKey ?? ''
  const key = platformKey.trim().toLowerCase()
  const i18nKey = `bots.channels.types.${key}`
  const out = t(i18nKey)
  return out !== i18nKey ? out : platformKey
}

const platformOptions = computed(() => {
  const options = new Set<string>(['telegram', 'feishu'])
  for (const identity of identities.value) {
    const platform = identity.channel.trim()
    if (platform) {
      options.add(platform)
    }
  }
  return Array.from(options)
})

onMounted(() => {
  void loadPageData()
})

async function loadPageData() {
  loadingInitial.value = true
  try {
    await Promise.all([loadMyAccount(), loadMyIdentities()])
  } catch {
    toast.error(t('settings.loadUserFailed'))
  } finally {
    loadingInitial.value = false
  }
}

async function loadMyAccount() {
  const data = await getMyAccount()
  account.value = data
  profileForm.display_name = data.display_name || ''
  profileForm.avatar_url = data.avatar_url || ''
  patchUserInfo({
    id: data.id,
    username: data.username,
    role: data.role,
    displayName: data.display_name || '',
    avatarUrl: data.avatar_url || '',
  })
}

async function loadMyIdentities() {
  loadingIdentities.value = true
  try {
    const data = await listMyIdentities()
    identities.value = data.items ?? []
  } finally {
    loadingIdentities.value = false
  }
}

async function onSaveProfile() {
  savingProfile.value = true
  try {
    const data = await updateMyProfile({
      display_name: profileForm.display_name.trim(),
      avatar_url: profileForm.avatar_url.trim(),
    })
    account.value = data
    profileForm.display_name = data.display_name || ''
    profileForm.avatar_url = data.avatar_url || ''
    patchUserInfo({
      displayName: data.display_name || '',
      avatarUrl: data.avatar_url || '',
    })
    toast.success(t('settings.profileUpdated'))
  } catch (error) {
    toast.error(resolveErrorMessage(error, t('settings.profileUpdateFailed')))
  } finally {
    savingProfile.value = false
  }
}

async function onUpdatePassword() {
  const currentPassword = passwordForm.currentPassword.trim()
  const newPassword = passwordForm.newPassword.trim()
  const confirmPassword = passwordForm.confirmPassword.trim()
  if (!currentPassword || !newPassword) {
    toast.error(t('settings.passwordRequired'))
    return
  }
  if (newPassword !== confirmPassword) {
    toast.error(t('settings.passwordNotMatch'))
    return
  }
  savingPassword.value = true
  try {
    await updateMyPassword({
      current_password: currentPassword,
      new_password: newPassword,
    })
    passwordForm.currentPassword = ''
    passwordForm.newPassword = ''
    passwordForm.confirmPassword = ''
    toast.success(t('settings.passwordUpdated'))
  } catch (error) {
    toast.error(resolveErrorMessage(error, t('settings.passwordUpdateFailed')))
  } finally {
    savingPassword.value = false
  }
}

function onPlatformChange(value: string) {
  bindForm.platform = value === anyPlatformValue ? '' : value
}

async function onGenerateBindCode() {
  generatingBindCode.value = true
  try {
    const ttl = Number.isFinite(bindForm.ttlSeconds) ? Math.max(60, Number(bindForm.ttlSeconds)) : 3600
    bindCode.value = await issueMyBindCode({
      platform: bindForm.platform || undefined,
      ttl_seconds: ttl,
    })
    toast.success(t('settings.bindCodeGenerated'))
  } catch (error) {
    toast.error(resolveErrorMessage(error, t('settings.bindCodeGenerateFailed')))
  } finally {
    generatingBindCode.value = false
  }
}

async function copyBindCode() {
  if (!bindCode.value?.token) {
    return
  }
  try {
    await navigator.clipboard.writeText(bindCode.value.token)
    toast.success(t('settings.bindCodeCopied'))
  } catch {
    toast.error(t('settings.bindCodeCopyFailed'))
  }
}

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return date.toLocaleString()
}

function onLogout() {
  exitLogin()
  void router.replace({ name: 'Login' })
}

function resolveErrorMessage(error: unknown, fallback: string) {
  if (error instanceof ApiError && error.body && typeof error.body === 'object') {
    const body = error.body as { message?: string; error?: string; detail?: string }
    const detail = body.message || body.error || body.detail
    if (detail) {
      return `${fallback}: ${detail}`
    }
  }
  return fallback
}
</script>
