<template>
  <div class="max-w-2xl mx-auto pb-6 space-y-4">
    <!-- Sovereign Header -->
    <div class="flex items-center justify-between pb-4 border-b border-border/50">
      <div class="space-y-1">
        <h3 class="text-sm font-semibold text-foreground">
          {{ $t('bots.settings.networkPageTitle') }}
        </h3>
        <p class="text-[11px] text-muted-foreground">
          {{ $t('bots.settings.networkPageSubtitle') }}
        </p>
      </div>

      <div class="flex items-center gap-3 shrink-0">
        <Transition name="fade">
          <div
            v-if="hasChanges"
            class="flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-muted/40 border border-border/50"
          >
            <div class="size-1 rounded-full bg-muted-foreground/40" />
            <span class="text-[10px] text-muted-foreground font-medium whitespace-nowrap">Unsaved</span>
          </div>
        </Transition>

        <Button
          variant="outline"
          size="sm"
          class="h-8 text-xs font-medium min-w-24 shadow-none"
          :disabled="!props.botId || isNetworkStatusFetching"
          @click="handleRefreshNetworkStatus"
        >
          <Spinner
            v-if="isNetworkStatusFetching"
            class="mr-1.5 size-3"
          />
          {{ $t('common.refresh') }}
        </Button>

        <Button
          size="sm"
          :disabled="!hasChanges || isSaving"
          class="h-8 text-xs font-medium min-w-24 shadow-none"
          @click="handleSave"
        >
          <Spinner
            v-if="isSaving"
            class="mr-1.5 size-3"
          />
          {{ $t('bots.settings.save') }}
        </Button>
      </div>
    </div>

    <!-- Workspace Status Card -->
    <div
      v-if="props.botId && workspaceStatusFields.length"
      class="space-y-6 rounded-md border border-border bg-background p-5 shadow-none"
    >
      <div class="space-y-1">
        <h4 class="text-xs font-semibold text-foreground">
          {{ $t('bots.settings.botStatusTitle') }}
        </h4>
        <p class="text-[10px] text-muted-foreground">
          {{ $t('bots.settings.botStatusSubtitle') }}
        </p>
      </div>

      <div class="space-y-6">
        <div
          v-for="(item, idx) in workspaceStatusFields"
          :key="`ws-${idx}-${item.label}`"
          class="space-y-1.5"
        >
          <div class="text-[11px] font-medium text-muted-foreground/90">
            {{ item.label }}
          </div>
          <div class="font-mono text-[11px] text-foreground/90 break-all leading-relaxed">
            {{ item.value }}
          </div>
        </div>
      </div>
    </div>

    <!-- SD-WAN Status Card -->
    <div
      v-if="props.botId && (showOverlayStatusInNetworkCard || isNetworkStatusPendingSave || isNetworkStatusLoading)"
      class="space-y-6 rounded-md border border-border bg-background p-5 shadow-none"
    >
      <div class="space-y-1">
        <h4 class="text-xs font-semibold text-foreground">
          {{ $t('bots.settings.networkSDWANSectionTitle') }}
        </h4>
        <p class="text-[10px] text-muted-foreground">
          {{ $t('bots.settings.networkSDWANSectionHint') }}
        </p>
      </div>

      <div class="space-y-6">
        <!-- Loading State -->
        <div
          v-if="isNetworkStatusLoading && !networkStatusCard"
          class="flex items-center gap-2 text-[11px] text-muted-foreground py-2"
        >
          <Spinner class="size-3" />
          <span>{{ $t('common.loading') }}</span>
        </div>

        <!-- Status Fields -->
        <template v-else-if="networkStatusCard">
          <!-- Pending Save Banner -->
          <div
            v-if="isNetworkStatusPendingSave"
            class="border-l-2 border-warning/50 bg-warning/5 px-3 py-2 rounded-r-md flex items-center gap-2"
          >
            <div class="size-1 rounded-full bg-warning animate-pulse" />
            <p class="text-[10px] text-warning font-medium">
              {{ $t('bots.settings.networkStatusPendingSave') }}
            </p>
          </div>

          <!-- Overlay Fields -->
          <div
            v-if="showOverlayStatusInNetworkCard && overlayNetworkStatusFields.length"
            class="space-y-6"
          >
            <!-- Auth Required -->
            <div
              v-if="overlayState === 'needs_login'"
              class="border-l-2 border-primary/50 bg-muted/20 px-3 py-2.5 rounded-r-md flex flex-col items-start gap-2"
            >
              <div class="space-y-0.5">
                <p class="text-[11px] font-medium text-foreground">
                  Action Required
                </p>
                <p class="text-[10px] text-muted-foreground">
                  {{ $t('bots.settings.networkNeedsLoginDescription') }}
                </p>
              </div>
              <Button
                v-if="overlayAuthURL"
                size="sm"
                variant="outline"
                class="h-7 text-[10px] shadow-none bg-background"
                @click="openAuthURL"
              >
                {{ $t('bots.settings.networkOpenLoginPage') }}
              </Button>
            </div>

            <div
              v-for="(item, idx) in overlayNetworkStatusFields"
              :key="`ov-${idx}-${item.label}`"
              class="space-y-1.5"
            >
              <div class="text-[11px] font-medium text-muted-foreground/90">
                {{ item.label }}
              </div>
              <div class="font-mono text-[11px] text-foreground/90 break-all leading-relaxed">
                {{ item.value }}
              </div>
            </div>

            <!-- Logout -->
            <div
              v-if="showLogoutButton"
              class="pt-2"
            >
              <Button
                variant="ghost"
                size="sm"
                class="h-7 text-[10px] text-muted-foreground hover:text-destructive px-2 -ml-2"
                :disabled="isLoggingOut"
                @click="handleLogout"
              >
                <Spinner
                  v-if="isLoggingOut"
                  class="mr-1.5 size-3"
                />
                {{ $t('bots.settings.networkLogout') }}
              </Button>
            </div>
          </div>
        </template>
        <div
          v-else
          class="text-[11px] text-muted-foreground py-2 italic"
        >
          {{ $t('bots.settings.networkStatusEmpty') }}
        </div>
      </div>
    </div>

    <!-- SD-WAN Configuration Card -->
    <div
      v-if="props.botId"
      class="space-y-6 rounded-md border border-border bg-background p-5 shadow-none"
    >
      <div class="space-y-1">
        <h4 class="text-xs font-semibold text-foreground">
          {{ $t('bots.settings.networkSDWANSectionTitle') }}
        </h4>
        <p class="text-[10px] text-muted-foreground">
          {{ $t('bots.settings.networkSDWANSectionHint') }}
        </p>
      </div>

      <div class="space-y-6">
        <!-- Enable SD-WAN Toggle -->
        <div class="flex items-center justify-between gap-6">
          <div class="space-y-0.5 flex-1 pr-4">
            <Label class="text-[11px] font-medium text-foreground/70">{{ $t('common.enable') }}</Label>
          </div>
          <div class="shrink-0 flex justify-end">
            <Switch
              :model-value="form.overlay_enabled"
              @update:model-value="(val) => form.overlay_enabled = val"
            />
          </div>
        </div>

        <template v-if="form.overlay_enabled">
          <!-- Connection Type (Provider) -->
          <div class="flex items-center justify-between gap-6 pt-4 border-t border-border/40">
            <div class="space-y-0.5 flex-1 pr-4">
              <Label class="text-[11px] font-medium text-foreground/70">{{ $t('bots.settings.overlayProviderFieldLabel') }}</Label>
            </div>
            <div class="shrink-0 flex justify-end min-w-[160px] max-w-[320px] w-full">
              <OverlayProviderSelect
                v-model="form.overlay_provider"
                :providers="overlayProviderMeta"
                :placeholder="$t('bots.settings.overlayProviderPlaceholder')"
              />
            </div>
          </div>

          <!-- Configuration Sub-Boxes -->
          <div
            v-if="showOverlayConfig"
            class="pt-4 border-t border-border/50 space-y-4"
          >
            <!-- Primary Configuration -->
            <div
              v-if="primarySchema?.fields?.length"
              class="space-y-4 rounded-md border border-border/50 bg-background/50 p-4 shadow-none"
            >
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-1.5 text-xs font-medium text-foreground">
                  {{ $t('bots.settings.overlayPrimaryConfigTitle') }}
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-6 px-2 text-[10px] text-muted-foreground"
                  @click="isEditorDialogOpen = true"
                >
                  <SquarePen class="mr-1.5 size-3" />
                  Edit JSON
                </Button>
              </div>

              <div class="pt-4 border-t border-border/50 space-y-1">
                <template
                  v-for="field in primarySchema.fields"
                  :key="field.key"
                >
                  <!-- Multiline Fields (Boxed) -->
                  <div 
                    v-if="isMultilineField(field)"
                    class="space-y-2.5 p-4 bg-muted/20 rounded-md border border-border/40 my-3"
                  >
                    <div class="space-y-1">
                      <Label
                        :for="`bot-network-config-primary-${field.key}`"
                        class="text-[11px] font-semibold text-foreground/70"
                      >
                        {{ field.title || field.key }}
                      </Label>
                      <p
                        v-if="field.description"
                        class="text-[10px] text-muted-foreground/80 leading-relaxed"
                      >
                        {{ field.description }}
                      </p>
                    </div>
                    <Textarea
                      :id="`bot-network-config-primary-${field.key}`"
                      :model-value="stringValue(field)"
                      :placeholder="placeholderOf(field)"
                      :readonly="field.readonly"
                      rows="4"
                      class="text-xs shadow-none resize-none bg-background/50 focus:bg-background transition-colors border-border/60"
                      @update:model-value="(val: string) => updateValue(field.key, val)"
                    />
                  </div>

                  <!-- Standard Fields (Horizontal) -->
                  <div 
                    v-else
                    class="flex items-center justify-between gap-6 py-4 border-b border-border/40 last:border-0 min-h-[64px]"
                  >
                    <div class="space-y-0.5 flex-1 pr-4">
                      <Label
                        :for="`bot-network-config-primary-${field.key}`"
                        class="text-[11px] font-semibold text-foreground/70"
                      >
                        {{ field.title || field.key }}
                      </Label>
                      <p
                        v-if="field.description"
                        class="text-[10px] text-muted-foreground/80 leading-relaxed"
                      >
                        {{ field.description }}
                      </p>
                    </div>

                    <div class="shrink-0 flex justify-end min-w-[160px] max-w-[320px] w-full">
                      <!-- Switch for Bool -->
                      <Switch
                        v-if="field.type === 'bool'"
                        :model-value="!!getFieldValue(field)"
                        :disabled="field.readonly"
                        @update:model-value="(val: boolean) => updateValue(field.key, !!val)"
                      />

                      <!-- Select for Enum -->
                      <Select
                        v-else-if="field.type === 'enum' && field.enum"
                        :model-value="stringValue(field)"
                        :disabled="field.readonly"
                        @update:model-value="(val: string) => updateValue(field.key, val)"
                      >
                        <SelectTrigger class="h-8 text-xs w-full shadow-none bg-background/50 border-border/60">
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

                      <!-- Secret Input -->
                      <div
                        v-else-if="field.type === 'secret'"
                        class="relative w-full"
                      >
                        <Input
                          :id="`bot-network-config-primary-${field.key}`"
                          :model-value="stringValue(field)"
                          :type="visibleSecrets[field.key] ? 'text' : 'password'"
                          :placeholder="placeholderOf(field)"
                          :readonly="field.readonly"
                          class="h-8 text-xs shadow-none pr-8 bg-background/50 border-border/60 focus:bg-background transition-colors w-full"
                          @update:model-value="(val: string) => updateValue(field.key, val)"
                        />
                        <button
                          type="button"
                          class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                          @click="visibleSecrets[field.key] = !visibleSecrets[field.key]"
                        >
                          <component
                            :is="visibleSecrets[field.key] ? EyeOff : Eye"
                            class="size-3.5"
                          />
                        </button>
                      </div>

                      <!-- Number Input -->
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
                        :class="[
                          'h-8 text-xs shadow-none w-full bg-background/50 border-border/60 focus:bg-background transition-colors',
                          field.key.toLowerCase().includes('port') ? '[appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none' : ''
                        ]"
                        @update:model-value="(val: string) => updateNumber(field.key, val)"
                      />

                      <!-- Default Text Input -->
                      <Input
                        v-else
                        :id="`bot-network-config-primary-${field.key}`"
                        :model-value="stringValue(field)"
                        type="text"
                        :placeholder="placeholderOf(field)"
                        :readonly="field.readonly"
                        class="h-8 text-xs shadow-none w-full bg-background/50 border-border/60 focus:bg-background transition-colors"
                        @update:model-value="(val: string) => updateValue(field.key, val)"
                      />
                    </div>
                  </div>
                </template>
              </div>
            </div>

            <!-- Advanced Configuration -->
            <div
              v-if="advancedSchema?.fields?.length"
              class="rounded-md border border-border/50 bg-background/50 p-4 shadow-none transition-all duration-200"
              :class="showAdvancedConfig ? 'space-y-4' : 'space-y-0'"
            >
              <button
                type="button"
                class="flex items-center gap-1.5 text-xs font-medium text-foreground hover:text-foreground transition-colors w-full outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-sm"
                @click="showAdvancedConfig = !showAdvancedConfig"
              >
                {{ $t('common.advanced') }}
                <span class="text-[10px] text-muted-foreground font-normal ml-1">({{ $t('common.allOptional') }})</span>
                <component
                  :is="showAdvancedConfig ? ChevronDown : ChevronRight"
                  class="size-3.5 shrink-0 ml-auto"
                />
              </button>
                  
              <div
                v-show="showAdvancedConfig"
                class="pt-4 border-t border-border/50 space-y-8"
              >
                <div
                  v-for="group in advancedSchemaGroups"
                  :key="group.title"
                  class="space-y-2"
                >
                  <div class="flex items-center pt-2 pb-1 border-b border-border/20 mb-1">
                    <span class="text-[10px] font-bold text-foreground/50 shrink-0">
                      {{ group.title }}
                    </span>
                  </div>

                  <div class="space-y-1">
                    <template
                      v-for="field in group.fields"
                      :key="field.key"
                    >
                      <!-- Multiline Fields (Boxed) -->
                      <div 
                        v-if="isMultilineField(field)"
                        class="space-y-2.5 p-4 bg-muted/20 rounded-md border border-border/40 my-3"
                      >
                        <div class="space-y-1">
                          <Label
                            :for="`bot-network-config-advanced-${field.key}`"
                            class="text-[11px] font-semibold text-foreground/70"
                          >
                            {{ field.title || field.key }}
                          </Label>
                          <p
                            v-if="field.description"
                            class="text-[10px] text-muted-foreground/80 leading-relaxed"
                          >
                            {{ field.description }}
                          </p>
                        </div>
                        <Textarea
                          :id="`bot-network-config-advanced-${field.key}`"
                          :model-value="stringValue(field)"
                          :placeholder="placeholderOf(field)"
                          :readonly="field.readonly"
                          rows="4"
                          class="text-xs shadow-none resize-none bg-background/50 focus:bg-background transition-colors border-border/60"
                          @update:model-value="(val: string) => updateValue(field.key, val)"
                        />
                      </div>

                      <!-- Standard Fields (Horizontal) -->
                      <div 
                        v-else
                        class="flex items-center justify-between gap-6 py-4 border-b border-border/40 last:border-0 min-h-[64px]"
                      >
                        <div class="space-y-0.5 flex-1 pr-4">
                          <Label
                            :for="`bot-network-config-advanced-${field.key}`"
                            class="text-[11px] font-semibold text-foreground/70"
                          >
                            {{ field.title || field.key }}
                          </Label>
                          <p
                            v-if="field.description"
                            class="text-[10px] text-muted-foreground/80 leading-relaxed"
                          >
                            {{ field.description }}
                          </p>
                        </div>

                        <div class="shrink-0 flex justify-end min-w-[160px] max-w-[320px] w-full">
                          <!-- Switch for Bool -->
                          <Switch
                            v-if="field.type === 'bool'"
                            :model-value="!!getFieldValue(field)"
                            :disabled="field.readonly"
                            @update:model-value="(val: boolean) => updateValue(field.key, !!val)"
                          />

                          <!-- Select for Enum -->
                          <Select
                            v-else-if="field.type === 'enum' && field.enum"
                            :model-value="stringValue(field)"
                            :disabled="field.readonly"
                            @update:model-value="(val: string) => updateValue(field.key, val)"
                          >
                            <SelectTrigger class="h-8 text-xs w-full shadow-none bg-background/50 border-border/60">
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

                          <!-- Secret Input -->
                          <div
                            v-else-if="field.type === 'secret'"
                            class="relative w-full"
                          >
                            <Input
                              :id="`bot-network-config-advanced-${field.key}`"
                              :model-value="stringValue(field)"
                              :type="visibleSecrets[field.key] ? 'text' : 'password'"
                              :placeholder="placeholderOf(field)"
                              :readonly="field.readonly"
                              class="h-8 text-xs shadow-none pr-8 bg-background/50 border-border/60 focus:bg-background transition-colors w-full"
                              @update:model-value="(val: string) => updateValue(field.key, val)"
                            />
                            <button
                              type="button"
                              class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                              @click="visibleSecrets[field.key] = !visibleSecrets[field.key]"
                            >
                              <component
                                :is="visibleSecrets[field.key] ? EyeOff : Eye"
                                class="size-3.5"
                              />
                            </button>
                          </div>

                          <!-- Number Input -->
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
                            :class="[
                              'h-8 text-xs shadow-none w-full bg-background/50 border-border/60 focus:bg-background transition-colors',
                              field.key.toLowerCase().includes('port') ? '[appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none' : ''
                            ]"
                            @update:model-value="(val: string) => updateNumber(field.key, val)"
                          />

                          <!-- Default Text Input -->
                          <Input
                            v-else
                            :id="`bot-network-config-advanced-${field.key}`"
                            :model-value="stringValue(field)"
                            type="text"
                            :placeholder="placeholderOf(field)"
                            :readonly="field.readonly"
                            class="h-8 text-xs shadow-none w-full bg-background/50 border-border/60 focus:bg-background transition-colors"
                            @update:model-value="(val: string) => updateValue(field.key, val)"
                          />
                        </div>
                      </div>
                    </template>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- SD-WAN Actions -->
          <div class="flex justify-end gap-2 pt-2 border-t border-border/30">
            <Button
              variant="ghost"
              size="sm"
              class="h-8 text-[11px] font-medium px-4 shadow-none"
              @click="handleCancel"
            >
              {{ $t('common.cancel') }}
            </Button>
            <Button
              variant="outline"
              size="sm"
              class="h-8 text-[11px] font-medium px-4 shadow-none"
              :disabled="!hasChanges || isSaving"
              @click="handleSaveWithEnable(false)"
            >
              <Spinner
                v-if="isSaving && !form.overlay_enabled"
                class="mr-1.5 size-3"
              />
              {{ $t('bots.settings.saveOnly') }}
            </Button>
            <Button
              variant="secondary"
              size="sm"
              class="h-8 text-[11px] font-medium px-4 shadow-none"
              :disabled="!hasChanges || isSaving"
              @click="handleSaveWithEnable(true)"
            >
              <Spinner
                v-if="isSaving && form.overlay_enabled"
                class="mr-1.5 size-3"
              />
              {{ $t('bots.settings.saveAndEnable') }}
            </Button>
          </div>
        </template>
      </div>
    </div>

    <!-- Exit Node independent block -->
    <div
      v-if="showExitNodeSelector"
      class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none"
    >
      <div class="flex items-start justify-between gap-3">
        <div class="space-y-1">
          <h4 class="text-xs font-medium text-foreground">
            {{ $t('bots.settings.networkExitNode') }}
          </h4>
          <p class="text-[11px] text-muted-foreground">
            {{ $t('bots.settings.networkExitNodeSectionHint') }}
          </p>
        </div>
        <Button
          variant="ghost"
          size="sm"
          class="shrink-0 h-7 text-[10px] text-muted-foreground px-2"
          :disabled="!shouldLoadNodeOptions || isNodeListLoading"
          @click="handleRefreshNodes"
        >
          <Spinner
            v-if="isNodeListLoading"
            class="mr-1.5 size-3"
          />
          {{ $t('common.refresh') }}
        </Button>
      </div>

      <div class="space-y-1.5">
        <NetworkNodeSelect
          v-model="exitNodeValue"
          :nodes="exitNodeOptions"
          :placeholder="$t('bots.settings.networkExitNodePlaceholder')"
        />
        <p class="text-[10px] text-muted-foreground">
          {{ nodeListHint }}
        </p>
      </div>

      <!-- Node Meta details (Single Column) -->
      <div
        v-if="selectedExitNodeMeta"
        class="space-y-3 pt-3 border-t border-border/50"
      >
        <div class="space-y-1">
          <div class="text-[11px] font-medium text-muted-foreground">
            {{ $t('bots.settings.networkExitNodeStatus') }}
          </div>
          <div class="font-mono text-[10px] leading-relaxed tracking-tight break-all rounded px-2 py-1 bg-secondary/30 opacity-60 grayscale border border-transparent">
            {{ selectedExitNodeMeta.online ? $t('bots.settings.networkExitNodeOnline') : $t('bots.settings.networkExitNodeOffline') }}
          </div>
        </div>
        <div class="space-y-1">
          <div class="text-[11px] font-medium text-muted-foreground">
            {{ $t('bots.settings.networkExitNodeAddresses') }}
          </div>
          <div class="font-mono text-[10px] leading-relaxed tracking-tight break-all rounded px-2 py-1 bg-secondary/30 opacity-60 grayscale border border-transparent">
            {{ (selectedExitNodeMeta.addresses ?? []).join(', ') || '-' }}
          </div>
        </div>
      </div>
    </div>

    <!-- Edit Dialog (Modal IDE for Raw JSON) -->
    <Dialog v-model:open="isEditorDialogOpen">
      <DialogContent class="sm:max-w-3xl max-h-[calc(100vh - 2rem)] sm:h-[70vh] flex flex-col overflow-hidden p-0 gap-0">
        <DialogHeader class="shrink-0 p-4 border-b border-border/50 bg-background">
          <DialogTitle class="text-sm font-semibold">
            Edit Raw SD-WAN Configuration
          </DialogTitle>
          <DialogDescription class="text-[11px] leading-snug">
            Advanced configuration via raw JSON. Modifications will reflect in the visual form.
          </DialogDescription>
        </DialogHeader>
        
        <div class="flex-1 min-h-0 relative p-4 bg-muted/5">
          <div class="absolute inset-4 rounded-md border border-border/50 bg-background/50 overflow-hidden flex flex-col shadow-sm">
            <MonacoEditor
              v-model="editorDraftRaw"
              language="json"
              class="flex-1 min-h-0"
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
        </div>

        <DialogFooter class="shrink-0 p-4 border-t border-border/50 bg-background flex items-center justify-between gap-2">
          <p
            v-if="editorError"
            class="text-[10px] text-warning/80"
          >
            {{ editorError }}
          </p>
          <div v-else />
          <div class="flex items-center gap-2">
            <DialogClose as-child>
              <Button
                variant="ghost"
                size="sm"
                class="h-8 text-xs font-medium px-4 shadow-none"
              >
                {{ $t('common.cancel') }}
              </Button>
            </DialogClose>
            <Button
              size="sm"
              class="h-8 text-xs font-medium px-4 min-w-24 shadow-none bg-foreground text-background hover:bg-foreground/90"
              :disabled="!!editorError"
              @click="handleEditorSave"
            >
              {{ $t('common.save') }}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import {
  Label,
  Button,
  Spinner,
  Switch,
  Input,
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
  Textarea,
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter, DialogClose,
} from '@memohai/ui'
import { SquarePen, ChevronDown, ChevronRight, Eye, EyeOff } from 'lucide-vue-next'
import { reactive, computed, watch, nextTick, onBeforeUnmount, ref } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { getBotsByBotIdSettings, putBotsByBotIdSettings } from '@memohai/sdk'
import type { SettingsSettings } from '@memohai/sdk'
import { cloneConfig, getPathValue, setPathValue } from '@/components/config-schema-form/utils'
import type { ConfigSchema, ConfigSchemaField } from '@/components/config-schema-form/types'
import { resolveApiErrorMessage } from '@/utils/api-error'
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
    editorError.value = 'Invalid JSON format'
  }
})

function handleEditorSave() {
  try {
    const parsed = JSON.parse(editorDraftRaw.value)
    form.overlay_config = cloneConfig(parsed)
    isEditorDialogOpen.value = false
  } catch {
    // Should be caught by watcher, but just in case
    editorError.value = 'Invalid JSON format'
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
