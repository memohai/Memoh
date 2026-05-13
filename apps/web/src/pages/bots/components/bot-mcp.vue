<template>
  <div
    class="flex gap-6 h-full absolute inset-0 px-4 pt-4 pb-6 w-full max-w-4xl mx-auto font-sans"
    :style="{ fontFamily: 'system-ui, -apple-system, sans-serif' }"
  >
    <!-- Left: Server List (L3 Rail) -->
    <div 
      class="shrink-0 flex flex-col border border-border rounded-lg overflow-hidden max-h-full bg-background shadow-sm transition-all duration-300"
      :class="isMobileCollapsed ? 'w-12 items-center' : 'w-60'"
    >
      <div
        class="p-3 pb-2 border-b border-border/50 space-y-3 shrink-0 w-full"
        :class="{'px-1': isMobileCollapsed}"
      >
        <div class="flex items-center justify-between">
          <h4
            v-if="!isMobileCollapsed"
            class="text-xs font-medium"
          >
            {{ $t('mcp.servers') }}
          </h4>
          <div
            class="flex items-center gap-1"
            :class="{'flex-col': isMobileCollapsed}"
          >
            <Button
              variant="ghost"
              size="icon-sm"
              class="size-7 text-muted-foreground hover:text-foreground"
              :title="$t('mcp.addNew')"
              @click="handleAddNewDraft"
            >
              <Plus class="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              class="size-7 text-muted-foreground hover:text-foreground"
              :title="$t('common.import')"
              @click="startImport"
            >
              <Download class="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              class="size-7 md:hidden"
              @click="isMobileCollapsed = !isMobileCollapsed"
            >
              <Menu class="size-4" />
            </Button>
          </div>
        </div>
        <div
          v-if="!isMobileCollapsed"
          class="relative"
        >
          <Search class="absolute left-2.5 top-1/2 -translate-y-1/2 size-3 text-muted-foreground" />
          <input 
            v-model="searchText"
            class="w-full min-w-0 rounded-lg border border-border bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground transition-all outline-none focus:border-ring focus:ring-2 focus:ring-ring/20 disabled:opacity-50 pl-8 h-8 text-xs shadow-none" 
            :placeholder="$t('mcp.searchServers')" 
          >
        </div>
      </div>
      
      <ScrollArea
        ref="sidebarScrollRef"
        class="flex-1 min-h-0 bg-muted/5 w-full"
      >
        <section class="p-2 space-y-0.5">
          <div
            v-if="loading && items.length === 0"
            class="flex justify-center p-4"
          >
            <Spinner class="size-4 text-muted-foreground" />
          </div>
          <button 
            v-for="item in filteredItems" 
            :key="item.id === DRAFT_ID ? `_draft_${blinkKey}` : item.id"
            class="w-full text-left px-3 py-1.5 rounded-md text-xs transition-colors hover:bg-accent/40 group relative text-muted-foreground flex items-center gap-2"
            :class="{
              'bg-accent/60 text-foreground': selectedItem?.id === item.id
            }"
            @click="attemptSelectItem(item)"
          >
            <span
              class="size-2 rounded-full shrink-0 transition-colors"
              :class="statusDotClass(item)"
            />
            <span
              v-if="!isMobileCollapsed"
              class="truncate flex-1"
            >
              {{ item.name || (item.id === DRAFT_ID ? $t('mcp.unnamedServer') : $t('mcp.untitled')) }}
              <span
                v-if="!item.id"
                class="text-muted-foreground font-italic"
              > ({{ $t('mcp.draft') }})</span>
            </span>
            <span
              v-if="!isMobileCollapsed && isItemDirty(item)"
              class="size-1.5 rounded-full bg-muted-foreground shrink-0"
              :title="$t('mcp.unsavedChangesTitle')"
            />
          </button>
        </section>
      </ScrollArea>
    </div>

    <!-- Right: Workspace (L4) -->
    <div 
      class="relative flex-1 flex flex-col border rounded-lg overflow-hidden bg-background shadow-sm"
    >
      <!-- Blink Overlay (Avoids child z-index clipping) -->
      <div 
        class="pointer-events-none absolute inset-0 z-50 rounded-lg transition-opacity duration-500"
        :class="isBlinking ? 'opacity-100 ring-2 ring-foreground ring-inset' : 'opacity-0'"
        aria-hidden="true"
      />
      
      <!-- Normal Workspace -->
      <template v-if="selectedItem">
        <!-- Sovereign Header -->
        <div class="pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-10 p-4 shrink-0 flex items-start justify-between">
          <div class="flex items-center gap-3">
            <span class="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-muted/30 text-muted-foreground shadow-sm">
              <Plug class="size-4" />
            </span>
            <div class="space-y-0.5 flex flex-col justify-center">
              <h3 class="text-sm font-semibold text-foreground flex items-center gap-2">
                {{ formData.name || (selectedItem.id === DRAFT_ID ? $t('mcp.unnamedServer') : selectedItem.name) }}
                <span
                  class="size-2 rounded-full shrink-0 transition-colors"
                  :class="statusDotClass(selectedItem)"
                />
              </h3>
              <p class="text-[11px] text-muted-foreground font-mono leading-none">
                {{ $t('mcp.lastProbed') }}: {{ formatDate(selectedItem.last_probed_at) || $t('mcp.statusUnknown') }}
              </p>
            </div>
          </div>
          <div class="flex items-center gap-3 mt-1 shrink-0">
            <!-- Dynamic context micro-copy -->
            <Transition name="fade">
              <div
                v-if="selectedItem && isItemDirty(selectedItem)"
                class="flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-muted/40 border border-border/50"
              >
                <div class="size-1 rounded-full bg-muted-foreground/40" />
                <span class="text-[10px] text-muted-foreground font-medium whitespace-nowrap">
                  Unsaved
                </span>
              </div>
            </Transition>

            <button 
              v-if="selectedItem.id"
              type="button"
              class="inline-flex items-center justify-center whitespace-nowrap transition-all outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer border border-border bg-background hover:bg-accent rounded-lg gap-1.5 px-3 h-8 text-xs font-medium shadow-none"
              @click="handleExportSingle"
            >
              <Download class="size-3.5" /> {{ $t('common.export') }}
            </button>
            <button 
              v-if="selectedItem"
              type="button"
              :disabled="saveState === 'syncing' || !canProbe"
              class="inline-flex items-center justify-center whitespace-nowrap transition-all disabled:opacity-50 outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer border border-border bg-background hover:bg-accent rounded-lg gap-1.5 px-3 h-8 text-xs font-medium shadow-none group relative"
              @click="handleProbeInterruption"
            >
              <template v-if="saveState === 'verifying'">
                <X class="size-3.5 hidden group-hover:block text-destructive" />
                <RefreshCw class="size-3.5 animate-spin group-hover:hidden" />
                <span class="group-hover:text-destructive">{{ $t('mcp.verifyingCancel') }}</span>
              </template>
              <template v-else>
                <RefreshCw class="size-3.5" /> {{ $t('mcp.probe') }}
              </template>
            </button>
            <div
              class="w-px h-4 bg-border mx-1"
              aria-hidden="true"
            />
            <ConfirmPopover
              v-if="isDraft"
              :message="$t('mcp.discardDraftConfirm')"
              @confirm="removeDraft"
            >
              <template #trigger>
                <button 
                  type="button"
                  class="inline-flex items-center justify-center whitespace-nowrap transition-all outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer border border-border bg-background hover:bg-accent rounded-lg gap-1.5 px-3 h-8 text-xs font-medium shadow-none"
                >
                  {{ $t('mcp.discard') }}
                </button>
              </template>
            </ConfirmPopover>
            <button 
              type="button"
              :disabled="saveState === 'syncing' || saveState === 'verifying'"
              class="inline-flex items-center justify-center whitespace-nowrap transition-all disabled:opacity-50 outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer rounded-lg gap-1.5 px-3 h-8 text-xs font-medium min-w-24 shadow-none"
              :class="saveBtnClass"
              @click="handleSave"
            >
              <Loader2
                v-if="saveState === 'syncing'"
                class="size-3.5 animate-spin"
              />
              <Check
                v-else-if="saveState === 'connected'"
                class="size-3.5"
              />
              <Save
                v-else
                class="size-3.5"
              />
              {{ saveBtnText }}
            </button>
          </div>
        </div>

        <ScrollArea class="flex-1 min-h-0 bg-muted/5">
          <section class="max-w-2xl mx-auto p-4 pb-12 space-y-4">
            <!-- Tier 1 Error Contextual Helper (Sync Error) -->
            <div
              v-if="tier1Error"
              class="rounded-md border border-destructive/50 bg-destructive/5 p-3 flex items-start gap-2"
            >
              <AlertCircle class="size-4 text-destructive shrink-0 mt-0.5" />
              <div class="text-xs text-destructive flex-1">
                <strong class="font-medium">{{ $t('mcp.saveFailed') }}:</strong> {{ tier1ErrorMessage }}
                <p class="text-[10px] opacity-80 mt-1 leading-relaxed">
                  {{ $t('mcp.invalidConfig') }}
                </p>
              </div>
            </div>

            <!-- Tier 2 Error Contextual Helper (Probe Error) -->
            <div
              v-if="tier2Error"
              class="rounded-md border border-warning/30 bg-warning/5 p-3 flex items-start gap-2"
            >
              <ZapOff class="size-4 text-warning shrink-0 mt-0.5" />
              <div class="text-xs text-warning flex-1">
                <strong class="font-medium">{{ $t('mcp.probeFailed') }}:</strong> {{ selectedItem?.status_message || $t('mcp.handshakeError') }}
                <div class="mt-2">
                  <button
                    type="button"
                    class="inline-flex items-center justify-center whitespace-nowrap transition-all outline-none cursor-pointer border border-warning/30 bg-transparent hover:bg-warning/10 rounded-lg px-2 h-6 text-[10px] font-medium text-warning shadow-none"
                    @click="showRawLog = true"
                  >
                    {{ $t('mcp.viewRawLog') }}
                  </button>
                </div>
              </div>
            </div>

            <!-- Block A: Identity & Protocol -->
            <div 
              class="rounded-md border border-border bg-background p-4 shadow-none space-y-4 transition-opacity duration-300" 
              :class="[
                tier1Error ? 'border-destructive/50' : '',
                (isOAuthSpotlight && !showAdvanced) ? 'opacity-40 pointer-events-none' : ''
              ]"
            >
              <div class="space-y-1">
                <h4 class="text-xs font-medium">
                  {{ $t('mcp.identity') }}
                </h4>
                <Badge
                  v-if="selectedItem.status === 'connected'"
                  variant="outline"
                  class="text-[10px] text-success"
                >
                  {{ $t('mcp.statusConnected') }}
                </Badge>
                <Badge
                  v-else-if="selectedItem.status === 'error'"
                  variant="outline"
                  class="text-[10px] text-destructive"
                >
                  {{ $t('mcp.statusError') }}
                </Badge>
                <Badge
                  v-else
                  variant="outline"
                  class="text-[10px] text-muted-foreground"
                >
                  {{ $t('mcp.statusUnknown') }}
                </Badge>
              </div>
              <div class="space-y-4 pt-2">
                <div class="space-y-1.5 flex flex-col">
                  <label class="flex items-center gap-2 select-none text-xs font-medium">{{ $t('common.name') }}</label>
                  <input
                    v-model="formData.name"
                    class="w-full min-w-0 rounded-lg border border-border bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground transition-all outline-none focus:border-ring focus:ring-2 focus:ring-ring/20 h-8 text-xs shadow-none"
                    :placeholder="$t('mcp.placeholders.name')"
                  >
                </div>
                <div class="space-y-1.5 flex flex-col">
                  <label class="flex items-center gap-2 select-none text-xs font-medium">{{ $t('mcp.transportType') }}</label>
                  <div class="flex bg-muted/50 p-1 rounded-lg h-9 border border-border/50 gap-1">
                    <button
                      type="button"
                      class="flex-1 text-xs font-medium rounded-md transition-all duration-200 flex items-center justify-center gap-1.5"
                      :class="connectionType === 'stdio' ? 'bg-background shadow-sm text-foreground ring-1 ring-border/50' : 'text-muted-foreground hover:text-foreground hover:bg-muted/80'"
                      @click="connectionType = 'stdio'"
                    >
                      <Check
                        v-if="connectionType === 'stdio'"
                        class="size-3"
                      />
                      {{ $t('mcp.types.stdio') }}
                    </button>
                    <button
                      type="button"
                      class="flex-1 text-xs font-medium rounded-md transition-all duration-200 flex items-center justify-center gap-1.5"
                      :class="connectionType === 'remote' ? 'bg-background shadow-sm text-foreground ring-1 ring-border/50' : 'text-muted-foreground hover:text-foreground hover:bg-muted/80'"
                      @click="connectionType = 'remote'"
                    >
                      <Check
                        v-if="connectionType === 'remote'"
                        class="size-3"
                      />
                      {{ $t('mcp.types.remote') }}
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <!-- Block B: Technical Payload -->
            <div 
              class="rounded-md border border-border bg-background p-4 shadow-none space-y-4 transition-opacity duration-300"
              :class="(isOAuthSpotlight && !showAdvanced) ? 'opacity-40 pointer-events-none' : ''"
            >
              <div class="space-y-1">
                <h4 class="text-xs font-medium">
                  {{ $t('mcp.payload') }}
                </h4>
              </div>
              
              <div class="space-y-4 pt-2">
                <template v-if="connectionType === 'stdio'">
                  <div class="space-y-1.5 flex flex-col">
                    <label class="flex items-center gap-2 select-none text-xs font-medium">{{ $t('mcp.command') }}</label>
                    <input
                      v-model="formData.command"
                      class="w-full min-w-0 rounded-lg border border-border bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground transition-all outline-none focus:border-ring focus:ring-2 focus:ring-ring/20 h-8 text-xs shadow-none font-mono"
                      :placeholder="$t('mcp.commandPlaceholder')"
                    >
                  </div>
                  <div class="space-y-1.5 flex flex-col">
                    <label class="flex items-center gap-2 select-none text-xs font-medium">{{ $t('mcp.arguments') }}</label>
                    <TagsInput
                      v-model="argsTags"
                      :add-on-blur="true"
                      :duplicate="true"
                      class="text-xs min-h-8 font-mono border-border bg-transparent shadow-none rounded-lg"
                    >
                      <TagsInputItem
                        v-for="tag in argsTags"
                        :key="tag"
                        :value="tag"
                      >
                        <TagsInputItemText />
                        <TagsInputItemDelete />
                      </TagsInputItem>
                      <TagsInputInput
                        :placeholder="$t('mcp.argumentsPlaceholder')"
                        class="w-full py-1 text-xs"
                      />
                    </TagsInput>
                  </div>
                  <div class="space-y-1.5 flex flex-col">
                    <label class="flex items-center gap-2 select-none text-xs font-medium">{{ $t('mcp.cwd') }}</label>
                    <input
                      v-model="formData.cwd"
                      class="w-full min-w-0 rounded-lg border border-border bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground transition-all outline-none focus:border-ring focus:ring-2 focus:ring-ring/20 h-8 text-xs shadow-none font-mono"
                      :placeholder="$t('mcp.cwdPlaceholder')"
                    >
                  </div>
                </template>

                <template v-else>
                  <div class="space-y-1.5 flex flex-col">
                    <label class="flex items-center gap-2 select-none text-xs font-medium">{{ $t('mcp.endpointUrl') }}</label>
                    <div class="relative">
                      <input
                        v-model="formData.url"
                        class="w-full min-w-0 rounded-lg border border-border bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground transition-all outline-none focus:border-ring focus:ring-2 focus:ring-ring/20 h-8 text-xs shadow-none font-mono pr-8"
                        :placeholder="$t('mcp.placeholders.url')"
                        @blur="validateUrl"
                      >
                      <div class="absolute right-2 top-1/2 -translate-y-1/2 flex items-center">
                        <Check
                          v-if="isUrlValid === true"
                          class="size-3.5 text-success"
                        />
                        <AlertCircle
                          v-else-if="isUrlValid === false"
                          class="size-3.5 text-destructive"
                        />
                      </div>
                    </div>
                  </div>
                  <div class="space-y-1.5 flex flex-col">
                    <label class="flex items-center gap-2 select-none text-xs font-medium">{{ $t('mcp.streamProtocol') }}</label>
                    <Select v-model="formData.transport">
                      <SelectTrigger class="h-8 text-xs shadow-none border-border rounded-lg bg-background">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem
                          value="http"
                          class="text-xs"
                        >
                          {{ $t('mcp.protocol.http') }}
                        </SelectItem>
                        <SelectItem
                          value="sse"
                          class="text-xs"
                        >
                          {{ $t('mcp.protocol.sse') }}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </template>
              </div>
            </div>

            <!-- Advanced Settings Collapsible Area -->
            <div
              class="rounded-md border border-border bg-background p-4 shadow-none space-y-4 transition-all duration-300"
              :class="{'ring-2 ring-primary/20 border-primary/30 shadow-md': isOAuthSpotlight}"
            >
              <div class="flex items-center justify-between">
                <div class="space-y-1">
                  <h4 class="text-xs font-medium">
                    {{ $t('mcp.advancedSettings') }}
                  </h4>
                  <p class="text-[11px] text-muted-foreground">
                    {{ $t('mcp.advancedSettingsHint') }}
                  </p>
                </div>
                <div class="flex gap-2">
                  <button
                    v-if="!showAdvanced"
                    type="button"
                    class="inline-flex items-center justify-center whitespace-nowrap font-medium transition-all disabled:opacity-50 outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer hover:bg-accent bg-transparent rounded-lg h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
                    @click="showAdvanced = true"
                  >
                    {{ $t('mcp.expand') }}
                  </button>
                  <button
                    v-if="showAdvanced"
                    type="button"
                    class="inline-flex items-center justify-center whitespace-nowrap font-medium transition-all disabled:opacity-50 outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer hover:bg-accent bg-transparent rounded-lg h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
                    @click="showAdvanced = false"
                  >
                    {{ $t('mcp.collapse') }}
                  </button>
                </div>
              </div>
              
              <div
                class="pt-4 border-t border-border/50 space-y-4"
                :style="{ display: showAdvanced || isOAuthSpotlight ? 'block' : 'none' }"
              >
                <!-- Sub Block: Environment / Headers -->
                <div class="space-y-3">
                  <h5 class="text-[11px] font-medium text-foreground uppercase tracking-wider">
                    {{ connectionType === 'stdio' ? $t('mcp.envVars') : $t('mcp.httpHeaders') }}
                  </h5>
                  <div v-if="connectionType === 'stdio'">
                    <KeyValueEditor
                      v-model="envPairs"
                      :key-placeholder="$t('mcp.placeholders.envKey')"
                      :value-placeholder="$t('mcp.placeholders.envValue')"
                    />
                  </div>
                  <div v-else>
                    <KeyValueEditor
                      v-model="headerPairs"
                      :key-placeholder="$t('mcp.placeholders.headerKey')"
                      :value-placeholder="$t('mcp.placeholders.headerValue')"
                    />
                  </div>
                </div>

                <!-- Sub Block: OAuth (Remote Only) -->
                <div
                  v-if="connectionType === 'remote'"
                  class="space-y-3 pt-2 border-t border-border/50"
                >
                  <div class="flex items-center justify-between">
                    <h5 class="text-[11px] font-medium text-foreground uppercase tracking-wider flex items-center gap-2">
                      <Lock class="size-3 text-muted-foreground" /> {{ $t('mcp.oauth.title') }}
                    </h5>
                    <div
                      class="inline-flex items-center justify-center rounded-sm border font-medium w-fit whitespace-nowrap shrink-0 px-2 py-0.5 text-[9px] shadow-none uppercase tracking-wide"
                      :class="oauthStatus?.has_token ? 'bg-success/10 text-success border-success/20' : 'text-muted-foreground bg-muted border-border'"
                    >
                      {{ oauthStatus?.has_token ? $t('mcp.oauth.authorized') : $t('mcp.notConfigured') }}
                    </div>
                  </div>
                  
                  <div
                    v-if="oauthNeedsClientId && (!oauthStatus?.has_token || oauthStatus?.expired)"
                    class="space-y-3 p-3 bg-muted/20 border border-border/50 rounded-lg"
                  >
                    <div class="space-y-1.5 flex flex-col">
                      <label class="flex items-center gap-2 select-none text-xs font-medium">{{ $t('mcp.oauth.clientId') }}</label>
                      <input
                        v-model="oauthClientId"
                        class="w-full min-w-0 rounded-lg border border-border bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground transition-all outline-none focus:border-ring focus:ring-2 focus:ring-ring/20 h-8 text-xs shadow-none font-mono"
                        :placeholder="$t('mcp.oauth.clientIdPlaceholder')"
                      >
                    </div>
                    <div class="space-y-1.5 flex flex-col">
                      <label class="flex items-center gap-2 select-none text-xs font-medium">{{ $t('mcp.oauth.clientSecret') }}</label>
                      <div class="relative">
                        <input
                          v-model="oauthClientSecret"
                          :type="showSecret ? 'text' : 'password'"
                          class="w-full min-w-0 rounded-lg border border-border bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground transition-all outline-none focus:border-ring focus:ring-2 focus:ring-ring/20 h-8 text-xs shadow-none font-mono pr-16"
                          :placeholder="$t('mcp.oauth.clientSecretPlaceholder')"
                        >
                        <div class="absolute right-1 top-1/2 -translate-y-1/2 flex gap-1">
                          <button
                            type="button"
                            class="inline-flex items-center justify-center whitespace-nowrap text-muted-foreground hover:text-foreground bg-transparent rounded-md size-6 p-0 outline-none"
                            @click="showSecret = !showSecret"
                          >
                            <Eye
                              v-if="!showSecret"
                              class="size-3.5"
                            />
                            <EyeOff
                              v-else
                              class="size-3.5"
                            />
                          </button>
                          <button
                            type="button"
                            class="inline-flex items-center justify-center whitespace-nowrap text-muted-foreground hover:text-foreground bg-transparent rounded-md size-6 p-0 outline-none"
                            @click="openModalEditor($t('mcp.oauth.clientSecret'), oauthClientSecret, (v: string) => oauthClientSecret = v)"
                          >
                            <Maximize2 class="size-3.5" />
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                  
                  <div>
                    <button
                      v-if="!oauthStatus?.has_token"
                      type="button"
                      :disabled="oauthDiscovering || oauthAuthorizing"
                      class="inline-flex items-center justify-center whitespace-nowrap font-medium transition-all disabled:opacity-50 outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer rounded-lg gap-1.5 px-3 h-8 text-xs shadow-none bg-foreground text-background hover:bg-foreground/90"
                      @click="handleOAuthFlow"
                    >
                      <Loader2
                        v-if="oauthDiscovering || oauthAuthorizing"
                        class="size-3.5 mr-1.5 animate-spin"
                      />
                      <KeyRound
                        v-else
                        class="size-3.5 mr-1.5"
                      />
                      {{ oauthDiscovering ? $t('mcp.oauth.discovering') : $t('mcp.oauth.authorize') }}
                    </button>
                    <button
                      v-else
                      type="button"
                      class="inline-flex items-center justify-center whitespace-nowrap font-medium transition-all outline-none cursor-pointer border border-border bg-background hover:bg-accent rounded-lg gap-1.5 px-3 h-8 text-xs shadow-none"
                      @click="handleOAuthRevoke"
                    >
                      {{ $t('mcp.oauth.revoke') }}
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <!-- Discovered Tools Summary -->
            <div
              v-if="selectedItem.id && selectedItem.status === 'connected'"
              class="rounded-md border border-border bg-background p-4 shadow-none space-y-3"
            >
              <div class="flex items-center justify-between border-b border-border/50 pb-2">
                <h4 class="text-xs font-medium flex items-center gap-2">
                  <Wrench class="size-3 text-muted-foreground" /> {{ $t('mcp.discoveredTools') }}
                </h4>
                <button
                  type="button"
                  class="inline-flex items-center justify-center whitespace-nowrap font-medium transition-all outline-none cursor-pointer hover:underline text-[10px] text-muted-foreground hover:text-foreground"
                  @click="openToolsModal"
                >
                  {{ $t('common.viewAll') }} ({{ displayTools.length }})
                </button>
              </div>
              <div
                v-if="displayTools.length === 0"
                class="text-[11px] text-muted-foreground py-1"
              >
                {{ $t('mcp.noToolsExposed') }}
              </div>
              <div
                v-else
                class="flex flex-wrap gap-1.5"
              >
                <Badge
                  v-for="tool in displayTools.slice(0, 5)"
                  :key="tool.name"
                  variant="secondary"
                  class="text-[10px] font-mono hover:bg-secondary/80 cursor-default shadow-none border border-border bg-muted max-w-[140px] truncate block"
                  :title="tool.description"
                >
                  {{ tool.name }}
                </Badge>
                <button
                  v-if="displayTools.length > 5"
                  type="button"
                  class="text-[10px] font-medium text-muted-foreground hover:text-foreground transition-colors ml-1"
                  @click="openToolsModal"
                >
                  +{{ displayTools.length - 5 }} {{ $t('mcp.more') }}
                </button>
              </div>
            </div>

            <!-- Stoic Danger Zone -->
            <div
              v-if="selectedItem.id && selectedItem.id !== DRAFT_ID"
              class="pt-4 mt-8"
            >
              <div class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none">
                <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                  <div class="space-y-0.5">
                    <h4 class="text-xs font-medium text-destructive">
                      {{ $t('common.dangerZone') }}
                    </h4>
                    <p class="text-[11px] text-muted-foreground">
                      {{ $t('mcp.dangerZoneHint') }}
                    </p>
                  </div>
                  <div class="flex justify-end shrink-0">
                    <ConfirmPopover
                      :message="$t('mcp.deleteConfirm')"
                      @confirm="handleDelete(selectedItem!)"
                    >
                      <template #trigger>
                        <button
                          type="button"
                          class="inline-flex items-center justify-center whitespace-nowrap transition-all outline-none cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90 rounded-lg px-3 min-w-28 h-8 text-xs font-medium shadow-none"
                        >
                          {{ $t('common.delete') }}
                        </button>
                      </template>
                    </ConfirmPopover>
                  </div>
                </div>
              </div>
            </div>
          </section>
        </ScrollArea>
      </template>

      <!-- Empty State Workspace -->
      <div
        v-else
        class="flex-1 flex flex-col items-center justify-center bg-muted/5 p-8"
      >
        <div class="mb-4 flex shrink-0 items-center justify-center bg-transparent">
          <Plug class="size-8 text-muted-foreground opacity-50" />
        </div>
        <h2 class="tracking-tight text-xs font-medium mt-4">
          {{ $t('mcp.emptyTitle') }}
        </h2>
        <p class="text-[11px] text-muted-foreground text-center max-w-sm mt-2 mb-6 leading-relaxed">
          {{ $t('mcp.emptyDescription') }}
        </p>
        <button
          type="button"
          class="inline-flex items-center justify-center whitespace-nowrap text-xs font-medium transition-all outline-none cursor-pointer border border-border bg-background hover:bg-accent h-8 rounded-lg gap-1.5 px-3 has-[>svg]:px-2.5 w-fit shadow-none"
          @click="handleAddNewDraft"
        >
          <Plus class="size-4 mr-1.5" /> {{ $t('mcp.addNew') }}
        </button>
      </div>
    </div>
  </div>

  <!-- Dialogs & Modals Below -->

  <!-- Import Modal (Centered, Large Editor) -->
  <Dialog v-model:open="importDialogOpen">
    <DialogContent
      :show-close-button="false"
      class="sm:max-w-4xl p-0 overflow-hidden border border-border shadow-2xl h-[85vh] flex flex-col bg-background"
    >
      <div class="px-5 py-4 border-b border-border/50 bg-muted/10 flex justify-between items-center shrink-0">
        <div class="space-y-1">
          <h2 class="text-sm font-medium text-foreground">
            {{ $t('mcp.importSandbox') }}
          </h2>
          <div class="flex items-center gap-2">
            <p class="text-[10px] text-muted-foreground font-mono bg-background border border-border px-1.5 py-0.5 rounded">
              .gemini/config/mcpServers.json
            </p>
            <button
              type="button"
              class="inline-flex items-center justify-center size-5 rounded hover:bg-background text-muted-foreground hover:text-foreground transition-colors"
              @click="copyText('.gemini/config/mcpServers.json'); toast.success($t('mcp.pathCopied'))"
            >
              <Copy class="size-3" />
            </button>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="inline-flex items-center justify-center whitespace-nowrap outline-none cursor-pointer border border-border bg-background hover:bg-accent rounded-lg px-2 h-7 text-[10px] font-medium shadow-none text-muted-foreground hover:text-foreground"
            @click="formatImportJson"
          >
            { } {{ $t('common.format') }}
          </button>
          <DialogClose as-child>
            <button
              type="button"
              class="inline-flex items-center justify-center size-7 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
            >
              <X class="size-4" />
            </button>
          </DialogClose>
        </div>
      </div>
      
      <div class="flex-1 min-h-0 bg-muted/5 p-4 flex flex-col relative">
        <div class="flex-1 rounded-md border border-border overflow-hidden shadow-none bg-background relative z-0">
          <MonacoEditor
            v-model="importJson"
            language="json"
          />
        </div>
      </div>

      <!-- Sticky Error Banner -->
      <div
        v-if="importError"
        class="px-5 py-3 border-t border-destructive/30 bg-destructive/10 flex items-center gap-2 shrink-0"
      >
        <AlertCircle class="size-4 text-destructive shrink-0" />
        <p class="text-xs text-destructive flex-1 font-mono">
          {{ importError }}
        </p>
      </div>

      <div class="px-5 py-4 border-t border-border/50 bg-muted/10 flex justify-end gap-2 shrink-0">
        <DialogClose as-child>
          <button
            type="button"
            class="inline-flex items-center justify-center whitespace-nowrap outline-none cursor-pointer border border-border bg-background hover:bg-accent rounded-lg px-3 h-8 text-xs font-medium shadow-none"
          >
            {{ $t('common.cancel') }}
          </button>
        </DialogClose>
        <button
          type="button"
          :disabled="importSubmitting || !importJson.trim()"
          class="inline-flex items-center justify-center whitespace-nowrap outline-none cursor-pointer bg-foreground text-background hover:bg-foreground/90 rounded-lg px-3 h-8 text-xs font-medium shadow-none disabled:opacity-50"
          @click="executeImport"
        >
          <Loader2
            v-if="importSubmitting"
            class="size-3.5 mr-1.5 animate-spin"
          />
          {{ $t('mcp.blindImport') }}
        </button>
      </div>
    </DialogContent>
  </Dialog>

  <!-- Complete Tools List Modal -->
  <Dialog v-model:open="showToolsModal">
    <DialogContent
      :show-close-button="false"
      class="sm:max-w-3xl p-0 overflow-hidden border border-border shadow-2xl h-[80vh] flex flex-col bg-background"
    >
      <div class="px-5 py-4 border-b border-border/50 bg-muted/10 flex justify-between items-center shrink-0">
        <h2 class="text-sm font-medium text-foreground">
          {{ $t('mcp.allTools') }}
        </h2>
        <DialogClose as-child>
          <button
            type="button"
            class="inline-flex items-center justify-center size-7 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
          >
            <X class="size-4" />
          </button>
        </DialogClose>
      </div>
      <div class="p-4 border-b border-border/50 bg-background shrink-0">
        <div class="relative">
          <Search class="absolute left-3 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground" />
          <input
            v-model="toolsSearchText"
            class="w-full min-w-0 rounded-lg border border-border bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground transition-all outline-none focus:border-ring focus:ring-2 focus:ring-ring/20 h-9 text-xs shadow-none pl-9"
            :placeholder="$t('mcp.searchTools')"
          >
        </div>
      </div>
      <ScrollArea class="flex-1 bg-muted/5 p-4">
        <section class="space-y-3 max-w-3xl mx-auto pb-8">
          <div
            v-for="tool in filteredTools"
            :key="tool.name"
            class="p-4 rounded-md border border-border bg-background shadow-none space-y-2"
          >
            <h4 class="text-xs font-medium font-mono text-foreground">
              {{ tool.name }}
            </h4>
            <p class="text-[11px] text-muted-foreground leading-relaxed">
              {{ tool.description || $t('mcp.noDescription') }}
            </p>
          </div>
          <div
            v-if="filteredTools.length === 0"
            class="text-xs text-muted-foreground text-center py-12"
          >
            {{ $t('mcp.noToolsMatch') }}
          </div>
        </section>
      </ScrollArea>
    </DialogContent>
  </Dialog>

  <!-- Diagnostics Raw Log Popover (Tier 2 Error) -->
  <Dialog v-model:open="showRawLog">
    <DialogContent
      :show-close-button="false"
      class="sm:max-w-2xl p-0 overflow-hidden border border-border shadow-2xl bg-background"
    >
      <div class="px-4 py-3 border-b border-border/50 bg-muted/10 flex justify-between items-center">
        <h2 class="text-xs font-medium text-foreground">
          {{ $t('mcp.diagnosticLog') }}
        </h2>
        <DialogClose as-child>
          <button
            type="button"
            class="inline-flex items-center justify-center size-7 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
          >
            <X class="size-4" />
          </button>
        </DialogClose>
      </div>
      <ScrollArea class="h-64 bg-background">
        <pre class="p-4 text-[10px] font-mono text-muted-foreground whitespace-pre-wrap break-all">{{ selectedItem?.status_message || $t('mcp.noLog') }}</pre>
      </ScrollArea>
      <div class="p-3 border-t border-border/50 bg-muted/10 flex justify-end">
        <button
          type="button"
          class="inline-flex items-center justify-center whitespace-nowrap outline-none cursor-pointer border border-border bg-background hover:bg-accent rounded-lg px-3 h-7 text-[10px] font-medium shadow-none"
          @click="copyText(selectedItem?.status_message || ''); toast.success($t('common.copied'))"
        >
          {{ $t('mcp.copyLog') }}
        </button>
      </div>
    </DialogContent>
  </Dialog>

  <!-- Modal Editor (Long Text Focused Edit) -->
  <Dialog v-model:open="showModalEditor">
    <DialogContent
      :show-close-button="false"
      class="sm:max-w-4xl p-0 overflow-hidden border border-border shadow-2xl h-[85vh] flex flex-col bg-background"
    >
      <div class="px-5 py-4 border-b border-border/50 bg-muted/10 flex justify-between items-center shrink-0">
        <h2 class="text-sm font-medium text-foreground">
          {{ $t('mcp.editValue', { value: modalEditorTitle }) }}
        </h2>
        <DialogClose as-child>
          <button
            type="button"
            class="inline-flex items-center justify-center size-7 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
          >
            <X class="size-4" />
          </button>
        </DialogClose>
      </div>
      <div class="flex-1 flex min-h-0 bg-background">
        <div class="w-1/2 border-r border-border p-5 flex flex-col bg-muted/5">
          <p class="text-[11px] text-muted-foreground leading-relaxed">
            {{ $t('mcp.editLongTextHint') }}
          </p>
        </div>
        <div class="w-1/2 p-4">
          <textarea
            v-model="modalEditorValue"
            class="w-full h-full resize-none border-0 rounded-md text-xs font-mono text-foreground focus:outline-none focus:ring-0 bg-transparent"
            :placeholder="$t('mcp.enterContent')"
          />
        </div>
      </div>
      <div class="px-5 py-4 border-t border-border/50 bg-muted/10 flex justify-end gap-2 shrink-0">
        <button
          type="button"
          class="inline-flex items-center justify-center whitespace-nowrap outline-none cursor-pointer border border-border bg-background hover:bg-accent rounded-lg px-3 h-8 text-xs font-medium shadow-none"
          @click="showModalEditor = false"
        >
          {{ $t('common.cancel') }}
        </button>
        <button
          type="button"
          class="inline-flex items-center justify-center whitespace-nowrap outline-none cursor-pointer bg-foreground text-background hover:bg-foreground/90 rounded-lg px-3 h-8 text-xs font-medium shadow-none"
          @click="confirmModalEditor"
        >
          {{ $t('common.confirm') }}
        </button>
      </div>
    </DialogContent>
  </Dialog>

  <!-- Route Intercept Dialog -->
  <Dialog v-model:open="showInterceptDialog">
    <DialogContent class="sm:max-w-sm p-0 overflow-hidden border border-border shadow-2xl bg-background">
      <div class="p-5 bg-background space-y-3">
        <h3 class="text-sm font-medium text-foreground">
          {{ $t('mcp.unsavedChangesTitle') }}
        </h3>
        <p class="text-xs text-muted-foreground leading-relaxed">
          {{ $t('mcp.unsavedChangesDesc') }}
        </p>
      </div>
      <div class="p-4 border-t border-border/50 bg-muted/10 flex justify-end gap-2">
        <button
          type="button"
          class="inline-flex items-center justify-center whitespace-nowrap outline-none cursor-pointer border border-border bg-background hover:bg-accent rounded-lg px-3 h-8 text-xs font-medium shadow-none"
          @click="showInterceptDialog = false"
        >
          {{ $t('mcp.keepEditing') }}
        </button>
        <button
          type="button"
          class="inline-flex items-center justify-center whitespace-nowrap outline-none cursor-pointer bg-foreground text-background hover:bg-foreground/90 rounded-lg px-3 h-8 text-xs font-medium shadow-none"
          @click="confirmIntercept"
        >
          {{ $t('mcp.discardAndSwitch') }}
        </button>
      </div>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { 
  Search, Plus, RefreshCw, Lock, Copy, KeyRound, Wrench, Plug, Check, AlertCircle, ZapOff, 
  Maximize2, Eye, EyeOff, Loader2, Save, X, Download, Menu
} from 'lucide-vue-next'
import { computed, nextTick, ref, watch, type ComponentPublicInstance } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { useQueryCache } from '@pinia/colada'
import {
  Badge, Button, Dialog, DialogClose, DialogContent, ScrollArea, Select, 
  SelectContent, SelectItem, SelectTrigger, SelectValue, Spinner, TagsInput, TagsInputInput, 
  TagsInputItem, TagsInputItemDelete, TagsInputItemText
} from '@memohai/ui'
import MonacoEditor from '@/components/monaco-editor/index.vue'
import KeyValueEditor from '@/components/key-value-editor/index.vue'
import type { KeyValuePair } from '@/components/key-value-editor/index.vue'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import {
  getBotsByBotIdMcp, postBotsByBotIdMcp, putBotsByBotIdMcpById, deleteBotsByBotIdMcpById,
  postBotsByBotIdMcpByIdProbe, putBotsByBotIdMcpImport, getBotsByBotIdMcpByIdOauthStatus,
  postBotsByBotIdMcpByIdOauthDiscover, postBotsByBotIdMcpByIdOauthAuthorize, deleteBotsByBotIdMcpByIdOauthToken,
} from '@memohai/sdk'
import type {
  McpUpsertRequest, McpImportRequest, McpToolDescriptor, McpMcpServerEntry, McpOAuthStatus,
} from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { useClipboard } from '@/composables/useClipboard'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'
import { useSupermarketMcpDraft } from '@/stores/supermarket-mcp-draft'

