<template>
  <main class="flex flex-col flex-1 min-w-0 overflow-hidden bg-card">
    <section class="flex-1 min-h-0 overflow-hidden">
      <router-view v-slot="{ Component }">
        <KeepAlive>
          <component :is="Component" />
        </KeepAlive>
      </router-view>
    </section>
  </main>
</template>
