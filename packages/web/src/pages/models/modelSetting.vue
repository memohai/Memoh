<template>
  <div class="p-4  **:[input]:mt-3 **:[input]:mb-4">
    <section class="flex justify-between items-center ">
      <h4 class="scroll-m-20   tracking-tight">
        {{ curProvider?.name }}
      </h4>
      <Switch />
    </section>
    <Separator class="mt-4 mb-6" />
    <form>
      <section>
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          API 密钥
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="username"
        >
          <FormItem>
            <FormControl>
              <Input
                type="text"
                placeholder="请输入API密钥"
              />
            </FormControl>       
          </FormItem>
        </FormField>
      </section>
    
      <section>
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          URL
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="username"
        >
          <FormItem>
            <FormControl>
              <Input
                type="text"
                placeholder="请输入URL"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>
      <section class="flex justify-end mt-4">
        <Button>确定修改</Button>
      </section>
    </form>
    <Separator class="mt-4 mb-6" />
    <section>
      <section class="flex justify-between items-center mb-4 ">
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          模型
        </h4>
        <CreateModel />
      </section>
      <section class="flex flex-col gap-4">
        <Item variant="outline">
          <ItemContent>
            <ItemTitle>Deep Seek R1</ItemTitle>
            <ItemDescription>
              <Badge variant="secondary">
                Chat
              </Badge>
            </ItemDescription>
          </ItemContent>
          <ItemActions>
            <Button
              variant="outline"
              size="sm"
            >
              编辑
            </Button>
          </ItemActions>
        </Item>
        <Item variant="outline">
          <ItemContent>
            <ItemTitle>Deep Seek R1</ItemTitle>
            <ItemDescription>
              <Badge variant="secondary">
                Chat
              </Badge>
            </ItemDescription>
          </ItemContent>
          <ItemActions>
            <Button
              variant="outline"
              size="sm"
            >
              编辑
            </Button>
          </ItemActions>
        </Item>
        <Item variant="outline">
          <ItemContent>
            <ItemTitle>Deep Seek R1</ItemTitle>
            <ItemDescription>
              <Badge variant="secondary">
                Chat
              </Badge>              
            </ItemDescription>
          </ItemContent>
          <ItemActions>
            <Button
              variant="outline"
              size="sm"
            >
              编辑
            </Button>
          </ItemActions>
        </Item>
      </section>
    </section>
  </div>
</template>

<script setup lang="ts">
import {
  Switch, Separator, InputGroup, Input, Button,
  FormControl,
  FormField,
  FormItem,
  Item,
  ItemContent,
  ItemDescription,
  ItemActions,
  ItemTitle,
  Badge
} from '@memoh/ui'
import CreateModel from '@/components/CreateModel/index.vue'
import { inject,ref } from 'vue'
import {type ModelList } from '@memoh/shared'

const curProvider=inject('curProvider',ref<Partial<ModelList>>())
</script>