interface McpItem {
  id: string
  name: string
  type: string
  config: Record<string, unknown>
  is_active: boolean
  status: string
  tools_cache: McpToolDescriptor[]
  last_probed_at: string | null
  status_message: string
  auth_type: string
}

const DRAFT_ID = ''
const props = defineProps<{ botId: string }>()
const { t } = useI18n()
const { copyText } = useClipboard()
const queryCache = useQueryCache()

const loading = ref(false)
const items = ref<McpItem[]>([])
const selectedItem = ref<McpItem | null>(null)
const selectedMcpId = useSyncedQueryParam('mcpId', '')
const searchText = ref('')

const isMobileCollapsed = ref(false)

const isBlinking = ref(false)
const blinkKey = ref(0)
const sidebarScrollRef = ref<ComponentPublicInstance | null>(null)

// Automata Flow States
const saveState = ref<'idle' | 'syncing' | 'verifying' | 'connected' | 'error'>('idle')
const tier1Error = ref(false)
const tier1ErrorMessage = ref('')
const tier2Error = ref(false)
let probeAbortController: AbortController | null = null

// Import Flow
const importDialogOpen = ref(false)
const importJson = ref('{\n  "mcpServers": {\n    \n  }\n}')
const importSubmitting = ref(false)
const importError = ref('')

