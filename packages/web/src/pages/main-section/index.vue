<template>
  <section>
    <MainLayout>
      <template #sidebar>
        <SideBar />
      </template>
      <template #main>
        <MainContainer />
      </template>
    </MainLayout>
  </section>
</template>

<script setup lang="ts">
import MainLayout from '@/layout/main-layout/index.vue'
import SideBar from '@/components/sidebar/index.vue'
import MainContainer from '@/components/main-container/index.vue'
import { provide,ref } from 'vue'


provide('sideBarIsOpen',ref(true))
</script>