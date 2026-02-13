<template>
  <section class="w-screen h-screen flex *:m-auto bg-linear-to-t from-[#BFA4A0] to-[#7784AC] ">
    <section class="w-full max-w-sm flex flex-col gap-10 ">
      <section>
        <h3
          class="scroll-m-20 text-3xl tracking-wide font-semibold  text-white text-center"
          style="font-family: 'Source Han Serif CN', 'Noto Serif SC', 'STSong', 'SimSun', serif;"
        >
          {{ $t('auth.welcome') }}
        </h3>
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

          <CardFooter class="flex flex-col gap-4">
            <Button
              class="w-full"
              type="submit"
              @click="login"
            >
              <Spinner v-if="loading" />
              {{ $t('auth.login') }}
            </Button>
            <Button
              variant="outline"
              class="w-full"
            >
              {{ $t('auth.register') }}
            </Button>
          </CardFooter>
        </Card>
      </form>
    </section>
  </section>
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
} from '@memoh/ui'
import { useRouter } from 'vue-router'
import { toTypedSchema } from '@vee-validate/zod'
import { useForm } from 'vee-validate'
import * as z from 'zod'
import { useUserStore } from '@/store/user'
import { ref } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { login as loginApi } from '@/composables/api/useAuth'

const router = useRouter()
const { t } = useI18n()

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
    const data = await loginApi(values)
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
    router.replace({ name: 'Main' })
  } catch {
    toast.error(t('auth.invalidCredentials'), {
      description: t('auth.retryHint'),
    })
  } finally {
    loading.value = false
  }
})
</script>
