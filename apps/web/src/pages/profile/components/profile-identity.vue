<template>
  <div class="group/row mx-4 flex items-center gap-4 border-b border-border py-4 last:border-b-0">
    <!-- Avatar (hover → dialog to set URL) -->
    <Dialog
      v-model:open="avatarOpen"
      @update:open="onAvatarOpenChange"
    >
      <DialogTrigger as-child>
        <button
          type="button"
          class="group/avatar relative shrink-0 cursor-pointer rounded-full outline-none"
          :aria-label="$t('settings.avatarUrl')"
        >
          <Avatar class="size-14">
            <AvatarImage
              v-if="avatarUrl"
              :src="avatarUrl"
              :alt="fallback"
            />
            <AvatarFallback>
              {{ fallback }}
            </AvatarFallback>
          </Avatar>
          <span class="absolute inset-0 flex items-center justify-center rounded-full bg-black/45 opacity-0 transition-opacity group-hover/avatar:opacity-100">
            <Pencil class="size-4 text-white" />
          </span>
        </button>
      </DialogTrigger>

      <DialogContent :show-close-button="false">
        <DialogHeader>
          <DialogTitle>{{ $t('settings.avatarUrl') }}</DialogTitle>
        </DialogHeader>
        <div class="flex items-center gap-4">
          <Avatar class="size-14 shrink-0">
            <AvatarImage
              v-if="avatarDraft"
              :src="avatarDraft"
              :alt="fallback"
            />
            <AvatarFallback>
              {{ fallback }}
            </AvatarFallback>
          </Avatar>
          <Input
            v-model="avatarDraft"
            type="url"
            class="flex-1"
            :aria-label="$t('settings.avatarUrl')"
          />
        </div>
        <DialogFooter>
          <DialogClose as-child>
            <Button variant="outline">
              {{ $t('common.cancel') }}
            </Button>
          </DialogClose>
          <Button @click="applyAvatar">
            {{ $t('common.save') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Name: read-only with a hover Edit pencil; click → inline editor (✓ / ✗).
         Both modes share the same h-8 line so the username row never shifts. -->
    <div class="min-w-0 flex-1">
      <div class="flex h-8 items-center gap-1.5">
        <template v-if="editing">
          <Input
            v-model="nameDraft"
            autofocus
            :aria-label="$t('settings.displayName')"
            class="h-8 max-w-[16rem]"
            @keydown.enter="confirmName"
            @keydown.esc="cancelName"
          />
          <Button
            variant="secondary"
            size="icon-sm"
            :aria-label="$t('common.save')"
            @click="confirmName"
          >
            <Check class="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            :aria-label="$t('common.cancel')"
            @click="cancelName"
          >
            <X class="size-4" />
          </Button>
        </template>
        <template v-else>
          <span class="truncate text-sm font-medium text-foreground">
            {{ displayName || username }}
          </span>
          <Button
            variant="ghost"
            size="icon-sm"
            class="shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover/row:opacity-100"
            :aria-label="$t('common.edit')"
            @click="startEdit"
          >
            <Pencil class="size-3.5" />
          </Button>
        </template>
      </div>

      <div class="mt-0.5 truncate text-xs text-muted-foreground">
        {{ username }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
  Button,
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  Input,
} from '@memohai/ui'
import { Check, Pencil, X } from 'lucide-vue-next'

const props = defineProps<{
  avatarUrl: string
  displayName: string
  username: string
  fallback: string
}>()

const emit = defineEmits<{
  'update:avatarUrl': [value: string]
  'update:displayName': [value: string]
  save: []
}>()

// ── Name inline editor ──
const editing = ref(false)
const nameDraft = ref('')

function startEdit() {
  nameDraft.value = props.displayName
  editing.value = true
}

function confirmName() {
  emit('update:displayName', nameDraft.value.trim())
  editing.value = false
  emit('save')
}

function cancelName() {
  editing.value = false
}

// ── Avatar editor (dialog) ──
const avatarOpen = ref(false)
const avatarDraft = ref('')

function onAvatarOpenChange(value: boolean) {
  if (value) avatarDraft.value = props.avatarUrl
}

function applyAvatar() {
  emit('update:avatarUrl', avatarDraft.value.trim())
  avatarOpen.value = false
  emit('save')
}
</script>
