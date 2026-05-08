<template>
  <section class="max-w-7xl mx-auto p-4 pb-12">
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
          <p class="text-xs font-medium truncate">
            {{ displayTitle }}
          </p>
          <p class="text-xs text-muted-foreground truncate">
            {{ displayUserID }}
          </p>
        </div>
      </div>

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
        :timezone="profileForm.timezone"
        :saving="savingProfile"
        :loading="loadingInitial"
        @update:display-name="profileForm.display_name = $event"
        @update:avatar-url="profileForm.avatar_url = $event"
        @update:timezone="profileForm.timezone = $event"
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
    </div>
  </section>
</template>

<script setup lang="ts">
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
  Button,
  Separator,
} from '@memohai/ui'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ProfileSection from './components/profile-section.vue'
import PasswordSection from './components/password-section.vue'
import { getUsersMe, putUsersMe, putUsersMePassword } from '@memohai/sdk'
import type { AccountsAccount, AccountsUpdateProfileRequest, AccountsUpdatePasswordRequest } from '@memohai/sdk'
import { useUserStore } from '@/store/user'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { useAvatarInitials } from '@/composables/useAvatarInitials'

type UserAccount = AccountsAccount

const { t } = useI18n()
const router = useRouter()
const userStore = useUserStore()
const { userInfo, exitLogin, patchUserInfo } = userStore

// ---- User data ----
const account = ref<UserAccount | null>(null)

const loadingInitial = ref(false)
const savingProfile = ref(false)
const savingPassword = ref(false)

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
  profileForm.display_name = data.display_name || ''
  profileForm.avatar_url = data.avatar_url || ''
  profileForm.timezone = data.timezone || 'UTC'
  patchUserInfo({
    id: data.id,
    username: data.username,
    role: data.role,
    displayName: data.display_name || '',
    avatarUrl: data.avatar_url || '',
    timezone: data.timezone || 'UTC',
  })
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
    profileForm.display_name = data.display_name || ''
    profileForm.avatar_url = data.avatar_url || ''
    profileForm.timezone = data.timezone || 'UTC'
    patchUserInfo({
      displayName: data.display_name || '',
      avatarUrl: data.avatar_url || '',
      timezone: data.timezone || 'UTC',
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
