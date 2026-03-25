<template>
  <section :class="mountNode.id">
    <Teleport :to="mountNode.rightDefault">
      <SidebarProvider :open="sidebarOpen">
        <SidebarInset>
          <Sidebar
            class="absolute! bg-red-500 overflow-hidden"
            side="right"
            collapsible="icon"
          >
            <SidebarHeader class="h-12 flex flex-rows justify-center">
              <section class="flex items-center gap-2">
                <FontAwesomeIcon
                  :icon="['fas', 'bars']"
                  class="group-data-[collapsible=icon]:m-auto"
                  @click="sidebarOpen=!sidebarOpen"
                />
                <h3 class="font-semibold text-sm group-data-[collapsible=icon]:hidden">
                  Session 源数据
                </h3>
              </section>
            </SidebarHeader>
            <Separator />
            <section
              class="not-group-data-[collapsible=icon]:hidden flex justify-center"
              style="writing-mode:sideways-rl"
            >
              <span class="flex-none w-[1em] mr-2.5 mt-4 text-muted-foreground text-sm">
                Session 源数据
              </span>
            </section>
            <SidebarContent class="group-data-[collapsible=icon]:invisible">
              <SidebarGroup>
                <SidebarGroupContent>
                  <SidebarMenu>
                    <form
                      class="**:[label]:text-xs **:[label]:uppercase **:[label]:text-muted-foreground **:[label]:tracking-wide "
                    >
                      <section class="flex flex-col gap-6">
                        <FormField
                          v-slot="{ componentField }"
                          name="username"
                        >
                          <FormItem>
                            <FormLabel>
                              Bot
                            </FormLabel>
                            <FormControl>
                              <InputGroup>
                                <InputGroupInput v-bind="componentField" />
                                <InputGroupAddon>
                                  <FontAwesomeIcon :icon="['fas', 'robot']" />
                                </InputGroupAddon>
                              </InputGroup>
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        </FormField>
                        <FormField
                          v-slot="{ componentField }"
                          name="username"
                        >
                          <FormItem>
                            <FormLabel>
                              Top Skills
                            </FormLabel>
                            <FormControl>
                              <InputGroup>
                                <InputGroupInput v-bind="componentField" />
                                <InputGroupAddon>
                                  <FontAwesomeIcon
                                    :icon="['fas', 'circle']"
                                    class="w-1.5!"
                                  />
                                </InputGroupAddon>
                              </InputGroup>
                              <InputGroup>
                                <InputGroupInput v-bind="componentField" />
                                <InputGroupAddon>
                                  <FontAwesomeIcon
                                    :icon="['fas', 'circle']"
                                    class="w-1.5!"
                                  />
                                </InputGroupAddon>
                              </InputGroup>
                              <InputGroup>
                                <InputGroupInput v-bind="componentField" />
                                <InputGroupAddon>
                                  <FontAwesomeIcon
                                    :icon="['fas', 'circle']"
                                    class="w-1.5!"
                                  />
                                </InputGroupAddon>
                              </InputGroup>
                            </FormControl>
                            <p class="text-xs text-muted-foreground mt-4">
                              <FontAwesomeIcon :icon="['fas', 'arrows-split-up-and-left']" />
                              管理全部 Skills(5)
                            </p>
                          </FormItem>
                        </FormField>

                        <section>
                          <h5 class="text-xs text-muted-foreground mb-2">
                            用量统计
                          </h5>
                          <Card class="p-4!">
                            <CardContent
                              class="p-0 flex flex-col **:[section]:text-xs gap-3 text-muted-foreground *:flex *:justify-between "
                            >
                              <section>
                                <h5>输入Token</h5>
                                <data>4,281</data>
                              </section>
                              <section>
                                <h5>输出Token</h5>
                                <data>2,104</data>
                              </section>
                              <section>
                                <h5>总成本</h5>
                                <data>$12.00</data>
                              </section>
                            </CardContent>
                          </Card>
                        </section>
                        <Separator class="" />
                        <section class="flex flex-col items-start *:p-0! ">
                          <Button
                            variant="link"
                            class="text-muted-foreground text-xs cursor-pointer"
                          >
                            <FontAwesomeIcon :icon="['fas', 'book-open']" />
                            帮助文档
                          </Button>
                          <Button
                            variant="link"
                            class="text-muted-foreground text-xs cursor-pointer text-destructive"
                          >
                            <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                            删除 Session
                          </Button>
                        </section>
                      </section>
                    </form>
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            </SidebarContent>
          </Sidebar>
        </SidebarInset>
      </SidebarProvider>
    </Teleport>
    <section class="hidden-clip-section" />
  </section>
</template>

<script setup lang="ts">
// import { computed } from 'vue'
// import { useI18n } from 'vue-i18n'

// import { useQuery } from '@pinia/colada'
// import { getBotsQuery } from '@memoh/sdk/colada'
// import type { BotsBot } from '@memoh/sdk'
// import { useChatStore } from '@/store/chat-list'
// import { storeToRefs } from 'pinia'
import {
  SidebarHeader,
  SidebarProvider,
  SidebarContent,
  Sidebar,
  SidebarInset,
  Separator,
  SidebarMenu,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  InputGroup,
  InputGroupAddon,  
  InputGroupInput, 
  Card,
  CardContent,
  Button,
  SidebarGroup,
  SidebarGroupContent
} from '@memohai/ui'
import useControlVisibleStatus from '@/utils/useControlVisibleStatus'
import { ref } from 'vue'

const sidebarOpen=ref(true)

// const chatStore = useChatStore()
// const { currentBotId } = storeToRefs(chatStore)

// const { data: botData } = useQuery(getBotsQuery())
// const bots = computed<BotsBot[]>(() => botData.value?.items ?? [])

// const isActive = (id: string) => computed(() => {
//   return currentBotId.value === id
// })

// function handleSelect(bot: BotsBot) {
//   chatStore.selectBot(bot.id)
// }

const mountNode = useControlVisibleStatus()
</script>
