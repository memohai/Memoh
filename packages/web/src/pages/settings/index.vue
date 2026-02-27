<template>
  <section class="h-full max-w-7xl mx-auto p-4">
    <div class="max-w-3xl mx-auto space-y-8">
      <!-- Avatar & name -->
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
          <p class="font-semibold truncate">
            {{ displayTitle }}
          </p>
          <p class="text-sm text-muted-foreground truncate">
            {{ displayUserID }}
          </p>
        </div>
      </div>

      <!-- Display Settings -->
      <section>
        <h2 class="mb-2 flex items-center text-base font-semibold">
          <FontAwesomeIcon
            :icon="['fas', 'gear']"
            class="mr-2"
          />
          {{ $t('settings.display') }}
        </h2>
        <Separator />
        <div class="mt-4 space-y-4">
          <div class="flex items-center justify-between">
            <Label>{{ $t('settings.language') }}</Label>
            <Select
              :model-value="language"
              @update:model-value="(v) => v && setLanguage(v as Locale)"
            >
              <SelectTrigger
                class="w-40"
                :aria-label="$t('settings.language')"
              >
                <SelectValue :placeholder="$t('settings.languagePlaceholder')" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="zh">
                    {{ $t('settings.langZh') }}
                  </SelectItem>
                  <SelectItem value="en">
                    {{ $t('settings.langEn') }}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          <Separator />
          <div class="flex items-center justify-between">
            <Label>{{ $t('settings.theme') }}</Label>
            <Select
              :model-value="theme"
              @update:model-value="(v) => v && setTheme(v as 'light' | 'dark')"
            >
              <SelectTrigger
                class="w-40"
                :aria-label="$t('settings.theme')"
              >
                <SelectValue :placeholder="$t('settings.themePlaceholder')" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="light">
                    {{ $t('settings.themeLight') }}
                  </SelectItem>
                  <SelectItem value="dark">
                    {{ $t('settings.themeDark') }}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
        </div>
      </section>

      <!-- Logout -->
      <section>
        <Separator class="mb-4" />
        <ConfirmPopover
          :message="$t('auth.logoutConfirm')"
          @confirm="onLogout"
        >
          <template #trigger>
            <Button>
              {{ $t('auth.logout') }}
            </Button>
          </template>
        </ConfirmPopover>
      </section>

      <ProfileSection
        :display-user-id="displayUserID"
        :display-username="displayUsername"
        :display-name="profileForm.display_name"
        :avatar-url="profileForm.avatar_url"
        :saving="savingProfile"
        :loading="loadingInitial"
        @update:display-name="profileForm.display_name = $event"
        @update:avatar-url="profileForm.avatar_url = $event"
        @save="onSaveProfile"
      />

      <PasswordSection
        :current-password="passwordForm.currentPassword"
        :new-password="passwordForm.newPassword"
        :confirm-password="passwordForm.confirmPassword"
        :saving="savingPassword"
        :loading="loadingInitial"
        @update:current-password="passwordForm.currentPassword = $event"
        @update:new-password="passwordForm.newPassword = $event"
        @update:confirm-password="passwordForm.confirmPassword = $event"
        @update-password="onUpdatePassword"
      />

      <!-- Linked Channels -->
      <section>
        <h2 class="mb-2 flex items-center text-base font-semibold">
          <FontAwesomeIcon
            :icon="['fas', 'network-wired']"
            class="mr-2"
          />
          {{ $t('settings.linkedChannels') }}
        </h2>
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

      <BindCodeSection
        :any-platform-value="anyPlatformValue"
        :platform="bindForm.platform"
        :platform-options="platformOptions"
        :ttl-seconds="bindForm.ttlSeconds"
        :generating="generatingBindCode"
        :loading="loadingInitial"
        :bind-code="bindCode"
        :platform-label="platformLabel"
        :format-date="formatDate"
        @update:platform="onPlatformChange"
        @update:ttl-seconds="bindForm.ttlSeconds = $event"
        @generate="onGenerateBindCode"
        @copy="copyBindCode"
      />
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
  Label,
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Separator,
} from '@memoh/ui'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ProfileSection from './components/profile-section.vue'
import PasswordSection from './components/password-section.vue'
import BindCodeSection from './components/bind-code-section.vue'
import { getUsersMe, putUsersMe, putUsersMePassword, getUsersMeIdentities } from '@memoh/sdk'
import { client } from '@memoh/sdk/client'
import type { AccountsAccount, AccountsUpdateProfileRequest, AccountsUpdatePasswordRequest } from '@memoh/sdk'
import { useUserStore } from '@/store/user'
import { useSettingsStore } from '@/store/settings'
import type { Locale } from '@/i18n'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { formatDateTime } from '@/utils/date-time'
import { useClipboard } from '@/composables/useClipboard'
import { useAvatarInitials } from '@/composables/useAvatarInitials'