// Modals & UI Toggles
const showAdvanced = ref(false)
const showSecret = ref(false)
const isOAuthSpotlight = computed(() => {
  return connectionType.value === 'remote' && probeAuthRequired.value && !oauthStatus.value?.has_token
})

const showRawLog = ref(false)
const showToolsModal = ref(false)
const toolsSearchText = ref('')

// Modal Editor
const showModalEditor = ref(false)
const modalEditorTitle = ref('')
const modalEditorValue = ref('')
let modalEditorCallback: ((val: string) => void) | null = null

// Intercept Flow
const showInterceptDialog = ref(false)
let interceptTargetItem: McpItem | null = null
let isCreatingNewAfterDiscard = false

// Form State
const connectionType = ref<'stdio' | 'remote'>('stdio')
const formData = ref({ name: '', command: '', url: '', cwd: '', transport: 'http' as 'http'|'sse', active: true })
const argsTags = ref<string[]>([])
const envPairs = ref<KeyValuePair[]>([])
const headerPairs = ref<KeyValuePair[]>([])

// OAuth State
const probeAuthRequired = ref(false)
const oauthDiscovering = ref(false)
const oauthAuthorizing = ref(false)
const oauthStatus = ref<McpOAuthStatus | null>(null)
const oauthClientId = ref('')
const oauthClientSecret = ref('')
const oauthNeedsClientId = ref(false)
const oauthCallbackUrl = ref('')
const oauthDiscovered = ref(false)

