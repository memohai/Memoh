<template>
  <section class="max-w-7xl mx-auto px-4 pt-2 pb-10 md:px-6 md:pt-4 md:pb-12 min-h-[calc(100vh-4rem)] flex flex-col">
    <!-- Inner Wrapper limits reading width -->
    <div class="max-w-3xl mx-auto w-full flex-1">
      <!-- Page Header -->
      <header class="flex items-start justify-between pb-4 border-b border-border/50 mb-6">
        <template v-if="loadingInitial">
          <div class="flex items-center gap-4">
            <Skeleton class="size-14 rounded-full" />
            <div class="space-y-2">
              <Skeleton class="h-5 w-[150px]" />
              <Skeleton class="h-3 w-[100px]" />
            </div>
          </div>
        </template>
        <template v-else>
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
              <h1 class="text-lg font-semibold truncate">
                {{ displayTitle }}
              </h1>
              <p class="text-xs text-muted-foreground mt-1">
                {{ $t('settings.userProfile') }}
              </p>
            </div>
          </div>
        </template>
      </header>

      <!-- Content Area -->
      <div class="space-y-6">
        <!-- Skeleton Loading State -->
        <template v-if="loadingInitial">
          <div class="rounded-md border p-6 space-y-6">
            <div class="grid gap-6 sm:grid-cols-2">
              <div class="space-y-2">
                <Skeleton class="h-3 w-16" /><Skeleton class="h-9 w-full" />
              </div>
              <div class="space-y-2">
                <Skeleton class="h-3 w-16" /><Skeleton class="h-9 w-full" />
              </div>
              <div class="space-y-2">
                <Skeleton class="h-3 w-16" /><Skeleton class="h-9 w-full" />
              </div>
              <div class="space-y-2">
                <Skeleton class="h-3 w-16" /><Skeleton class="h-9 w-full" />
              </div>
            </div>
          </div>
        </template>

        <!-- Main Config Area -->
        <template v-else>
          <!-- Profile Card -->
          <ProfileSection
            :display-user-id="displayUserID"
            :display-username="displayUsername"
            :display-name="profileForm.display_name"
            :avatar-url="profileForm.avatar_url"
            :timezone="profileForm.timezone"
            :saving="savingProfile"
            :loading="loadingInitial"
            :is-dirty="isProfileDirty"
            @update:display-name="profileForm.display_name = $event"
            @update:avatar-url="profileForm.avatar_url = $event"
            @update:timezone="profileForm.timezone = $event"
            @save="onSaveProfile"
            @discard="onDiscardProfile"
          />

          <!-- Password Card -->
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

          <!-- Connected IM Accounts -->
          <ConnectedAccountsSection />

          <!-- Sign Out Zone (hidden in the desktop shell: it auto-logs in and has no login screen to return to) -->
          <div
            v-if="!desktopShell"
            class="pt-6"
          >
            <div class="rounded-md border bg-background shadow-sm">
              <div class="flex items-center justify-between p-4 md:p-6">
                <div>
                  <h3 class="text-sm font-medium">
                    {{ $t('auth.logout') }}
                  </h3>
                  <p class="text-xs text-muted-foreground mt-1">
                    End your current session securely.
                  </p>
                </div>
                <ConfirmPopover
                  :message="$t('auth.logoutConfirm')"
                  @confirm="onLogout"
                >
                  <template #trigger>
                    <Button
                      variant="outline"
                      size="sm"
                    >
                      {{ $t('auth.logout') }}
                    </Button>
                  </template>
                </ConfirmPopover>
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { Button, Skeleton, Avatar, AvatarFallback, AvatarImage } from '@memohai/ui'

import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ProfileSection from './components/profile-section.vue'
import PasswordSection from './components/password-section.vue'
import ConnectedAccountsSection from './components/connected-accounts-section.vue'

import { getUsersMe, putUsersMe, putUsersMePassword } from '@memohai/sdk'
import type { AccountsAccount, AccountsUpdateProfileRequest, AccountsUpdatePasswordRequest } from '@memohai/sdk'
import { useUserStore } from '@/store/user'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { DesktopShellKey } from '@/lib/desktop-shell'

type UserAccount = AccountsAccount

const { t } = useI18n()
const router = useRouter()
const userStore = useUserStore()
const { userInfo, exitLogin, patchUserInfo } = userStore

// In the desktop shell the app auto-logs in and has no login screen, so the
// sign-out action is hidden there. Connected Accounts stays available.
const desktopShell = inject(DesktopShellKey, false)

// ---- User data ----
const account = ref<UserAccount | null>(null)

const loadingInitial = ref(true)
const savingProfile = ref(false)
const savingPassword = ref(false)

const originalProfile = reactive({
  display_name: '',
  avatar_url: '',
  timezone: '',
})

const profileForm = reactive({
  display_name: '',
  avatar_url: '',
  timezone: '',
})

const passwordForm = reactive({
  currentPassword: '',
  newPassword: '',
  confirmPassword: '',
})

const displayUserID = computed(() => account.value?.id || userInfo.id || '')
const displayUsername = computed(() => account.value?.username || userInfo.username || '')
const displayTitle = computed(() => {
  return profileForm.display_name.trim() || displayUsername.value || displayUserID.value || t('settings.user')
})
const avatarFallback = useAvatarInitials(() => displayTitle.value, 'U')

const isProfileDirty = computed(() => {
  return profileForm.display_name !== originalProfile.display_name ||
         profileForm.avatar_url !== originalProfile.avatar_url ||
         profileForm.timezone !== originalProfile.timezone
})

onMounted(() => {
  void loadPageData()
})

async function loadPageData() {
  loadingInitial.value = true
  try {
    await loadMyAccount()
  } catch {
    toast.error(t('settings.loadUserFailed'))
  } finally {
    loadingInitial.value = false
  }
}

async function loadMyAccount() {
  const { data } = await getUsersMe({ throwOnError: true })
  account.value = data
  
  const dName = data.display_name || ''
  const aUrl = data.avatar_url || ''
  const tZone = data.timezone || 'UTC'

  // Set forms
  profileForm.display_name = dName
  profileForm.avatar_url = aUrl
  profileForm.timezone = tZone
  
  // Set originals
  originalProfile.display_name = dName
  originalProfile.avatar_url = aUrl
  originalProfile.timezone = tZone

  patchUserInfo({
    id: data.id,
    username: data.username,
    role: data.role,
    displayName: dName,
    avatarUrl: aUrl,
    timezone: tZone,
  })
}

function onDiscardProfile() {
  profileForm.display_name = originalProfile.display_name
  profileForm.avatar_url = originalProfile.avatar_url
  profileForm.timezone = originalProfile.timezone
}

async function onSaveProfile() {
  savingProfile.value = true
  try {
    const body: AccountsUpdateProfileRequest = {
      display_name: profileForm.display_name.trim(),
      avatar_url: profileForm.avatar_url.trim(),
      timezone: profileForm.timezone.trim(),
    }
    const { data } = await putUsersMe({ body, throwOnError: true })
    account.value = data
    
    const dName = data.display_name || ''
    const aUrl = data.avatar_url || ''
    const tZone = data.timezone || 'UTC'

    profileForm.display_name = dName
    profileForm.avatar_url = aUrl
    profileForm.timezone = tZone

    originalProfile.display_name = dName
    originalProfile.avatar_url = aUrl
    originalProfile.timezone = tZone

    patchUserInfo({
      displayName: dName,
      avatarUrl: aUrl,
      timezone: tZone,
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

function onLogout() {
  exitLogin()
  void router.replace({ name: 'Login' })
}
</script>
