<template>
  <Card class="h-full flex flex-col">
    <CardHeader>
      <CardTitle class="text-muted-foreground flex justify-between">
        <span>{{ $t('platform.platformLabel') }}: {{ platform.name }}</span>
        <Badge
          v-if="platform.active"
          variant="outline"
        >
          {{ $t('platform.running') }}
        </Badge>
      </CardTitle>
      <CardContent class="px-0 pb-0">
        <ol class="space-y-2 text-sm">
          <li
            v-for="(value, key) in platform.config"
            :key="key"
          >
            {{ key }}: {{ value }}
          </li>
        </ol>
      </CardContent>
    </CardHeader>
    <CardFooter class="flex gap-4 mt-auto">
      <Switch
        :model-value="platform.active"
        :aria-label="`Toggle ${platform.name}`"
      />
      <Button
        class="ml-auto"
        @click="$emit('edit', platform)"
      >
        {{ $t('common.edit') }}
      </Button>
      <Button
        variant="destructive"
        @click="$emit('delete', platform)"
      >
        {{ $t('common.delete') }}
      </Button>
    </CardFooter>
  </Card>
</template>

<script setup lang="ts">
import {
  Card,
  CardHeader,
  CardFooter,
  CardContent,
  CardTitle,
  Switch,
  Button,
  Badge,
} from '@memoh/ui'

defineProps<{
  platform: {
    name: string
    active: boolean
    config: Record<string, string>
  }
}>()

defineEmits<{
  edit: [platform: unknown]
  delete: [platform: unknown]
}>()
</script>
