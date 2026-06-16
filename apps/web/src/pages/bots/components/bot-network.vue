<template>
  <PageShell
    variant="tab"
    :title="$t('bots.settings.networkPageTitle')"
    :description="$t('bots.settings.networkPageSubtitle')"
  >
    <template #actions>
      <span
        v-if="hasChanges"
        class="text-xs text-muted-foreground"
      >
        {{ $t('common.unsaved') }}
      </span>
      <Button
        variant="outline"
        size="sm"
        :disabled="!props.botId || isNetworkStatusFetching"
        @click="handleRefreshNetworkStatus"
      >
        <Spinner
          v-if="isNetworkStatusFetching"
          class="size-3"
        />
        {{ $t('common.refresh') }}
      </Button>
    </template>

    <div class="space-y-8">
      <SettingsSection
        v-if="props.botId && workspaceStatusFields.length"
        :title="$t('bots.settings.botStatusTitle')"
      >
        <div
          v-for="item in workspaceStatusFields"
          :key="`ws-${item.label}`"
          class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3 last:border-b-0"
        >
          <div class="min-w-0 text-sm font-medium text-foreground">
            {{ item.label }}
          </div>
          <div class="max-w-md break-all text-right font-mono text-xs text-muted-foreground">
            {{ item.value }}
          </div>
        </div>
      </SettingsSection>

      <SettingsSection
        v-if="props.botId && (showOverlayStatusInNetworkCard || isNetworkStatusPendingSave || isNetworkStatusLoading)"
        :title="$t('bots.settings.networkSDWANSectionTitle')"
      >
        <div
          v-if="isNetworkStatusLoading && !networkStatusCard"
          class="mx-4 flex min-h-[3.75rem] items-center gap-2 py-3 text-sm text-muted-foreground"
        >
          <Spinner class="size-4" />
          <span>{{ $t('common.loading') }}</span>
        </div>

        <template v-else-if="networkStatusCard">
          <div
            v-if="isNetworkStatusPendingSave"
            class="mx-4 flex min-h-[3.75rem] items-center gap-3 border-b border-border py-3"
          >
            <span class="size-1.5 rounded-sm bg-warning" />
            <div class="min-w-0">
              <div class="text-sm font-medium text-foreground">
                {{ $t('bots.settings.networkStatusPendingSaveTitle') }}
              </div>
              <p class="mt-0.5 text-xs text-muted-foreground">
                {{ $t('bots.settings.networkStatusPendingSave') }}
              </p>
            </div>
          </div>

          <template v-if="showOverlayStatusInNetworkCard && overlayNetworkStatusFields.length">
            <div
              v-if="overlayState === 'needs_login'"
              class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3"
            >
              <div class="min-w-0">
                <div class="text-sm font-medium text-foreground">
                  {{ $t('common.actionRequired') }}
                </div>
                <p class="mt-0.5 text-xs text-muted-foreground">
                  {{ $t('bots.settings.networkNeedsLoginDescription') }}
                </p>
              </div>
              <Button
                v-if="overlayAuthURL"
                size="sm"
                variant="outline"
                class="shrink-0"
                @click="openAuthURL"
              >
                {{ $t('bots.settings.networkOpenLoginPage') }}
              </Button>
            </div>

            <div
              v-for="item in overlayNetworkStatusFields"
              :key="`ov-${item.label}`"
              class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3 last:border-b-0"
            >
              <div class="min-w-0 text-sm font-medium text-foreground">
                {{ item.label }}
              </div>
              <div class="max-w-md break-all text-right font-mono text-xs text-muted-foreground">
                {{ item.value }}
              </div>
            </div>

            <div
              v-if="showLogoutButton"
              class="mx-4 flex min-h-[3.75rem] items-center justify-end border-t border-border py-3"
            >
              <Button
                variant="destructive"
                size="sm"
                :disabled="isLoggingOut"
                @click="handleLogout"
              >
                <Spinner
                  v-if="isLoggingOut"
                  class="size-3"
                />
                {{ $t('bots.settings.networkLogout') }}
              </Button>
            </div>
          </template>
        </template>
        <div
          v-else
          class="mx-4 flex min-h-[3.75rem] items-center py-3 text-sm text-muted-foreground"
        >
          {{ $t('bots.settings.networkStatusEmpty') }}
        </div>
      </SettingsSection>

      <SettingsSection
        v-if="props.botId"
        :title="$t('bots.settings.networkSDWANSectionTitle')"
      >
        <SettingsRow
          :label="$t('common.enable')"
          :description="$t('bots.settings.networkSDWANSectionHint')"
        >
          <Switch
            :model-value="form.overlay_enabled"
            @update:model-value="(val) => form.overlay_enabled = val"
          />
        </SettingsRow>

        <template v-if="form.overlay_enabled">
          <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3">
            <div class="min-w-0">
              <div class="text-sm font-medium text-foreground">
                {{ $t('bots.settings.overlayProviderFieldLabel') }}
              </div>
            </div>
            <OverlayProviderSelect
              v-model="form.overlay_provider"
              :providers="overlayProviderMeta"
              :placeholder="$t('bots.settings.overlayProviderPlaceholder')"
              class="w-72 shrink-0"
            />
          </div>

          <div
            v-if="showOverlayConfig"
            class="border-b border-border"
          >
            <template v-if="primarySchema?.fields?.length">
              <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3">
                <div class="text-sm font-medium text-foreground">
                  {{ $t('bots.settings.overlayPrimaryConfigTitle') }}
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  @click="isEditorDialogOpen = true"
                >
                  <SquarePen class="size-4" />
                  {{ $t('common.editJson') }}
                </Button>
              </div>

              <template
                v-for="field in primarySchema.fields"
                :key="field.key"
              >
                <div
                  class="mx-4 border-b border-border py-3 last:border-b-0"
                  :class="isMultilineField(field) ? 'space-y-3' : 'flex min-h-[3.75rem] items-center justify-between gap-4'"
                >
                  <div class="min-w-0">
                    <Label
                      :for="`bot-network-config-primary-${field.key}`"
                      class="text-sm font-medium text-foreground"
                    >
                      {{ field.title || field.key }}
                    </Label>
                    <p
                      v-if="field.description"
                      class="mt-0.5 text-xs text-muted-foreground"
                    >
                      {{ field.description }}
                    </p>
                  </div>

                  <div :class="isMultilineField(field) ? 'w-full' : 'w-full shrink-0 sm:w-80'">
                    <Textarea
                      v-if="isMultilineField(field)"
                      :id="`bot-network-config-primary-${field.key}`"
                      :model-value="stringValue(field)"
                      :placeholder="placeholderOf(field)"
                      :readonly="field.readonly"
                      rows="4"
                      class="min-h-24 resize-none"
                      @update:model-value="(val: string) => updateValue(field.key, val)"
                    />

                    <Switch
                      v-else-if="field.type === 'bool'"
                      :model-value="!!getFieldValue(field)"
                      :disabled="field.readonly"
                      @update:model-value="(val: boolean) => updateValue(field.key, !!val)"
                    />

                    <Select
                      v-else-if="field.type === 'enum' && field.enum"
                      :model-value="stringValue(field)"
                      :disabled="field.readonly"
                      @update:model-value="(val: string) => updateValue(field.key, val)"
                    >
                      <SelectTrigger
                        size="sm"
                        class="w-full"
                      >
                        <SelectValue :placeholder="placeholderOf(field)" />
                      </SelectTrigger>
                      <SelectContent class="w-[--reka-select-trigger-width]">
                        <SelectItem
                          v-for="option in field.enum"
                          :key="option"
                          :value="option"
                        >
                          {{ option }}
                        </SelectItem>
                      </SelectContent>
                    </Select>

                    <InputGroup v-else-if="field.type === 'secret'">
                      <InputGroupInput
                        :id="`bot-network-config-primary-${field.key}`"
                        :model-value="stringValue(field)"
                        :type="visibleSecrets[field.key] ? 'text' : 'password'"
                        :placeholder="placeholderOf(field)"
                        :readonly="field.readonly"
                        @update:model-value="(val: string) => updateValue(field.key, val)"
                      />
                      <InputGroupAddon align="inline-end">
                        <InputGroupButton
                          size="icon-sm"
                          :aria-label="$t('common.preview')"
                          @click="visibleSecrets[field.key] = !visibleSecrets[field.key]"
                        >
                          <component :is="visibleSecrets[field.key] ? EyeOff : Eye" />
                        </InputGroupButton>
                      </InputGroupAddon>
                    </InputGroup>

                    <Input
                      v-else-if="field.type === 'number'"
                      :id="`bot-network-config-primary-${field.key}`"
                      :model-value="numberValue(field)"
                      type="number"
                      :placeholder="placeholderOf(field)"
                      :readonly="field.readonly"
                      :min="field.constraint?.min"
                      :max="field.constraint?.max"
                      :step="field.constraint?.step ?? 1"
                      class="h-8 w-full tabular-nums"
                      @update:model-value="(val: string) => updateNumber(field.key, val)"
                    />

                    <Input
                      v-else
                      :id="`bot-network-config-primary-${field.key}`"
                      :model-value="stringValue(field)"
                      type="text"
                      :placeholder="placeholderOf(field)"
                      :readonly="field.readonly"
                      class="h-8 w-full"
                      @update:model-value="(val: string) => updateValue(field.key, val)"
                    />
                  </div>
                </div>
              </template>
            </template>

            <div
              v-if="advancedSchema?.fields?.length"
              class="mx-4 border-b border-border py-3"
            >
              <Button
                variant="ghost"
                size="sm"
                class="w-full justify-between"
                @click="showAdvancedConfig = !showAdvancedConfig"
              >
                <span>{{ $t('common.advanced') }} ({{ $t('common.allOptional') }})</span>
                <component :is="showAdvancedConfig ? ChevronDown : ChevronRight" />
              </Button>
                  
              <div
                v-show="showAdvancedConfig"
                class="space-y-6 pt-4"
              >
                <div
                  v-for="group in advancedSchemaGroups"
                  :key="group.title"
                  class="space-y-2"
                >
                  <div class="text-xs font-medium text-muted-foreground">
                    {{ group.title }}
                  </div>

                  <template
                    v-for="field in group.fields"
                    :key="field.key"
                  >
                    <div
                      class="border-t border-border py-3"
                      :class="isMultilineField(field) ? 'space-y-3' : 'flex min-h-[3.75rem] items-center justify-between gap-4'"
                    >
                      <div class="min-w-0">
                        <Label
                          :for="`bot-network-config-advanced-${field.key}`"
                          class="text-sm font-medium text-foreground"
                        >
                          {{ field.title || field.key }}
                        </Label>
                        <p
                          v-if="field.description"
                          class="mt-0.5 text-xs text-muted-foreground"
                        >
                          {{ field.description }}
                        </p>
                      </div>

                      <div :class="isMultilineField(field) ? 'w-full' : 'w-full shrink-0 sm:w-80'">
                        <Textarea
                          v-if="isMultilineField(field)"
                          :id="`bot-network-config-advanced-${field.key}`"
                          :model-value="stringValue(field)"
                          :placeholder="placeholderOf(field)"
                          :readonly="field.readonly"
                          rows="4"
                          class="min-h-24 resize-none"
                          @update:model-value="(val: string) => updateValue(field.key, val)"
                        />

                        <Switch
                          v-else-if="field.type === 'bool'"
                          :model-value="!!getFieldValue(field)"
                          :disabled="field.readonly"
                          @update:model-value="(val: boolean) => updateValue(field.key, !!val)"
                        />

                        <Select
                          v-else-if="field.type === 'enum' && field.enum"
                          :model-value="stringValue(field)"
                          :disabled="field.readonly"
                          @update:model-value="(val: string) => updateValue(field.key, val)"
                        >
                          <SelectTrigger
                            size="sm"
                            class="w-full"
                          >
                            <SelectValue :placeholder="placeholderOf(field)" />
                          </SelectTrigger>
                          <SelectContent class="w-[--reka-select-trigger-width]">
                            <SelectItem
                              v-for="option in field.enum"
                              :key="option"
                              :value="option"
                            >
                              {{ option }}
                            </SelectItem>
                          </SelectContent>
                        </Select>

                        <InputGroup v-else-if="field.type === 'secret'">
                          <InputGroupInput
                            :id="`bot-network-config-advanced-${field.key}`"
                            :model-value="stringValue(field)"
                            :type="visibleSecrets[field.key] ? 'text' : 'password'"
                            :placeholder="placeholderOf(field)"
                            :readonly="field.readonly"
                            @update:model-value="(val: string) => updateValue(field.key, val)"
                          />
                          <InputGroupAddon align="inline-end">
                            <InputGroupButton
                              size="icon-sm"
                              :aria-label="$t('common.preview')"
                              @click="visibleSecrets[field.key] = !visibleSecrets[field.key]"
                            >
                              <component :is="visibleSecrets[field.key] ? EyeOff : Eye" />
                            </InputGroupButton>
                          </InputGroupAddon>
                        </InputGroup>

                        <Input
                          v-else-if="field.type === 'number'"
                          :id="`bot-network-config-advanced-${field.key}`"
                          :model-value="numberValue(field)"
                          type="number"
                          :placeholder="placeholderOf(field)"
                          :readonly="field.readonly"
                          :min="field.constraint?.min"
                          :max="field.constraint?.max"
                          :step="field.constraint?.step ?? 1"
                          class="h-8 w-full tabular-nums"
                          @update:model-value="(val: string) => updateNumber(field.key, val)"
                        />

                        <Input
                          v-else
                          :id="`bot-network-config-advanced-${field.key}`"
                          :model-value="stringValue(field)"
                          type="text"
                          :placeholder="placeholderOf(field)"
                          :readonly="field.readonly"
                          class="h-8 w-full"
                          @update:model-value="(val: string) => updateValue(field.key, val)"
                        />
                      </div>
                    </div>
                  </template>
                </div>
              </div>
            </div>
          </div>

          <div class="mx-4 flex min-h-[3.75rem] items-center justify-end gap-2 py-3">
            <Button
              variant="outline"
              size="sm"
              @click="handleCancel"
            >
              {{ $t('common.cancel') }}
            </Button>
            <Button
              variant="outline"
              size="sm"
              :disabled="!hasChanges || isSaving"
              @click="handleSaveWithEnable(false)"
            >
              <Spinner
                v-if="isSaving"
                class="size-3"
              />
              {{ $t('bots.settings.saveOnly') }}
            </Button>
            <Button
              size="sm"
              :disabled="!hasChanges || isSaving"
              @click="handleSaveWithEnable(true)"
            >
              <Spinner
                v-if="isSaving"
                class="size-3"
              />
              {{ $t('bots.settings.saveAndEnable') }}
            </Button>
          </div>
        </template>
      </SettingsSection>

      <SettingsSection
        v-if="showExitNodeSelector"
        :title="$t('bots.settings.networkExitNode')"
      >
        <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3">
          <div class="min-w-0">
            <div class="text-sm font-medium text-foreground">
              {{ $t('bots.settings.networkExitNode') }}
            </div>
            <p class="mt-0.5 text-xs text-muted-foreground">
              {{ nodeListHint }}
            </p>
          </div>
          <div class="flex w-80 shrink-0 items-center gap-2">
            <NetworkNodeSelect
              v-model="exitNodeValue"
              :nodes="exitNodeOptions"
              :placeholder="$t('bots.settings.networkExitNodePlaceholder')"
            />
            <Button
              variant="outline"
              size="icon-sm"
              :aria-label="$t('common.refresh')"
              :disabled="!shouldLoadNodeOptions || isNodeListLoading"
              @click="handleRefreshNodes"
            >
              <Spinner
                v-if="isNodeListLoading"
                class="size-3"
              />
              <RefreshCw
                v-else
                class="size-3.5"
              />
            </Button>
          </div>
        </div>

        <template v-if="selectedExitNodeMeta">
          <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3">
            <div class="text-sm font-medium text-foreground">
              {{ $t('bots.settings.networkExitNodeStatus') }}
            </div>
            <div class="font-mono text-xs text-muted-foreground">
              {{ selectedExitNodeMeta.online ? $t('bots.settings.networkExitNodeOnline') : $t('bots.settings.networkExitNodeOffline') }}
            </div>
          </div>
          <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 py-3">
            <div class="text-sm font-medium text-foreground">
              {{ $t('bots.settings.networkExitNodeAddresses') }}
            </div>
            <div class="max-w-md break-all text-right font-mono text-xs text-muted-foreground">
              {{ (selectedExitNodeMeta.addresses ?? []).join(', ') || '-' }}
            </div>
          </div>
        </template>
      </SettingsSection>
    </div>

    <Dialog v-model:open="isEditorDialogOpen">
      <DialogContent class="flex max-h-[calc(100vh-2rem)] flex-col overflow-hidden p-0 sm:h-[70vh] sm:max-w-3xl">
        <DialogHeader class="shrink-0 border-b border-border p-4">
          <DialogTitle class="text-sm font-semibold">
            {{ $t('mcp.editValue') }}
          </DialogTitle>
          <DialogDescription class="text-xs leading-snug">
            {{ $t('mcp.editLongTextHint') }}
          </DialogDescription>
        </DialogHeader>
        
        <div class="flex min-h-0 flex-1 p-4">
          <MonacoEditor
            v-model="editorDraftRaw"
            language="json"
            class="min-h-0 flex-1 rounded-md border border-border"
            :options="{
              automaticLayout: true,
              fixedOverflowWidgets: true,
              minimap: { enabled: false },
              scrollBeyondLastLine: false,
              formatOnPaste: true,
              formatOnType: true
            }"
          />
        </div>

        <DialogFooter class="flex shrink-0 items-center justify-between gap-2 border-t border-border p-4">
          <p
            v-if="editorError"
            class="text-xs text-warning"
          >
            {{ editorError }}
          </p>
          <div v-else />
          <div class="flex items-center gap-2">
            <DialogClose as-child>
              <Button
                variant="ghost"
                size="sm"
              >
                {{ $t('common.cancel') }}
              </Button>
            </DialogClose>
            <Button
              size="sm"
              class="min-w-24"
              :disabled="!!editorError"
              @click="handleEditorSave"
            >
              {{ $t('common.confirm') }}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </PageShell>
