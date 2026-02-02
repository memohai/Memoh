<template>
  <section class="w-screen h-screen flex *:m-auto bg-linear-to-t from-[#BFA4A0] to-[#7784AC] ">
    <section class="w-full max-w-sm flex flex-col gap-10 ">
      <section>
        <img
          src="../../../public/logo.png"
          width="100"
          alt="logo.png"
          class="m-auto"
        >
        <h3 class="scroll-m-20 text-2xl font-semibold tracking-tight text-white text-center">
          Memoh
        </h3>
      </section>
      <form @submit="login">
        <Card class="py-14">
          <CardContent class="flex flex-col [&_input]:py-5">
            <FormField
              v-slot="{ componentField }"
              name="username"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  {{ $t("login.username") }}
                </FormLabel>
                <FormControl>
                  <Input
                    type="text"
                    :placeholder="$t('prompt.enter', { msg: $t(`login.username`).toLocaleLowerCase() })"
                    v-bind="componentField"
                    autocomplete="username"
                  />
                </FormControl>
                <blockquote class="h-5">
                  <FormMessage />
                </blockquote>
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="password"
            >
              <FormItem>
                <FormLabel class="mb-2">
                  {{ $t('login.password') }}
                </FormLabel>
                <FormControl>
                  <Input
                    type="password"
                    :placeholder="$t('prompt.enter', { msg: $t(`login.password`).toLocaleLowerCase() })"
                    autocomplete="password"
                    v-bind="componentField"
                  />
                </FormControl>
                <blockquote class="h-5">
                  <FormMessage />
                </blockquote>
              </FormItem>
            </FormField>
            <div class="flex">
              <a
                href="#"
                class="ml-auto inline-block text-sm underline mt-2"
              >
                {{ $t('login.forget') }}
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
              {{ $t("login.login") }}
            </Button>
            <Button
              variant="outline"
              class="w-full"
            >
              {{ $t("login.register") }}
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
  FormLabel,
  FormMessage,
  Spinner
} from '@memoh/ui'
import { useRouter } from 'vue-router'
import { toTypedSchema } from '@vee-validate/zod'
import { useForm } from 'vee-validate'
import * as z from 'zod'
import request from '@/utils/request'
import { useUserStore } from '@/store/User.ts'
import { ref } from 'vue'
import { toast } from 'vue-sonner'
const router = useRouter()
const formSchema = toTypedSchema(z.object({
  username: z.string().min(1),
  password: z.string().min(1),
}))
const form = useForm({
  validationSchema: formSchema,
})

const { login: LoginHandle } = useUserStore()
const loading = ref(false)
const login = form.handleSubmit(async (values) => {
  try {
    loading.value = true
    const loginState = await request({
      url: '/auth/login',
      method: 'post',
      data: { ...values }
    }, false)
    const data = loginState?.data
    if (data?.access_token && data?.user_id) {
      LoginHandle({
        id: data?.user_id,
        username: data?.username,
        displayName: '',
        role: ''
      }, data.access_token)
    } else {
      throw new Error('用户名和密码错误')
    }
    router.replace({
      name: 'Main'
    })
  } catch (error) {
    toast.error('用户名或密码错误', {
      description: '请重新输入用户名和密码',
    })
    return error
  } finally {
    loading.value = false
  }
})


</script>