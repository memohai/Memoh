<template>
  <div
    class="flex flex-col h-full shrink-0 border-l border-border bg-sidebar transition-[width] duration-200"
    :class="sidebarOpen ? 'w-[240px]' : 'w-10'"
  >
    <div class="h-12 flex items-center shrink-0 px-3">
      <button
        class="shrink-0"
        @click="sidebarOpen = !sidebarOpen"
      >
        <img
          src="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMTgiIGhlaWdodD0iMTgiIHZpZXdCb3g9IjAgMCAxOCAxOCIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPHBhdGggZD0iTTE2LjUgOUMxNi41IDYuMTg3NyAxNi41IDQuNzgxNTUgMTUuNzgzOCAzLjc5NTgxQzE1LjU1MjUgMy40Nzc0NSAxNS4yNzI2IDMuMTk3NDggMTQuOTU0MiAyLjk2NjE5QzEzLjk2ODQgMi4yNSAxMi41NjIzIDIuMjUgOS43NSAyLjI1SDguMjVDNS40Mzc3IDIuMjUgNC4wMzE1NSAyLjI1IDMuMDQ1ODEgMi45NjYxOUMyLjcyNzQ1IDMuMTk3NDggMi40NDc0OCAzLjQ3NzQ1IDIuMjE2MTkgMy43OTU4MUMxLjUgNC43ODE1NSAxLjUgNi4xODc3IDEuNSA5QzEuNSAxMS44MTIzIDEuNSAxMy4yMTg0IDIuMjE2MTkgMTQuMjA0MkMyLjQ0NzQ4IDE0LjUyMjUgMi43Mjc0NSAxNC44MDI1IDMuMDQ1ODEgMTUuMDMzOEM0LjAzMTU1IDE1Ljc1IDUuNDM3NyAxNS43NSA4LjI1IDE1Ljc1SDkuNzVDMTIuNTYyMyAxNS43NSAxMy45Njg0IDE1Ljc1IDE0Ljk1NDIgMTUuMDMzOEMxNS4yNzI2IDE0LjgwMjUgMTUuNTUyNSAxNC41MjI1IDE1Ljc4MzggMTQuMjA0MkMxNi41IDEzLjIxODQgMTYuNSAxMS44MTIzIDE2LjUgOVoiIHN0cm9rZT0iIzBBMEEwQSIgc3Ryb2tlLXdpZHRoPSIxLjUiIHN0cm9rZS1saW5lam9pbj0icm91bmQiLz4KPHBhdGggZD0iTTEwLjg3NSAyLjYyNUwxMC44NzUgMTUuMzc1IiBzdHJva2U9IiMwQTBBMEEiIHN0cm9rZS13aWR0aD0iMS41IiBzdHJva2UtbGluZWpvaW49InJvdW5kIi8+CjxwYXRoIGQ9Ik0xNC4yNSA1LjI1SDEzLjEyNSIgc3Ryb2tlPSIjMEEwQTBBIiBzdHJva2Utd2lkdGg9IjEuNSIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBzdHJva2UtbGluZWpvaW49InJvdW5kIi8+CjxwYXRoIGQ9Ik0xNC4yNSA4LjI1SDEzLjEyNSIgc3Ryb2tlPSIjMEEwQTBBIiBzdHJva2Utd2lkdGg9IjEuNSIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBzdHJva2UtbGluZWpvaW49InJvdW5kIi8+CjxwYXRoIGQ9Ik02IDcuNUw2LjkxOTkxIDguMjkyODlDNy4zMDY2MyA4LjYyNjIzIDcuNSA4Ljc5Mjg5IDcuNSA5QzcuNSA5LjIwNzExIDcuMzA2NjQgOS4zNzM3NyA2LjkxOTkxIDkuNzA3MTFMNiAxMC41IiBzdHJva2U9IiMwQTBBMEEiIHN0cm9rZS13aWR0aD0iMS41IiBzdHJva2UtbGluZWNhcD0icm91bmQiIHN0cm9rZS1saW5lam9pbj0icm91bmQiLz4KPC9zdmc+Cg=="
          alt="toggle"
          class="size-[18px] dark:invert"
        >
      </button>
      <h3
        v-if="sidebarOpen"
        class="font-medium text-xs ml-2"
      >
        Session 源数据
      </h3>
    </div>
    <Separator />

    <template v-if="sidebarOpen">
      <ScrollArea class="flex-1">
        <form
          class="p-3 **:[label]:text-xs **:[label]:uppercase **:[label]:text-muted-foreground **:[label]:tracking-wide"
        >
          <section class="flex flex-col gap-6">
            <FormField
              v-slot="{ componentField }"
              name="bot"
            >
              <FormItem>
                <FormLabel>Bot</FormLabel>
                <FormControl>
                  <InputGroup>
                    <InputGroupInput v-bind="componentField" />
                    <InputGroupAddon>
                      <FontAwesomeIcon
                        :icon="['fas', 'wand-magic-sparkles']"
                        class="size-3.5 text-muted-foreground"
                      />
                    </InputGroupAddon>
                  </InputGroup>
                </FormControl>
                <FormMessage />
              </FormItem>
            </FormField>

            <FormField
              v-slot="{ componentField }"
              name="skills"
            >
              <FormItem>
                <FormLabel>Top Skills</FormLabel>
                <FormControl>
                  <InputGroup>
                    <InputGroupInput v-bind="componentField" />
                    <InputGroupAddon>
                      <FontAwesomeIcon
                        :icon="['fas', 'circle']"
                        class="size-1.5 text-muted-foreground"
                      />
                    </InputGroupAddon>
                  </InputGroup>
                  <InputGroup>
                    <InputGroupInput v-bind="componentField" />
                    <InputGroupAddon>
                      <FontAwesomeIcon
                        :icon="['fas', 'circle']"
                        class="size-1.5 text-muted-foreground"
                      />
                    </InputGroupAddon>
                  </InputGroup>
                  <InputGroup>
                    <InputGroupInput v-bind="componentField" />
                    <InputGroupAddon>
                      <FontAwesomeIcon
                        :icon="['fas', 'circle']"
                        class="size-1.5 text-muted-foreground"
                      />
                    </InputGroupAddon>
                  </InputGroup>
                </FormControl>
                <p class="text-xs text-muted-foreground mt-4">
                  <FontAwesomeIcon
                    :icon="['fas', 'sliders']"
                    class="size-3 mr-1"
                  />
                  管理全部 Skills(5)
                </p>
              </FormItem>
            </FormField>

            <section>
              <h5 class="text-xs text-muted-foreground mb-2 uppercase tracking-wide">
                用量统计
              </h5>
              <Card class="p-4!">
                <CardContent class="p-0 flex flex-col gap-3 text-xs text-muted-foreground">
                  <div class="flex justify-between">
                    <span>
                      <FontAwesomeIcon
                        :icon="['fas', 'arrow-up']"
                        class="size-2.5 mr-1"
                      />输入Token
                    </span>
                    <data>4,281</data>
                  </div>
                  <div class="flex justify-between">
                    <span>
                      <FontAwesomeIcon
                        :icon="['fas', 'arrow-down']"
                        class="size-2.5 mr-1"
                      />输出Token
                    </span>
                    <data>2,104</data>
                  </div>
                  <div class="flex justify-between">
                    <span>
                      <FontAwesomeIcon
                        :icon="['fas', 'coins']"
                        class="size-2.5 mr-1"
                      />总成本
                    </span>
                    <data>$12.00</data>
                  </div>
                </CardContent>
              </Card>
            </section>

            <Separator />

            <section class="flex flex-col items-start gap-1">
              <Button
                variant="link"
                class="text-muted-foreground text-xs p-0! h-auto"
              >
                <FontAwesomeIcon
                  :icon="['fas', 'book']"
                  class="size-3 mr-1"
                />
                帮助文档
              </Button>
              <Button
                variant="link"
                class="text-destructive text-xs p-0! h-auto"
              >
                <FontAwesomeIcon
                  :icon="['fas', 'trash']"
                  class="size-3 mr-1"
                />
                删除 Session
              </Button>
            </section>
          </section>
        </form>
      </ScrollArea>
    </template>

    <template v-else>
      <div
        class="flex-1 flex justify-center pt-4"
        style="writing-mode: sideways-rl"
      >
        <span class="text-xs text-muted-foreground">
          Session 源数据
        </span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import {
  ScrollArea,
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
} from '@memohai/ui'
const sidebarOpen = ref(true)
</script>
