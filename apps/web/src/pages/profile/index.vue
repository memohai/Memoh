<template>
  <PageShell :title="$t('sidebar.profile')">
    <div class="space-y-8">
      <!-- Stay-local (not SettingsRow): skeleton screens have no owner yet —
           skeleton-shimmer isn't on the four-rung loading ladder. These rows
           borrow SettingsRow's geometry (mx-4/border-b/py-3.5/last:border-b-0)
           only so the real content doesn't reflow once it swaps in. -->
      <template v-if="loadingInitial">
        <div class="overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-card">
          <div
            v-for="i in 5"
            :key="i"
            class="mx-4 flex items-center justify-between border-b border-border py-3.5 last:border-b-0"
          >
            <Skeleton class="h-4 w-24" />
            <Skeleton class="h-8 w-64" />
          </div>
        </div>
      </template>

      <template v-else>
        <!-- Card 1 · Profile: avatar + display name (hover-to-edit) only -->
        <SettingsSection>
          <ProfileIdentity
            :avatar-url="profileForm.avatar_url"
            :display-name="profileForm.display_name"
            :username="displayUsername"
            :fallback="avatarFallback"
            @update:avatar-url="profileForm.avatar_url = $event"
            @update:display-name="profileForm.display_name = $event"
            @save="autoSaveProfile"
          />
        </SettingsSection>

        <!-- Card 2 · Account: timezone + password -->
        <SettingsSection :title="$t('settings.accountSection')">
          <SettingsRow :label="$t('settings.timezone')">
            <div class="w-64">
              <TimezoneSelect
                :model-value="profileForm.timezone"
                :placeholder="$t('settings.timezonePlaceholder')"
                @update:model-value="onTimezoneChange"
              />
            </div>
          </SettingsRow>

          <SettingsRow :label="$t('auth.password')">
            <PasswordSection
              :open="passwordDialogOpen"
              :saving="savingPassword"
              @update:open="passwordDialogOpen = $event"
              @submit="onSubmitPassword"
            />
          </SettingsRow>
        </SettingsSection>

        <!-- Connected IM Accounts -->
        <ConnectedAccountsSection />

        <!-- Card 3 · Session: user id + sign out (low-frequency, kept at the bottom).
             Sign-out is hidden only in the local desktop shell, which auto-logs
             in via [admin] and has no login screen to return to. The remote
             desktop shell and the browser keep it available. -->
        <SettingsSection :title="$t('settings.sessionSection')">
          <SettingsRow :label="$t('settings.userID')">
            <div class="flex items-center gap-1 text-sm text-muted-foreground">
              <span class="max-w-[16rem] truncate font-mono text-xs">{{ displayUserID }}</span>
              <Button
                variant="ghost"
                size="icon-sm"
                :aria-label="$t('common.copy')"
                @click="copyToClipboard(displayUserID)"
              >
                <Check
                  v-if="copiedId"
                  class="size-3.5"
                />
                <Copy
                  v-else
                  class="size-3.5"
                />
              </Button>
            </div>
          </SettingsRow>

          <SettingsRow
            v-if="canSignOut"
            :label="$t('auth.logout')"
          >
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
          </SettingsRow>
        </SettingsSection>
      </template>
    </div>
  </PageShell>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import { Button, Skeleton } from '@memohai/ui'
import { Check, Copy } from 'lucide-vue-next'

import PageShell from '@/components/page-shell/index.vue'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import TimezoneSelect from '@/components/timezone-select/index.vue'
import SettingsRow from '@/components/settings/row.vue'
import SettingsSection from '@/components/settings/section.vue'
import ProfileIdentity from './components/profile-identity.vue'
import PasswordSection from './components/password-section.vue'
import ConnectedAccountsSection from './components/connected-accounts-section.vue'

