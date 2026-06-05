<script setup lang="ts">
// Navigation. Sidebar is rendered inside SidebarProvider and a fixed-height
// box with collapsible="none" so it lays out inline instead of grabbing the
// viewport.
import { ref } from 'vue'
import {
  Breadcrumb, BreadcrumbEllipsis, BreadcrumbItem, BreadcrumbLink,
  BreadcrumbList, BreadcrumbPage, BreadcrumbSeparator,
  Pagination, PaginationContent, PaginationEllipsis, PaginationFirst,
  PaginationItem, PaginationLast, PaginationNext, PaginationPrevious,
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent,
  SidebarGroupLabel, SidebarHeader, SidebarMenu, SidebarMenuAction, SidebarMenuBadge,
  SidebarMenuButton, SidebarMenuItem, SidebarMenuSkeleton, SidebarMenuSub,
  SidebarMenuSubButton, SidebarMenuSubItem, SidebarProvider, SidebarSeparator, SidebarTrigger,
  Tabs, TabsContent, TabsList, TabsTrigger,
} from '@memohai/ui'
import { Home, Inbox, MoreHorizontal, Settings } from 'lucide-vue-next'
import SectionShell from '../components/SectionShell.vue'
import Specimen from '../components/Specimen.vue'

const currentPage = ref(2)
</script>

<template>
  <SectionShell
    id="navigation"
    label="Navigation"
    description="Breadcrumbs, pagination, tabs, and the sidebar system."
  >
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <Specimen label="<Breadcrumb>">
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink href="#">
                Home
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbEllipsis />
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbLink href="#">
                Settings
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>Appearance</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </Specimen>

      <Specimen label="<Pagination>">
        <Pagination
          :total="100"
          :items-per-page="10"
          :sibling-count="1"
          :page="currentPage"
          show-edges
          @update:page="currentPage = $event"
        >
          <PaginationContent v-slot="{ items }">
            <PaginationFirst />
            <PaginationPrevious />
            <template
              v-for="(item, index) in items"
              :key="index"
            >
              <PaginationEllipsis
                v-if="item.type === 'ellipsis'"
                :index="index"
              />
              <PaginationItem
                v-else
                :value="item.value"
                :is-active="item.value === currentPage"
              />
            </template>
            <PaginationNext />
            <PaginationLast />
          </PaginationContent>
        </Pagination>
      </Specimen>

      <div class="lg:col-span-2">
        <Specimen label="<Tabs>">
          <Tabs
            default-value="account"
            class="w-full max-w-md"
          >
            <TabsList>
              <TabsTrigger value="account">
                Account
              </TabsTrigger>
              <TabsTrigger value="password">
                Password
              </TabsTrigger>
              <TabsTrigger value="team">
                Team
              </TabsTrigger>
            </TabsList>
            <TabsContent
              value="account"
              class="text-sm text-muted-foreground"
            >
              Account panel content.
            </TabsContent>
            <TabsContent
              value="password"
              class="text-sm text-muted-foreground"
            >
              Password panel content.
            </TabsContent>
            <TabsContent
              value="team"
              class="text-sm text-muted-foreground"
            >
              Team panel content.
            </TabsContent>
          </Tabs>
        </Specimen>
      </div>

      <div class="lg:col-span-2">
        <Specimen
          label="<Sidebar> (SidebarProvider, collapsible=none)"
          note="full menu/sub/badge/action/skeleton/trigger composition"
        >
          <div class="h-80 w-full overflow-hidden rounded-lg border border-border">
            <SidebarProvider
              class="!min-h-0 h-full items-stretch"
              :style="{ '--sidebar-width': '14rem' }"
            >
              <Sidebar
                collapsible="none"
                class="h-full border-r border-sidebar-border"
              >
                <SidebarHeader class="px-3 py-2 text-sm font-semibold">
                  Memoh
                </SidebarHeader>
                <SidebarSeparator />
                <SidebarContent>
                  <SidebarGroup>
                    <SidebarGroupLabel>Platform</SidebarGroupLabel>
                    <SidebarGroupContent>
                      <SidebarMenu>
                        <SidebarMenuItem>
                          <SidebarMenuButton is-active>
                            <Home />
                            <span>Home</span>
                          </SidebarMenuButton>
                          <SidebarMenuBadge>3</SidebarMenuBadge>
                        </SidebarMenuItem>
                        <SidebarMenuItem>
                          <SidebarMenuButton>
                            <Inbox />
                            <span>Inbox</span>
                          </SidebarMenuButton>
                          <SidebarMenuAction>
                            <MoreHorizontal />
                          </SidebarMenuAction>
                          <SidebarMenuSub>
                            <SidebarMenuSubItem>
                              <SidebarMenuSubButton>Unread</SidebarMenuSubButton>
                            </SidebarMenuSubItem>
                            <SidebarMenuSubItem>
                              <SidebarMenuSubButton>Archived</SidebarMenuSubButton>
                            </SidebarMenuSubItem>
                          </SidebarMenuSub>
                        </SidebarMenuItem>
                        <SidebarMenuItem>
                          <SidebarMenuButton>
                            <Settings />
                            <span>Settings</span>
                          </SidebarMenuButton>
                        </SidebarMenuItem>
                        <SidebarMenuSkeleton show-icon />
                      </SidebarMenu>
                    </SidebarGroupContent>
                  </SidebarGroup>
                </SidebarContent>
                <SidebarFooter class="px-3 py-2 text-xs text-muted-foreground">
                  v0.9.0
                </SidebarFooter>
              </Sidebar>
              <main class="flex flex-1 items-start gap-2 p-3">
                <SidebarTrigger />
                <span class="text-xs text-muted-foreground">Inset content area</span>
              </main>
            </SidebarProvider>
          </div>
        </Specimen>
      </div>
    </div>
  </SectionShell>
</template>
