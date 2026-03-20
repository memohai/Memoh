<template>
  <section>
    <SidebarMenuButton
      as-child
      class="justify-start py-5! px-4"
      :disabled="bot.status==='error'"
    >
      <Toggle       
      
        :class="`p-2.75! border hover:border-border hover:bg-(--bot-item)!  ${isActive?'border-border bg-(--bot-item)!':'border-transparent bg-transparent!'} h-[initial]!  ${currentBotId === bot.id ? 'border-inherit' : ''}`"
        :model-value="isActive"
        @click="handleSelect(bot)"
      >
        <section
          v-if="bot.status==='loading'"
          class="flex gap-2 overflow-hidden w-full items-center "
        >
          <ChatStatus />
          <section class="flex flex-col gap-0.5 flex-1">
            <h5 class="text-xs flex ">
              <span class="truncate flex-1 max-w-30">
                {{ bot.display_name || bot.id }}
              </span>
             
              <time
                class="ml-auto flex-none"
                :datetime="create_date"
              >{{ create_date }}</time>
            </h5>
           
            <p class="text-xs text-muted-foreground min-w-0 ">
              <i class="w-1.25 aspect-square bg-blue-500 rounded-full inline-block" />
              推理中
            </p>
          </section>
        </section>
        <section
          v-if="bot.status === 'ready'"
          class="flex gap-2 overflow-hidden w-full"
        >
          <ChatStatus :is-loading="false" />
          <section class="flex flex-col gap-0.5  flex-1">
            <section>
              <h5 class="text-xs flex flex-1">
                <span class="truncate flex-1 max-w-30">
                  {{ bot.display_name || bot.id }}
                </span>
                <time
                  class="ml-auto "
                  :datetime="create_date"
                >{{ create_date }}</time>
              </h5>
            </section>
           
            <p class="text-xs text-muted-foreground min-w-0 "> 
              fewfwefewfewfewfwf 
            </p>
          </section>
        </section>
        <section
          v-if="bot.status === 'error'"
          class="flex gap-2 overflow-hidden w-full"
        >
          <ChatStatus
            :is-loading="false"
            :error="bot.status==='error'"
          />
          <section class="flex flex-col gap-0.5 flex-1">
            <h5 class="text-xs flex">
              <span class="truncate flex-1 max-w-30">
                {{ bot.display_name || bot.id }}
              </span>         
              <time
                class="ml-auto"
                :datetime="create_date"
              >{{ create_date }}</time>
            </h5>
            <p class="text-xs text-muted-foreground min-w-0 ">
              fewfwefewfewfewfwf
            </p>
          </section>
        </section>
        <section
          v-if="bot.status === 'no-setting'"
          class="flex gap-2 overflow-hidden w-full "
        >
          <ChatStatus
            :is-loading="false"
          />
          <section class="self-center flex-1">
            <h5 class="text-xs flex">
              <span class="truncate flex-1 max-w-30">
                {{ bot.display_name || bot.id }}
              </span>
            
              <time
                class="ml-auto"
                :datetime="create_date"
              >{{ create_date }}</time>
            </h5>          
          </section>
        </section>
      </Toggle>
      <!-- <Toggle
        v-if="bot.status === 'ready'"
        :class="`p-2.75! border  border-border h-[initial]! bg-(--bot-item)! ${currentBotId === bot.id ? 'border-inherit' : ''}`"
        :model-value="isActive(bot.id as string).value"
        @click="handleSelect(bot)"
      >
        <section class="flex gap-2 overflow-hidden w-full">
          <ChatStatus />
          <section class="flex flex-col gap-0.5">
            <h5 class="text-xs">
              {{ bot.display_name || bot.id }}
            </h5>
            <p class="text-xs text-muted-foreground min-w-0 ">
              <i class="w-1.25 aspect-square bg-blue-500 rounded-full inline-block" />
              推理中
            </p>
          </section>
        </section>
      </Toggle> -->
    </SidebarMenuButton>
  </section>
</template>

<script setup lang="ts">
import type { BotsBot } from '@memoh/sdk'
import {  computed, onMounted, watchEffect } from 'vue'
import { storeToRefs } from 'pinia'
import { useChatStore } from '@/store/chat-list'
import ChatStatus from '@/components/chat/chat-status/index.vue'
import {
  Toggle,
  SidebarMenuButton,
} from '@memoh/ui'
import moment from 'moment'

const {bot}=defineProps<{bot:BotsBot}>()
const chatStore = useChatStore()

const create_date = computed(() => {
  return moment(bot?.updated_at??Date.now()).format('hh:ss')
})

const { currentBotId } = storeToRefs(chatStore)

const isActive =computed(() => {
  return currentBotId.value === bot.id
})

function handleSelect(bot: BotsBot) {
  chatStore.selectBot(bot.id)
}

</script>