import { getUsersMe, putUsersMe, putUsersMePassword } from '@memohai/sdk'
import type { AccountsAccount, AccountsUpdateProfileRequest, AccountsUpdatePasswordRequest } from '@memohai/sdk'
import { useUserStore } from '@/store/user'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { DesktopShellKey, DesktopRuntimeModeKey } from '@/lib/desktop-shell'

type UserAccount = AccountsAccount

const { t } = useI18n()
const router = useRouter()
const userStore = useUserStore()
const { userInfo, exitLogin, patchUserInfo } = userStore

// In the local desktop shell the app auto-logs in via [admin] and has no
// login screen to return to, so sign-out is hidden there. The remote desktop
// shell (and the browser) talks to a real server with user accounts, so
// sign-out stays available. Connected Accounts stays available either way.
const desktopShell = inject(DesktopShellKey, false)
const desktopRuntimeMode = inject(DesktopRuntimeModeKey, 'local')
const canSignOut = !desktopShell || desktopRuntimeMode === 'remote'

// ---- User data ----
const account = ref<UserAccount | null>(null)

const loadingInitial = ref(true)
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

const passwordDialogOpen = ref(false)
const copiedId = ref(false)

async function copyToClipboard(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    copiedId.value = true
    setTimeout(() => copiedId.value = false, 2000)
  } catch {
    toast.error(t('common.copyFailed'))
  }
}

const displayUserID = computed(() => account.value?.id || userInfo.id || '')
const displayUsername = computed(() => account.value?.username || userInfo.username || '')
const displayTitle = computed(() => {
  return profileForm.display_name.trim() || displayUsername.value || displayUserID.value || t('settings.user')
})
const avatarFallback = useAvatarInitials(() => displayTitle.value, 'U')

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

function onTimezoneChange(value: string | number | undefined) {
  profileForm.timezone = String(value || 'UTC')
  void autoSaveProfile()
}

// Monotonic token so out-of-order PUT responses can't clobber newer state: each
// save claims a token, and only the latest dispatch is allowed to apply its
// result or roll back on failure.
let saveToken = 0

// Silent auto-save: triggered on name confirm, timezone change, and avatar apply.
// Skips the request when nothing actually changed; only surfaces errors.
async function autoSaveProfile() {
  const body: AccountsUpdateProfileRequest = {
    display_name: profileForm.display_name.trim(),
    avatar_url: profileForm.avatar_url.trim(),
    timezone: profileForm.timezone.trim(),
  }
  if (
    body.display_name === originalProfile.display_name
    && body.avatar_url === originalProfile.avatar_url
    && body.timezone === originalProfile.timezone
  ) {
    return
  }

  const token = ++saveToken
  try {
    const { data } = await putUsersMe({ body, throwOnError: true })
    // A newer save has since been dispatched — let it own the final state.
    if (token !== saveToken) return
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
  } catch (error) {
    // Roll back the optimistic local edit so the UI re-matches the server —
    // but only if no newer save superseded this one (which would own state).
    if (token === saveToken) {
      profileForm.display_name = originalProfile.display_name
      profileForm.avatar_url = originalProfile.avatar_url
      profileForm.timezone = originalProfile.timezone
      patchUserInfo({
        displayName: originalProfile.display_name,
        avatarUrl: originalProfile.avatar_url,
        timezone: originalProfile.timezone,
      })
    }
    toast.error(resolveApiErrorMessage(error, t('settings.profileUpdateFailed'), { prefixFallback: true }))
  }
}

async function onSubmitPassword(payload: { currentPassword: string, newPassword: string }) {
  const currentPassword = payload.currentPassword.trim()
  const newPassword = payload.newPassword.trim()
  if (!currentPassword || !newPassword) {
    toast.error(t('settings.passwordRequired'))
    return
  }
  savingPassword.value = true
  try {
    const body: AccountsUpdatePasswordRequest = {
      current_password: currentPassword,
      new_password: newPassword,
    }
    await putUsersMePassword({ body, throwOnError: true })
    passwordDialogOpen.value = false
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