const isUrlValid = ref<boolean | null>(null)

// Computeds
const isDraft = computed(() => selectedItem.value?.id === DRAFT_ID)
const canProbe = computed(() => {
  if (!selectedItem.value) return false
  if (connectionType.value === 'stdio') {
    return formData.value.command.trim() !== ''
  } else {
    return formData.value.url.trim() !== '' && isUrlValid.value !== false
  }
})
const filteredItems = computed(() => {
  if (!searchText.value) return items.value
  const kw = searchText.value.toLowerCase()
  return items.value.filter(i => i.id === DRAFT_ID || i.name.toLowerCase().includes(kw))
})
const displayTools = computed(() => selectedItem.value?.tools_cache ?? [])
const filteredTools = computed(() => {
  if (!toolsSearchText.value) return displayTools.value
  const kw = toolsSearchText.value.toLowerCase()
  return displayTools.value.filter(t => (t.name || '').toLowerCase().includes(kw) || (t.description || '').toLowerCase().includes(kw))
})

const saveBtnText = computed(() => {
  if (saveState.value === 'syncing') return t('mcp.syncing')
  if (saveState.value === 'verifying') return t('mcp.verifying')
  if (saveState.value === 'connected') return t('mcp.statusConnected')
  return isDraft.value ? t('mcp.createServer') : t('mcp.saveConfig')
})

