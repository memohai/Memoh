<template>
  <SidebarInset>
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
    <main class="flex flex-1 flex-col gap-4 pt-0 ">
      <router-view v-slot="{ Component }">
        <KeepAlive>
          <component :is="Component" />
        </KeepAlive>
      </router-view>
    </main>
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
  // DropdownMenu,
  // DropdownMenuContent,
  // DropdownMenuItem,
  // DropdownMenuTrigger,
} from '@memoh/ui'
import { useRoute } from 'vue-router'
import { computed } from 'vue'
// import SvgIcon from '@jamescoyle/vue-icon'
// import { mdiTranslate } from '@mdi/js'

const route = useRoute()

const curBreadcrumb = computed(() => {
  return route.matched.map(routeItem => ({
    path: routeItem.path,
    breadcrumb: routeItem.meta['breadcrumb']
  }))
})

</script>