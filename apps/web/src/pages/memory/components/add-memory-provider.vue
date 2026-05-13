<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child>
      <Button
        variant="outline"
        class="w-full shadow-none! text-muted-foreground h-9 px-3 rounded-md border-border bg-background hover:bg-accent"
      >
        <Plus
          class="mr-1 size-4"
        />
        {{ $t('memory.add') }}
      </Button>
    </DialogTrigger>
    <DialogContent class="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{{ $t('memory.add') }}</DialogTitle>
      </DialogHeader>
      <div class="space-y-4 py-4">
        <div class="space-y-2">
          <Label>{{ $t('memory.name') }}</Label>
          <Input
            v-model="form.name"
            :placeholder="$t('memory.namePlaceholder')"
          />
        </div>
        <div class="space-y-2">
          <Label>{{ $t('memory.provider') }}</Label>
          <Select v-model:model-value="form.provider">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="builtin">
                  {{ $t('memory.providerNames.builtin') }}
                </SelectItem>
                <SelectItem value="mem0">
                  {{ $t('memory.providerNames.mem0') }}
                </SelectItem>
                <SelectItem value="openviking">
                  {{ $t('memory.providerNames.openviking') }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
      </div>
      <DialogFooter>
        <Button
          variant="outline"
          @click="open = false"
        >
          {{ $t('common.cancel') }}
        </Button>
        <Button
          :disabled="!form.name.trim() || !form.provider || loading"
          @click="handleCreate"
        >
          <Spinner
            v-if="loading"
            class="mr-1.5"
          />
          {{ $t('common.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { Plus } from 'lucide-vue-next'
import { reactive, ref } from 'vue'
import {
  Button,
  Input,
  Label,
  Spinner,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogTrigger,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
} from '@memohai/ui'
import { postMemoryProviders } from '@memohai/sdk'
import type { AdaptersProviderType } from '@memohai/sdk'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useQueryCache } from '@pinia/colada'

const open = defineModel<boolean>('open', { default: false })
const { t } = useI18n()
const queryCache = useQueryCache()
const loading = ref(false)

const form = reactive({
  name: '',
  provider: 'builtin',
})

async function handleCreate() {
  loading.value = true
  try {
    await postMemoryProviders({
      body: {
        name: form.name.trim(),
        provider: form.provider as AdaptersProviderType,
        config: {},
      },
      throwOnError: true,
    })
    toast.success(t('memory.saveSuccess'))
    queryCache.invalidateQueries({ key: ['memory-providers'] })
    open.value = false
    form.name = ''
  } catch (error) {
    console.error('Failed to create memory provider:', error)
    toast.error(t('common.saveFailed'))
  } finally {
    loading.value = false
  }
}
</script>
