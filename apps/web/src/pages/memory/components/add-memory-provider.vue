<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child>
      <span
        v-if="hideTrigger"
        class="hidden"
      />
      <Button
        v-else
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
      <div class="py-4">
        <FormStack>
          <FieldStack :label="$t('memory.name')">
            <Input
              v-model="form.name"
              :placeholder="$t('memory.namePlaceholder')"
            />
          </FieldStack>
          <FieldStack :label="$t('memory.provider')">
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
          </FieldStack>
        </FormStack>
      </div>
      <DialogFooter>
        <Button
          variant="outline"
          @click="open = false"
        >
          {{ $t('common.cancel') }}
        </Button>
        <Button
          :disabled="!form.name.trim() || !form.provider"
          :loading="loading"
          @click="handleCreate"
        >
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
import FieldStack from '@/components/settings/field-stack.vue'
import FormStack from '@/components/settings/form-stack.vue'
import { postMemoryProviders } from '@memohai/sdk'
import type { AdaptersProviderType } from '@memohai/sdk'
import { toast } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import { useQueryCache } from '@pinia/colada'

const open = defineModel<boolean>('open', { default: false })
withDefaults(defineProps<{
  hideTrigger?: boolean
}>(), {
  hideTrigger: false,
})
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
