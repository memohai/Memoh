<template>
  <SidebarInset class="grid grid-rows-[auto_auto_1fr]">
    <header
      class="flex h-16 shrink-0 items-center gap-2 transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-12"
    >
      <div class="flex items-center gap-2 px-4">     
        <SidebarTrigger class="-ml-1" />
        <Separator
          orientation="vertical"
          class="mr-2 data-[orientation=vertical]:h-4"
        />
        <Breadcrumb>
          <BreadcrumbList>
            <template
              v-for="(breadcrumbItem, index) in curBreadcrumb"
              :key="breadcrumbItem"
            >
              <template v-if="(index + 1) !== curBreadcrumb.length">
                <BreadcrumbItem class="hidden md:block">
                  <BreadcrumbLink :href="breadcrumbItem.path">
                    {{ breadcrumbItem.breadcrumb }}
                  </BreadcrumbLink>
                </BreadcrumbItem>
                <BreadcrumbSeparator />
              </template>

              <BreadcrumbItem v-else>
                <BreadcrumbPage>
                  {{ breadcrumbItem.breadcrumb }}
                </BreadcrumbPage>
              </BreadcrumbItem>
            </template>
          </BreadcrumbList>
        </Breadcrumb>
      </div>
    </header>
    <Separator />
    <section class="w-full relative">
      <h1 class="sr-only">
        {{ currentPageTitle }}
      </h1>
      <ScrollArea class="absolute! inset-0">
        <router-view
          v-slot="{ Component }"
          class="p-4"
        >
          <KeepAlive>
            <component :is="Component" />
          </KeepAlive>
        </router-view>
      </ScrollArea>
    </section>
  </SidebarInset>
</template>

<script setup lang="ts">
import {
  SidebarTrigger, SidebarInset, Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
  Separator,
  ScrollArea,
} from '@memoh/ui'
import { useRoute } from 'vue-router'
import { computed } from 'vue'

const route = useRoute()

const curBreadcrumb = computed(() => {
  return route.matched
    .filter(routeItem => routeItem.meta['breadcrumb'])
    .map(routeItem => {
      const raw = routeItem.meta['breadcrumb']
      return {
        path: routeItem.path,
        breadcrumb: typeof raw === 'function' ? raw(route) : raw,
      }
    })
})

const currentPageTitle = computed(() => {
  const last = curBreadcrumb.value[curBreadcrumb.value.length - 1]
  const title = String(last?.breadcrumb ?? '').trim()
  return title || 'Memoh'
})

</script>