</template>

<script setup lang="ts">
import {
  Button,
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
  Label,
  Spinner,
  Switch,
  Input,
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
  Textarea,
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter, DialogClose,
} from '@memohai/ui'
import { SquarePen, ChevronDown, ChevronRight, Eye, EyeOff, RefreshCw } from 'lucide-vue-next'
import { reactive, computed, watch, nextTick, onBeforeUnmount, ref } from 'vue'
import { toast } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { getBotsByBotIdSettings, putBotsByBotIdSettings } from '@memohai/sdk'
import type { SettingsSettings } from '@memohai/sdk'
import { cloneConfig, getPathValue, setPathValue } from '@/components/config-schema-form/utils'
import type { ConfigSchema, ConfigSchemaField } from '@/components/config-schema-form/types'
import { resolveApiErrorMessage } from '@/utils/api-error'
import SettingsRow from '@/components/settings/row.vue'
import SettingsSection from '@/components/settings/section.vue'
import PageShell from '@/components/page-shell/index.vue'
import OverlayProviderSelect from './network-provider-select.vue'
import NetworkNodeSelect from './network-node-select.vue'
import MonacoEditor from '@/components/monaco-editor/index.vue'
import {
  getBotNetworkStatus,
  executeBotNetworkAction,
  listBotNetworkNodes,
  listOverlayProviderMeta,
  type NetworkBotStatus,
  type OverlayProviderMeta,
} from '@/pages/network/api'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const queryCache = useQueryCache()