interface ChannelIdentity {
  id: string
  user_id?: string
  channel: string
  channel_subject_id: string
  display_name?: string
  metadata?: Record<string, unknown>
  created_at: string
  updated_at: string
}

interface IssueBindCodeResponse {
  token: string
  platform?: string
  expires_at: string
}

type UserAccount = AccountsAccount

const anyPlatformValue = '__all__'

const { t } = useI18n()
const router = useRouter()
const userStore = useUserStore()
const { copyText } = useClipboard()
const { userInfo, exitLogin, patchUserInfo } = userStore

// ---- Display settings ----
const settingsStore = useSettingsStore()
const { language, theme } = storeToRefs(settingsStore)
const { setLanguage, setTheme } = settingsStore

// ---- User data ----
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
  return profileForm.display_name.trim() || displayUsername.value || displayUserID.value || t('settings.user')
})
const avatarFallback = useAvatarInitials(() => displayTitle.value, 'U')

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
  const { data } = await getUsersMe({ throwOnError: true })
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
    const { data } = await getUsersMeIdentities({ throwOnError: true })
    identities.value = data.items ?? []
  } finally {
    loadingIdentities.value = false
  }
}

async function onSaveProfile() {
  savingProfile.value = true
  try {
    const body: AccountsUpdateProfileRequest = {
      display_name: profileForm.display_name.trim(),
      avatar_url: profileForm.avatar_url.trim(),
    }
    const { data } = await putUsersMe({ body, throwOnError: true })
    account.value = data
    profileForm.display_name = data.display_name || ''
    profileForm.avatar_url = data.avatar_url || ''
    patchUserInfo({
      displayName: data.display_name || '',
      avatarUrl: data.avatar_url || '',
    })
    toast.success(t('settings.profileUpdated'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('settings.profileUpdateFailed'), { prefixFallback: true }))
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
    const body: AccountsUpdatePasswordRequest = {
      current_password: currentPassword,
      new_password: newPassword,
    }
    await putUsersMePassword({ body, throwOnError: true })
    passwordForm.currentPassword = ''
    passwordForm.newPassword = ''
    passwordForm.confirmPassword = ''
    toast.success(t('settings.passwordUpdated'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('settings.passwordUpdateFailed'), { prefixFallback: true }))
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
    const { data } = await client.post<IssueBindCodeResponse>({
      url: '/users/me/bind_codes',
      body: {
        platform: bindForm.platform || undefined,
        ttl_seconds: ttl,
      },
      throwOnError: true,
    })
    bindCode.value = data
    toast.success(t('settings.bindCodeGenerated'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('settings.bindCodeGenerateFailed'), { prefixFallback: true }))
  } finally {
    generatingBindCode.value = false
  }
}

async function copyBindCode() {
  if (!bindCode.value?.token) {
    return
  }
  try {
    const copied = await copyText(bindCode.value.token)
    if (copied) {
      toast.success(t('settings.bindCodeCopied'))
      return
    }
    toast.error(t('settings.bindCodeCopyFailed'))
  } catch {
    toast.error(t('settings.bindCodeCopyFailed'))
  }
}

function formatDate(value: string) {
  return formatDateTime(value, { fallback: value })
}

function onLogout() {
  exitLogin()
  void router.replace({ name: 'Login' })
}

</script>