const saveBtnClass = computed(() => {
  if (saveState.value === 'connected') return 'bg-success hover:bg-success/90 text-success-solid-foreground border-success'
  if (saveState.value === 'error') return 'bg-destructive hover:bg-destructive/90 text-white border-destructive'
  return 'bg-foreground text-background hover:bg-foreground/90 border-foreground'
})

function statusDotClass(item: McpItem): string {
  if (!item.id || !item.is_active) return 'bg-muted-foreground/40'
  if (item.status === 'connected') return 'bg-success'
  if (item.status === 'error') return 'bg-destructive'
  return 'bg-warning'
}

function formatDate(dateStr: string | null) {
  if (!dateStr) return ''
  try { return new Date(dateStr).toLocaleString() } catch { return dateStr }
}

function isItemDirty(item: McpItem) {
  if (item.id !== selectedItem.value?.id) return false
  
  // For drafts, we consider it dirty if any key field is filled
  if (item.id === DRAFT_ID) {
    return formData.value.name.trim() !== '' || 
           (connectionType.value === 'stdio' && formData.value.command.trim() !== '') ||
           (connectionType.value === 'remote' && formData.value.url.trim() !== '')
  }

  return saveState.value === 'idle' && (
     formData.value.name !== item.name ||
     (connectionType.value === 'stdio' && formData.value.command !== configValue(item.config, 'command')) ||
     (connectionType.value === 'remote' && formData.value.url !== configValue(item.config, 'url'))
  )
}