const { data: settings } = useQuery({
  key: () => ['bot-settings', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdSettings({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
})

const { data: overlayProviderMetaData } = useQuery({
  key: ['network-providers-meta'],
  query: () => listOverlayProviderMeta(),
})

const overlayProviderMeta = computed(() => overlayProviderMetaData.value ?? [])

const form = reactive({
  overlay_enabled: false,
  overlay_provider: '',
  overlay_config: {} as Record<string, unknown>,
})

const selectedOverlayProviderMeta = computed(() =>
  overlayProviderMeta.value.find((meta: OverlayProviderMeta) => meta.kind === form.overlay_provider),
)
const selectedNetworkCapabilities = computed(() =>
  selectedOverlayProviderMeta.value?.capabilities ?? null,
)

// Primary and Advanced schemas computed locally to avoid modifying the generic ConfigSchemaForm
const selectedOverlayProviderSchema = computed<ConfigSchema | undefined>(() => {
  const schema = selectedOverlayProviderMeta.value?.config_schema as ConfigSchema | undefined
  if (!schema) return undefined
  return {
    ...schema,
    fields: (schema.fields ?? []).filter(field => field.key !== 'exit_node'),
  }
})

const primarySchema = computed<ConfigSchema | undefined>(() => {
  if (!selectedOverlayProviderSchema.value) return undefined
  return {
    ...selectedOverlayProviderSchema.value,
    fields: selectedOverlayProviderSchema.value.fields.filter(f => !f.collapsed)
  }
})

const advancedSchema = computed<ConfigSchema | undefined>(() => {
  if (!selectedOverlayProviderSchema.value) return undefined
  // Map collapsed to false so the ConfigSchemaForm renders them flat. We handle the wrapper.
  return {
    ...selectedOverlayProviderSchema.value,
    fields: selectedOverlayProviderSchema.value.fields
      .filter(f => f.collapsed)
      .map(f => ({ ...f, collapsed: false }))
  }
})

const advancedSchemaGroups = computed(() => {
  if (!advancedSchema.value) return []
  const fields = advancedSchema.value.fields

  const auth = fields.filter(f => (f.order ?? 0) >= 10 && (f.order ?? 0) < 12)
  const network = fields.filter(f => (f.order ?? 0) >= 12 && (f.order ?? 0) < 20)
  const environment = fields.filter(f => (f.order ?? 0) >= 20 && (f.order ?? 0) < 30)
  const others = fields.filter(f => (f.order ?? 0) >= 30)

  const groups = []
  if (auth.length) groups.push({ title: t('mcp.oauth.title'), fields: auth })
  if (network.length) groups.push({ title: t('sidebar.network'), fields: network })
  if (environment.length) groups.push({ title: t('mcp.env'), fields: environment })
  if (others.length) groups.push({ title: t('common.advanced'), fields: others })

  return groups.map(g => ({
    ...g,
    fields: g.fields
  }))
})

const showOverlayConfig = computed(() =>
  !!form.overlay_enabled
  && !!form.overlay_provider
  && !!selectedOverlayProviderSchema.value?.fields?.length,
)

const showAdvancedConfig = ref(false)

// ---------------------------------------------------------------------------
// Manual Field Rendering Helpers
// ---------------------------------------------------------------------------
const visibleSecrets = reactive<Record<string, boolean>>({})

function getFieldValue(field: ConfigSchemaField) {
  const current = getPathValue(form.overlay_config, field.key)
  if (current !== undefined) return current
  return field.default
}

function stringValue(field: ConfigSchemaField) {
  const value = getFieldValue(field)
  return typeof value === 'string' ? value : value == null ? '' : String(value)
}

function numberValue(field: ConfigSchemaField) {
  const value = getFieldValue(field)
  return typeof value === 'number' ? String(value) : value == null ? '' : String(value)
}

function placeholderOf(field: ConfigSchemaField) {
  let base = field.placeholder || (field.example != null ? String(field.example) : '')

  if (!base) {
    const key = field.key.toLowerCase()
    if (key.includes('key') || key.includes('token') || key.includes('secret')) {
      base = 'tskey-auth-kru6P22CNTRL-...'
    } else if (key.includes('url')) {
      base = 'https://example.com'
    } else if (key.includes('port')) {
      base = '1080'
    } else if (key.includes('host') || key.includes('addr')) {
      base = '192.168.1.1'
    } else if (key.includes('tag')) {
      base = 'tag:bot,tag:server'
    } else if (key.includes('arg') || key.includes('cmd')) {
      base = '--verbose'
    } else if (key.includes('user')) {
      base = 'admin'
    } else if (field.type === 'number') {
      base = '60'
    } else if (field.type === 'textarea') {
      base = t('common.enterContent')
    } else {
      base = '...'
    }
  }

  return t('common.placeholderPrefix', { example: base })
}

function updateValue(path: string, value: unknown) {
  const next = cloneConfig(form.overlay_config)
  setPathValue(next, path, value)
  form.overlay_config = next
}

function updateNumber(path: string, value: string) {
  const nextValue = value === '' ? undefined : Number(value)
  updateValue(path, nextValue)
}

function isMultilineField(field: ConfigSchemaField) {
  return field.type === 'textarea' || field.multiline
}

// Exit node selection only makes sense after the sidecar is authenticated and connected.
const showExitNodeSelector = computed(() =>
  !!form.overlay_enabled
  && !!form.overlay_provider
  && !!selectedNetworkCapabilities.value?.exit_node
  && isConnected.value,
)

const persistedOverlayProvider = computed(() => settings.value?.overlay_provider ?? '')
const persistedOverlayEnabled = computed(() => settings.value?.overlay_enabled ?? false)
const persistedOverlayConfig = computed(() =>
  JSON.stringify((settings.value?.overlay_config as Record<string, unknown> | undefined) ?? {}),
)
const isSelectedNetworkPersisted = computed(() =>
  form.overlay_enabled === persistedOverlayEnabled.value
  && form.overlay_provider === persistedOverlayProvider.value
  && JSON.stringify(form.overlay_config ?? {}) === persistedOverlayConfig.value,
)
const shouldLoadNetworkStatus = computed(() =>
  !!props.botId
  && persistedOverlayEnabled.value
  && !!persistedOverlayProvider.value,
)
const shouldLoadNodeOptions = computed(() =>
  !!props.botId
  && shouldLoadNetworkStatus.value
  && !!selectedNetworkCapabilities.value?.exit_node,
)

// Transient states that should trigger automatic polling until resolved.
const TRANSIENT_STATES = ['starting', 'needs_login', 'needslogin', 'stopped']

const isTransientState = computed(() =>
  TRANSIENT_STATES.includes(overlayState.value),
)

const {
  data: networkStatusData,
  refetch: refetchNetworkStatus,
  isFetching: isNetworkStatusFetching,
  isLoading: isNetworkStatusLoading,
} = useQuery({
  key: () => ['bot-network-status', props.botId],
  query: () => getBotNetworkStatus(props.botId),
  enabled: () => !!props.botId,
  refetchOnWindowFocus: true,
})

const {
  data: nodeListData,
  isLoading: isNodeListLoading,
  refetch: refetchNodeList,
} = useQuery({
  key: () => ['bot-network-nodes', props.botId, persistedOverlayProvider.value],
  query: () => listBotNetworkNodes(props.botId),
  enabled: () => shouldLoadNodeOptions.value,
})

const { mutateAsync: updateSettings, isLoading: isSaving } = useMutation({
  mutation: async (body: Partial<SettingsSettings>) => {
    const { data } = await putBotsByBotIdSettings({
      path: { bot_id: props.botId },
      body,
      throwOnError: true,
    })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bot-settings', props.botId] })
    queryCache.invalidateQueries({ key: ['bot-network-status', props.botId] })
    queryCache.invalidateQueries({ key: ['bot-network-nodes', props.botId] })
  },
})

const { mutateAsync: runNetworkAction, isLoading: isLoggingOut } = useMutation({
  mutation: (actionID: string) =>
    executeBotNetworkAction(props.botId, actionID, {}),
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bot-network-status', props.botId] })
  },
})

// ---------------------------------------------------------------------------
// Editor state
// ---------------------------------------------------------------------------
const isEditorDialogOpen = ref(false)
const editorDraftRaw = ref('')
const editorError = ref('')

watch(isEditorDialogOpen, (open) => {
  if (open) {
    editorDraftRaw.value = JSON.stringify(form.overlay_config, null, 2)
    editorError.value = ''
  }
})

watch(editorDraftRaw, (val) => {
  try {
    JSON.parse(val)
    editorError.value = ''
  } catch {
    editorError.value = t('mcp.importErrorJson')
  }
})

function handleEditorSave() {
  try {
    const parsed = JSON.parse(editorDraftRaw.value)
    form.overlay_config = cloneConfig(parsed)
    isEditorDialogOpen.value = false
  } catch {
    // Should be caught by watcher, but just in case
    editorError.value = t('mcp.importErrorJson')
  }
}

// ---------------------------------------------------------------------------
// Overlay state helpers
// ---------------------------------------------------------------------------

const overlayState = computed(() => {
  const status = networkStatusData.value as NetworkBotStatus | null
  return status?.state ?? ''
})

const overlayAuthURL = computed(() => {
  const status = networkStatusData.value as NetworkBotStatus | null
  return (status?.details?.auth_url as string | undefined) ?? ''
})

// "Connected" means sidecar is fully running and authenticated.
const isConnected = computed(() =>
  ['ready', 'running', 'degraded'].includes(overlayState.value),
)

// Show logout when the sidecar is alive (connected or waiting for login).
const showLogoutButton = computed(() =>
  shouldLoadNetworkStatus.value
  && !isNetworkStatusPendingSave.value
  && ['ready', 'running', 'degraded', 'needs_login', 'starting', 'stopped'].includes(overlayState.value),
)

// ---------------------------------------------------------------------------

const exitNodeOptions = computed(() =>
  (nodeListData.value?.items ?? []).filter(node => node.can_exit_node !== false),
)
const nodeListHint = computed(() => {
  if (!isSelectedNetworkPersisted.value) return t('bots.settings.networkNodesPendingSave')
  if (nodeListData.value?.message) return nodeListData.value.message
  if (!exitNodeOptions.value.length) return t('bots.settings.networkNodesEmpty')
  return t('bots.settings.networkExitNodeDescription')
})
const exitNodeValue = computed({
  get: () => String(form.overlay_config.exit_node ?? ''),
  set: (value: string) => {
    form.overlay_config = {
      ...form.overlay_config,
      exit_node: value || undefined,
    }
  },
})
const selectedExitNodeMeta = computed(() =>
  exitNodeOptions.value.find(node => node.value === exitNodeValue.value),
)

const networkStatusCard = computed(() => {
  if (form.overlay_enabled && form.overlay_provider && !isSelectedNetworkPersisted.value) {
    return {
      state: 'pending_save',
      title: t('bots.settings.networkStatusPendingSaveTitle'),
      description: t('bots.settings.networkStatusPendingSave'),
    }
  }
  if (networkStatusData.value) {
    return networkStatusData.value
  }
  return null
})
const isNetworkStatusPendingSave = computed(() =>
  networkStatusCard.value?.state === 'pending_save',
)

const showOverlayStatusInNetworkCard = computed(() =>
  shouldLoadNetworkStatus.value
  && !!networkStatusData.value,
)

async function handleRefreshNetworkStatus() {
  await refetchNetworkStatus()
}

function workspaceStateDisplay(state: string) {
  const key = `bots.settings.networkWorkspaceState.${state}`
  const translated = t(key)
  return translated === key ? t('bots.settings.networkWorkspaceState.unknown') : translated
}

const workspaceStatusFields = computed(() => {
  const status = networkStatusData.value
  if (!status || !status.workspace) return []
  const ws = status.workspace
  const items: { label: string; value: string }[] = [
    { label: t('bots.settings.networkWorkspaceStateLabel'), value: workspaceStateDisplay(ws.state) },
  ]
  if (ws.container_id) items.push({ label: t('bots.settings.networkWorkspaceContainerID'), value: ws.container_id })
  if (ws.task_status) items.push({ label: t('bots.settings.networkWorkspaceTaskStatus'), value: ws.task_status })
  if (ws.pid != null && ws.pid > 0) {
    items.push({ label: t('bots.settings.networkWorkspaceTaskPID'), value: String(ws.pid) })
  }
  if (ws.network_target) items.push({ label: t('bots.settings.networkWorkspaceTarget'), value: ws.network_target })
  if (ws.message) items.push({ label: t('bots.settings.networkWorkspaceMessage'), value: ws.message })
  return items.filter(item => item.value)
})

const overlayNetworkStatusFields = computed(() => {
  const status = networkStatusData.value
  if (!status) return []
  const details = status.details ?? {}
  const items = [
    { label: t('bots.settings.networkStatusState'), value: status.state },
    { label: t('bots.settings.networkStatusIP'), value: status.network_ip },
    { label: t('bots.settings.networkStatusProxy'), value: status.proxy_address },
    { label: t('bots.settings.networkStatusPID'), value: details.pid == null ? undefined : String(details.pid) },
    { label: t('bots.settings.networkStatusDNSName'), value: details.dns_name as string | undefined },
    { label: t('bots.settings.networkStatusBackendState'), value: details.backend_state as string | undefined },
    { label: t('bots.settings.networkStatusHealth'), value: Array.isArray(details.health) ? details.health.join('; ') : undefined },
    { label: t('bots.settings.networkStatusSocket'), value: details.localapi_socket_host_path as string | undefined },
    { label: t('bots.settings.networkStatusExitNode'), value: details.configured_exit_node as string | undefined },
  ]
  return items.filter(item => item.value)
})

const hasChanges = computed(() => {
  if (!settings.value) return true
  const s = settings.value
  return form.overlay_enabled !== (s.overlay_enabled ?? false)
    || form.overlay_provider !== (s.overlay_provider ?? '')
    || JSON.stringify(form.overlay_config ?? {}) !== JSON.stringify((s.overlay_config as Record<string, unknown> | undefined) ?? {})
})

// When settings load from API, overlay_provider goes from '' to the saved value in the
// same flush as configs are written. A separate watcher on overlay_provider must not
// wipe those configs (it would leave the UI empty after refresh).
let skipProviderChangeReset = false

watch(() => form.overlay_provider, (next, prev) => {
  if (next === prev || skipProviderChangeReset) return
  form.overlay_config = {}
})

watch(settings, (val) => {
  if (!val) return
  skipProviderChangeReset = true
  form.overlay_enabled = val.overlay_enabled ?? false
  form.overlay_provider = val.overlay_provider ?? ''
  form.overlay_config = cloneConfig((val.overlay_config as Record<string, unknown> | undefined) ?? {})
  void nextTick(() => {
    skipProviderChangeReset = false
  })
}, { immediate: true })

// Poll network status every 5s while in a transient state (starting, needs_login, etc.)
let pollTimer: ReturnType<typeof setInterval> | null = null

watch(isTransientState, (shouldPoll) => {
  if (shouldPoll && !pollTimer) {
    pollTimer = setInterval(() => {
      if (isTransientState.value && !isNetworkStatusFetching.value) {
        refetchNetworkStatus()
      }
    }, 5000)
  } else if (!shouldPoll && pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}, { immediate: true })

onBeforeUnmount(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})

async function handleSave() {
  if (form.overlay_enabled && !form.overlay_provider) {
    toast.error(t('bots.settings.overlayProviderRequired'))
    return
  }
  try {
    await updateSettings({
      overlay_enabled: form.overlay_enabled,
      overlay_provider: form.overlay_provider,
      overlay_config: form.overlay_config,
    })
    toast.success(t('bots.settings.saveSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.settings.networkActionFailed')))
  }
}

function handleCancel() {
  if (settings.value) {
    skipProviderChangeReset = true
    form.overlay_enabled = settings.value.overlay_enabled ?? false
    form.overlay_provider = settings.value.overlay_provider ?? ''
    form.overlay_config = cloneConfig((settings.value.overlay_config as Record<string, unknown> | undefined) ?? {})
    void nextTick(() => {
      skipProviderChangeReset = false
    })
  }
}

async function handleSaveWithEnable(enable: boolean) {
  form.overlay_enabled = enable
  await handleSave()
}

async function handleRefreshNodes() {
  try {
    await refetchNodeList()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.settings.networkNodesRefreshFailed')))
  }
}

function openAuthURL() {
  if (overlayAuthURL.value) {
    window.open(overlayAuthURL.value, '_blank', 'noopener,noreferrer')
  }
}

async function handleLogout() {
  try {
    await runNetworkAction('logout')
    toast.success(t('bots.settings.networkLogoutSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.settings.networkLogoutFailed')))
  }
}
</script>

<style scoped>
</style>
