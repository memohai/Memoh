<template>
  <section>
    <section class="max-w-187 m-auto">
      <Card>
        <CardHeader>
          <CardTitle class="text-2xl font-semibold tracking-tight">
            Settings
          </CardTitle>
          <CardDescription>
            Model Settings
          </CardDescription>
        </CardHeader>
        <!-- <CardHeader>
          <CardTitle>
            Setting
          </CardTitle>
        </CardHeader> -->
      </Card>
    </section>
  </section>
</template>

<script setup lang="ts">
import { useQuery } from '@pinia/colada'
import request from '@/utils/request'
import { watch } from 'vue'

import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle
} from '@memoh/ui'

const { data: settingData } = useQuery({
  key: ['Setting'],
  query: () => request({
    url: '/settings/',
    method: 'get'
  })
})

watch(settingData, () => {
  console.log(settingData.value?.data)
})
</script>