function validateUrl() {
  if (!formData.value.url) { isUrlValid.value = null; return }
  isUrlValid.value = formData.value.url.startsWith('http://') || formData.value.url.startsWith('https://')
}

function openModalEditor(title: string, currentVal: string, callback: (v: string) => void) {
  modalEditorTitle.value = title
  modalEditorValue.value = currentVal
  modalEditorCallback = callback
  showModalEditor.value = true
}

function confirmModalEditor() {
  if (modalEditorCallback) modalEditorCallback(modalEditorValue.value)
  showModalEditor.value = false
}

function openToolsModal() {
  toolsSearchText.value = ''
  showToolsModal.value = true
}

// Data Mapping Utilities
function configValue(config: Record<string, unknown>, key: string): string {
  const val = config?.[key]; return typeof val === 'string' ? val : ''
}
function configArray(config: Record<string, unknown>, key: string): string[] {
  const val = config?.[key]; return Array.isArray(val) ? val.map(String) : []
}
function configMap(config: Record<string, unknown>, key: string): Record<string, string> {
  const val = config?.[key]
  if (val && typeof val === 'object' && !Array.isArray(val)) {
    const out: Record<string, string> = {}
    for (const [k, v] of Object.entries(val)) out[k] = String(v)
    return out
  }
  return {}
}
function recordToPairs(record: Record<string, string>): KeyValuePair[] {
  return Object.entries(record).map(([key, value]) => ({ key, value }))
}
function pairsToRecord(pairs: KeyValuePair[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const p of pairs) if (p.key.trim()) out[p.key.trim()] = p.value
  return out
}

// Navigation & Actions
function attemptSelectItem(item: McpItem) {
  if (selectedItem.value && isItemDirty(selectedItem.value) && selectedItem.value.id !== item.id) {
     interceptTargetItem = item
     showInterceptDialog.value = true
  } else {
     selectItem(item)
  }
}

