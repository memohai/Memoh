<template>
  <Item variant="outline">
    <ItemContent>
      <ItemTitle>{{ model.name }}</ItemTitle>
      <ItemDescription class="gap-2 flex flex-wrap items-center mt-3">
        <Badge variant="outline">
          {{ model.type }}
        </Badge>
        <Badge
          v-if="model.client_type"
          variant="outline"
        >
          {{ model.client_type }}
        </Badge>
      </ItemDescription>
    </ItemContent>
    <ItemActions>
      <Button
        type="button"
        variant="outline"
        class="cursor-pointer"
        :aria-label="$t('common.edit')"
        @click="$emit('edit', model)"
      >
        <FontAwesomeIcon :icon="['fas', 'gear']" />
      </Button>

      <ConfirmPopover
        :message="$t('models.deleteModelConfirm')"
        :loading="deleteLoading"
        @confirm="$emit('delete', model.name)"
      >
        <template #trigger>
          <Button
            type="button"
            variant="outline"
            :aria-label="$t('common.delete')"
          >
            <FontAwesomeIcon :icon="['far', 'trash-can']" />
          </Button>
        </template>
      </ConfirmPopover>
    </ItemActions>
  </Item>
</template>

<script setup lang="ts">
import {
  Item,
  ItemContent,
  ItemDescription,
  ItemActions,
  ItemTitle,
  Badge,
  Button,
} from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import type { ModelsGetResponse } from '@memoh/sdk'

defineProps<{
  model: ModelsGetResponse
  deleteLoading: boolean
}>()

defineEmits<{
  edit: [model: ModelsGetResponse]
  delete: [name: string]
}>()
</script>
