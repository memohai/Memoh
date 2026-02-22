<template>
  <main class="w-screen h-screen flex *:m-auto bg-background relative">
    <header class="absolute top-6 right-6 flex items-center gap-2">
      <Select
        :model-value="language"
        @update:model-value="(v) => v && setLanguage(v as Locale)"
      >
        <SelectTrigger
          class="w-28 h-9"
          :aria-label="$t('settings.language')"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem value="en">
              English
            </SelectItem>
            <SelectItem value="zh">
              中文
            </SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>
      <Button
        variant="ghost"
        size="icon"
        type="button"
        aria-label="Toggle theme"
        @click="toggleTheme"
      >
        <Sun
          v-if="theme === 'dark'"
          class="size-5"
        />
        <Moon
          v-else
          class="size-5"
        />
      </Button>
    </header>
    <section class="w-full max-w-sm flex flex-col gap-10 ">
      <section>
        <h1
          class="scroll-m-20 text-3xl tracking-wide font-semibold text-foreground text-center"
        >
          {{ $t('auth.welcome') }}
        </h1>
      </section>
      <form
        @submit="login"
      >
        <Card class="py-14">
          <CardContent class="flex flex-col [&_input]:py-5 gap-4">
            <FormField
              v-slot="{ componentField }"
              name="username"
            >
              <FormItem>
                <Label
                  class="mb-2"
                  for="username"
                >
                  {{ $t('auth.username') }}
                </Label>
                <FormControl>
                  <Input
                    v-bind="componentField"
                    id="username"
                    type="text"
                    :placeholder="$t('auth.username')"
                    autocomplete="new-password"
                  />
                </FormControl>
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="password"
            >
              <FormItem>
                <Label
                  class="mb-2"
                  for="password"
                >
                  {{ $t('auth.password') }}
                </Label>
                <FormControl>
                  <Input
                    id="password"
                    type="password"
                    :placeholder="$t('auth.password')"
                    autocomplete="new-password"
                    v-bind="componentField"
                  />
                </FormControl>
              </FormItem>
            </FormField>
            <div class="flex">
              <a
                href="#"
                class="ml-auto inline-block text-sm underline mt-2"
              >
                {{ $t('auth.forgotPassword') }}
              </a>
            </div>
          </CardContent>

          <CardFooter>
            <Button
              class="w-full"
              type="submit"
              @click="login"
            >
              <Spinner v-if="loading" />
              {{ $t('auth.login') }}
            </Button>
          </CardFooter>
        </Card>
      </form>
    </section>
  </main>
</template>

<script setup lang="ts">
import {
  Card,
  CardContent,
  CardFooter,
  Input,
  Button,
  FormControl,
  FormField,
  FormItem,
  Label,
  Spinner,
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@memoh/ui'
import { Sun, Moon } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import { toTypedSchema } from '@vee-validate/zod'
import { useForm } from 'vee-validate'
import * as z from 'zod'
import { useUserStore } from '@/store/user'
import { useSettingsStore } from '@/store/settings'
import { ref } from 'vue'
import { storeToRefs } from 'pinia'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { postAuthLogin } from '@memoh/sdk'
import type { Locale } from '@/i18n'

const router = useRouter()
const { t } = useI18n()
const settingsStore = useSettingsStore()
const { theme, language } = storeToRefs(settingsStore)
const { setLanguage, setTheme } = settingsStore

const toggleTheme = () => {
  setTheme(theme.value === 'light' ? 'dark' : 'light')
}

const formSchema = toTypedSchema(z.object({
  username: z.string().min(1),
  password: z.string().min(1),
}))
const form = useForm({
  validationSchema: formSchema,
})

const { login: loginHandle } = useUserStore()
const loading = ref(false)

const login = form.handleSubmit(async (values) => {
  try {
    loading.value = true
    const { data } = await postAuthLogin({ body: values })
    if (data?.access_token && data?.user_id) {
      loginHandle({
        id: data.user_id,
        username: data.username,
        displayName: data.display_name ?? '',
        role: data.role ?? '',
        avatarUrl: data.avatar_url ?? '',
      }, data.access_token)
    } else {
      throw new Error(t('auth.loginFailed'))
    }
    router.replace({ path: '/chat' })
  } catch {
    toast.error(t('auth.invalidCredentials'), {
      description: t('auth.retryHint'),
    })
  } finally {
    loading.value = false
  }
})
</script>