function confirmIntercept() {
  showInterceptDialog.value = false
  if (isCreatingNewAfterDiscard) {
    isCreatingNewAfterDiscard = false
    createNewDraftInternal()
  } else if (interceptTargetItem) {
    selectItem(interceptTargetItem)
  }
}

function selectItem(item: McpItem) {
  selectedItem.value = item
  probeAuthRequired.value = false
  oauthStatus.value = null
  showAdvanced.value = false
  tier1Error.value = false
  tier2Error.value = false
  saveState.value = 'idle'
  if (probeAbortController) {
    probeAbortController.abort()
    probeAbortController = null
  }
  
  if (item.id && item.type !== 'stdio') loadOAuthStatus(item)
  
  const cfg = item.config ?? {}
  connectionType.value = item.type === 'stdio' ? 'stdio' : 'remote'
  formData.value = {
    name: item.name,
    command: configValue(cfg, 'command'),
    url: configValue(cfg, 'url'),
    cwd: configValue(cfg, 'cwd'),
    transport: item.type === 'sse' ? 'sse' : 'http',
    active: !!item.is_active,
  }
  argsTags.value = configArray(cfg, 'args')
  envPairs.value = recordToPairs(configMap(cfg, 'env'))
  headerPairs.value = recordToPairs(configMap(cfg, 'headers'))
}

function removeDraft() {
  const draft = items.value.find(i => i.id === DRAFT_ID)
  if (draft) {
    items.value = items.value.filter(i => i.id !== DRAFT_ID)
    if (selectedItem.value?.id === DRAFT_ID) {
      selectedItem.value = null
    }
  }
}

function createNewDraftInternal() {
  removeDraft()
  const draft: McpItem = {
    id: DRAFT_ID, name: '', type: 'stdio',
    config: {}, is_active: true, status: 'unknown', tools_cache: [],
    last_probed_at: null, status_message: '', auth_type: 'none'
  }
  items.value = [draft, ...items.value]
  selectItem(draft)
}

function handleAddNewDraft() {
  const existingDraft = items.value.find(i => i.id === DRAFT_ID)
  if (existingDraft && isItemDirty(existingDraft)) {
    isCreatingNewAfterDiscard = true
    interceptTargetItem = null
    showInterceptDialog.value = true
    return
  }

  if (existingDraft) {
    selectItem(existingDraft)
    // Reset and Trigger blink micro-interaction
    isBlinking.value = false
    nextTick(() => {
      blinkKey.value++
      isBlinking.value = true
      
      // Auto reset after animation duration (400ms)
      setTimeout(() => {
        isBlinking.value = false
      }, 400)
    })
    
    // Scroll to top to ensure draft is visible
    const viewport = sidebarScrollRef.value?.$el?.querySelector('[data-radix-scroll-area-viewport]')
    if (viewport) {
      viewport.scrollTo({ top: 0, behavior: 'smooth' })
    }
  } else {
    createNewDraftInternal()
  }
}

function startImport() {
  importJson.value = '{\n  "mcpServers": {\n    \n  }\n}'
  importError.value = ''
  importDialogOpen.value = true
}

function formatImportJson() {
  try {
     const parsed = JSON.parse(importJson.value)
     importJson.value = JSON.stringify(parsed, null, 2)
     importError.value = ''
  } catch {
     importError.value = t('mcp.importErrorJson')
  }
}

// Backend Interactions
function buildRequestBody(): McpUpsertRequest {
  const body: McpUpsertRequest = { name: formData.value.name.trim() || t('mcp.unnamedServer'), is_active: formData.value.active }
  if (connectionType.value === 'stdio') {
    body.command = formData.value.command.trim()
    if (argsTags.value.length > 0) body.args = argsTags.value
    const envRecord = pairsToRecord(envPairs.value)
    if (Object.keys(envRecord).length > 0) body.env = envRecord
    if (formData.value.cwd.trim()) body.cwd = formData.value.cwd.trim()
  } else {
    body.url = formData.value.url.trim()
    const headerRecord = pairsToRecord(headerPairs.value)
    if (Object.keys(headerRecord).length > 0) body.headers = headerRecord
    if (formData.value.transport === 'sse') body.transport = 'sse'
  }
  return body
}

async function loadList() {
  loading.value = true
  try {
    const { data } = await getBotsByBotIdMcp({ path: { bot_id: props.botId } as unknown as { bot_id: string }, throwOnError: true })
    const serverItems: McpItem[] = (data.items ?? []).map((item: Record<string, unknown>) => ({
      ...item as unknown as McpItem, status: (item.status as string) ?? 'unknown', tools_cache: (item.tools_cache as McpToolDescriptor[]) ?? [],
      last_probed_at: (item.last_probed_at as string) ?? null, status_message: (item.status_message as string) ?? '', auth_type: (item.auth_type as string) ?? 'none'
    }))
    const draft = items.value.find((i) => i.id === DRAFT_ID)
    items.value = draft ? [draft, ...serverItems] : serverItems

    if (selectedItem.value && selectedItem.value.id !== DRAFT_ID) {
      const still = serverItems.find((i) => i.id === selectedItem.value!.id)
      if (still) selectItem(still)
      else selectedItem.value = null
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('mcp.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  if (!selectedItem.value) return
  saveState.value = 'syncing'
  tier1Error.value = false
  tier2Error.value = false
  
  try {
    const body = buildRequestBody()
    let savedId: string | undefined
    if (isDraft.value) {
      const { data } = await postBotsByBotIdMcp({ path: { bot_id: props.botId } as unknown as { bot_id: string }, body, throwOnError: true })
      savedId = data?.id
      removeDraft()
      toast.success(t('mcp.createSuccess'))
      await loadList()
      const created = items.value.find(i => i.id === savedId) || items.value.find(i => i.name === body.name)
      if (created) selectItem(created)
    } else {
      savedId = selectedItem.value.id
      await putBotsByBotIdMcpById({ path: { bot_id: props.botId, id: selectedItem.value.id } as unknown as { bot_id: string, id: string }, body, throwOnError: true })
      toast.success(t('mcp.updateSuccess'))
      await loadList()
    }
    
    // Auto Probe
    if (savedId && selectedItem.value) {
      handleProbe(selectedItem.value)
    } else {
      saveState.value = 'idle'
    }
  } catch (error) {
    tier1Error.value = true
    tier1ErrorMessage.value = resolveApiErrorMessage(error, t('mcp.invalidConfig'))
    saveState.value = 'error'
    setTimeout(() => { if (saveState.value === 'error') saveState.value = 'idle' }, 3000)
  }
}

function handleProbeInterruption() {
  if (saveState.value === 'verifying' && probeAbortController) {
    probeAbortController.abort()
    probeAbortController = null
    saveState.value = 'idle'
    toast.info(t('mcp.probeAborted'))
  } else if (selectedItem.value) {
    if (isDraft.value) {
      handleSave()
    } else if (selectedItem.value.id) {
      handleProbe(selectedItem.value)
    }
  }
}

async function handleProbe(item: McpItem) {
  if (!item.id) return
  saveState.value = 'verifying'
  probeAuthRequired.value = false
  tier2Error.value = false
  
  if (probeAbortController) probeAbortController.abort()
  probeAbortController = new AbortController()
  
  try {
    const { data } = await postBotsByBotIdMcpByIdProbe({ path: { bot_id: props.botId, id: item.id } as unknown as { bot_id: string, id: string }, throwOnError: true })
    if (data) {
      item.status = data.status ?? item.status
      item.tools_cache = data.tools ?? []
      item.status_message = data.error ?? ''
      item.last_probed_at = new Date().toISOString()
      probeAuthRequired.value = !!data.auth_required
      
      if (data.status === 'connected') {
        saveState.value = 'connected'
        setTimeout(() => { if (saveState.value === 'connected') saveState.value = 'idle' }, 2000)
      } else {
        tier2Error.value = true
        saveState.value = 'error'
        setTimeout(() => { if (saveState.value === 'error') saveState.value = 'idle' }, 3000)
      }
    }
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') return // Interrupted by user
    tier2Error.value = true
    item.status_message = resolveApiErrorMessage(error, t('mcp.probeFailedNetwork'))
    saveState.value = 'error'
    setTimeout(() => { if (saveState.value === 'error') saveState.value = 'idle' }, 3000)
  } finally {
    probeAbortController = null
  }
}

async function executeImport() {
  importSubmitting.value = true
  importError.value = ''
  try {
    let parsed: McpImportRequest = JSON.parse(importJson.value)
    if (!parsed.mcpServers && typeof parsed === 'object') {
      parsed = { mcpServers: parsed as McpImportRequest['mcpServers'] }
    }
    await putBotsByBotIdMcpImport({ path: { bot_id: props.botId } as unknown as { bot_id: string }, body: parsed, throwOnError: true })
    importDialogOpen.value = false
    importJson.value = ''
    await loadList()
    toast.success(t('mcp.importSuccess'))
  } catch (error) {
     if (error instanceof SyntaxError) {
        importError.value = t('mcp.importErrorJson')
     } else {
        importError.value = resolveApiErrorMessage(error, t('mcp.importErrorFormat'))
     }
  } finally {
    importSubmitting.value = false
  }
}

async function handleDelete(item: McpItem) {
  try {
    await deleteBotsByBotIdMcpById({ path: { bot_id: props.botId, id: item.id } as unknown as { bot_id: string, id: string }, throwOnError: true })
    selectedItem.value = null
    await loadList()
    toast.success(t('mcp.deleteSuccess'))
  } catch {
    toast.error(t('mcp.deleteFailed'))
  }
}

function handleExportSingle() {
  if (!selectedItem.value || !selectedItem.value.id) return
  const mcpServers: Record<string, McpMcpServerEntry> = {
    [selectedItem.value.name]: {
      command: selectedItem.value.type === 'stdio' ? configValue(selectedItem.value.config, 'command') || undefined : undefined,
      args: selectedItem.value.type === 'stdio' && configArray(selectedItem.value.config, 'args').length ? configArray(selectedItem.value.config, 'args') : undefined,
      env: selectedItem.value.type === 'stdio' && Object.keys(configMap(selectedItem.value.config, 'env')).length ? configMap(selectedItem.value.config, 'env') : undefined,
      cwd: selectedItem.value.type === 'stdio' ? configValue(selectedItem.value.config, 'cwd') || undefined : undefined,
      url: selectedItem.value.type !== 'stdio' ? configValue(selectedItem.value.config, 'url') || undefined : undefined,
      headers: selectedItem.value.type !== 'stdio' && Object.keys(configMap(selectedItem.value.config, 'headers')).length ? configMap(selectedItem.value.config, 'headers') : undefined,
      transport: selectedItem.value.type === 'sse' ? 'sse' : undefined,
    }
  }
  copyText(JSON.stringify({ mcpServers }, null, 2))
  toast.success(t('mcp.copySuccess'))
}

// OAuth Handlers
async function loadOAuthStatus(item: McpItem) {
  if (!item.id || item.type === 'stdio') { oauthStatus.value = null; return }
  try {
    const { data } = await getBotsByBotIdMcpByIdOauthStatus({ path: { bot_id: props.botId, id: item.id } as unknown as { bot_id: string, id: string }, throwOnError: true })
    oauthStatus.value = data ?? null
    oauthCallbackUrl.value = `${window.location.origin}/oauth/mcp/callback`
  } catch { oauthStatus.value = null }
}

async function handleOAuthDiscover() {
  if (!selectedItem.value?.id) return false
  oauthDiscovering.value = true
  oauthNeedsClientId.value = false
  try {
    const { data } = await postBotsByBotIdMcpByIdOauthDiscover({ path: { bot_id: props.botId, id: selectedItem.value.id } as unknown as { bot_id: string, id: string }, throwOnError: true })
    if (!data?.registration_endpoint) oauthNeedsClientId.value = true
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('mcp.oauth.discoverFailed')))
    oauthDiscovering.value = false
    return false
  }
  oauthDiscovering.value = false
  return true
}

async function handleOAuthFlow() {
  if (!selectedItem.value?.id) return
  if (!oauthDiscovered.value) {
    const discovered = await handleOAuthDiscover()
    if (!discovered) return
    oauthDiscovered.value = true
    if (oauthNeedsClientId.value && !oauthClientId.value.trim()) return
  }

  oauthAuthorizing.value = true
  try {
    const { data } = await postBotsByBotIdMcpByIdOauthAuthorize({
      path: { bot_id: props.botId, id: selectedItem.value.id } as unknown as { bot_id: string, id: string },
      body: {
        client_id: oauthClientId.value.trim() || undefined,
        client_secret: oauthClientSecret.value.trim() || undefined,
        callback_url: `${window.location.origin}/oauth/mcp/callback`,
      },
      throwOnError: true,
    })
    if (!data?.authorization_url) throw new Error('No authorization URL returned')
    
    const popup = window.open(data.authorization_url, 'mcp-oauth', 'width=600,height=700')
    const onMessage = async (event: MessageEvent) => {
      if (event.data?.type === 'mcp-oauth-callback') {
        window.removeEventListener('message', onMessage)
        oauthAuthorizing.value = false
        if (event.data.status === 'success') {
          toast.success(t('mcp.oauth.authSuccess'))
          await loadOAuthStatus(selectedItem.value!)
          handleProbe(selectedItem.value!)
        } else {
          toast.error(event.data.error || t('mcp.oauth.authFailed'))
        }
      }
    }
    window.addEventListener('message', onMessage)

    const pollTimer = setInterval(() => {
      if (popup && popup.closed) {
        clearInterval(pollTimer)
        window.removeEventListener('message', onMessage)
        oauthAuthorizing.value = false
        loadOAuthStatus(selectedItem.value!)
      }
    }, 500)
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('mcp.oauth.flowInitFailed')))
    oauthAuthorizing.value = false
  }
}

async function handleOAuthRevoke() {
  if (!selectedItem.value?.id) return
  try {
    await deleteBotsByBotIdMcpByIdOauthToken({ path: { bot_id: props.botId, id: selectedItem.value.id } as unknown as { bot_id: string, id: string }, throwOnError: true })
    toast.success(t('mcp.oauth.revokeSuccess'))
    oauthDiscovered.value = false
    oauthNeedsClientId.value = false
    oauthClientId.value = ''
    oauthClientSecret.value = ''
    await loadOAuthStatus(selectedItem.value)
  } catch {
    toast.error(t('mcp.oauth.revokeFailed'))
  }
}

const { consumePendingDraft } = useSupermarketMcpDraft()
function applyPendingDraft() {
  const entry = consumePendingDraft()
  if (!entry) return
  removeDraft()
  const isStdio = entry.transport === 'stdio'
  const draft: McpItem = {
    id: DRAFT_ID, name: entry.name ?? '', type: isStdio ? 'stdio' : (entry.transport === 'sse' ? 'sse' : 'http'),
    config: {}, is_active: true, status: 'unknown', tools_cache: [], last_probed_at: null, status_message: '', auth_type: 'none'
  }
  items.value = [draft, ...items.value]
  selectItem(draft)
}

watch(() => props.botId, async () => { if (props.botId) { await loadList(); applyPendingDraft() } }, { immediate: true })

watch(
  () => {
    const entries = queryCache.getEntries({ key: ['bot-mcp', props.botId] })
    return entries[0]?.state.value.data
  },
  (next, prev) => {
    if (!props.botId) return
    if (next === prev) return
    void loadList()
  },
)

watch(selectedItem, (item) => {
  const next = item?.id && item.id !== DRAFT_ID ? item.id : ''
  if (selectedMcpId.value !== next) selectedMcpId.value = next
})

watch(selectedMcpId, (id) => {
  if (!id || selectedItem.value?.id === id) return
  const target = items.value.find((i) => i.id === id)
  if (target) selectItem(target)
})

</script>
