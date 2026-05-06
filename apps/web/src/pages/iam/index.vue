<template>
  <SettingsShell width="wide">
    <div class="space-y-5">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h1 class="text-lg font-semibold">
            {{ $t('iam.title') }}
          </h1>
          <p class="mt-1 text-xs text-muted-foreground">
            {{ $t('iam.subtitle') }}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          :disabled="loading"
          @click="loadAll"
        >
          <RefreshCw class="size-3.5" />
          {{ $t('common.refresh') }}
        </Button>
      </div>

      <div
        v-if="loading"
        class="flex items-center text-xs text-muted-foreground"
      >
        <Spinner class="mr-2 size-4" />
        {{ $t('common.loading') }}
      </div>

      <Tabs
        default-value="groups"
        class="w-full"
      >
        <TabsList>
          <TabsTrigger value="groups">
            {{ $t('iam.groups') }}
          </TabsTrigger>
          <TabsTrigger value="sso">
            {{ $t('iam.sso') }}
          </TabsTrigger>
          <TabsTrigger value="botPermissions">
            {{ $t('iam.botPermissions') }}
          </TabsTrigger>
          <TabsTrigger value="roles">
            {{ $t('iam.roles') }}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="groups">
          <section class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_340px]">
            <div class="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{{ $t('iam.key') }}</TableHead>
                    <TableHead>{{ $t('iam.displayName') }}</TableHead>
                    <TableHead>{{ $t('iam.source') }}</TableHead>
                    <TableHead class="text-right">
                      {{ $t('iam.actions') }}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow v-if="groups.length === 0">
                    <TableCell
                      colspan="4"
                      class="text-center text-muted-foreground"
                    >
                      {{ $t('iam.empty') }}
                    </TableCell>
                  </TableRow>
                  <TableRow
                    v-for="group in groups"
                    v-else
                    :key="group.id"
                    :class="selectedGroupId === group.id ? 'bg-accent/40' : ''"
                  >
                    <TableCell class="font-medium">
                      {{ group.key }}
                    </TableCell>
                    <TableCell>{{ group.display_name }}</TableCell>
                    <TableCell>{{ group.source }}</TableCell>
                    <TableCell>
                      <div class="flex justify-end gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          @click="selectGroup(group)"
                        >
                          {{ $t('iam.members') }}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          @click="editGroup(group)"
                        >
                          {{ $t('common.edit') }}
                        </Button>
                        <ConfirmPopover
                          :message="$t('iam.deleteGroupConfirm')"
                          @confirm="deleteGroup(group.id || '')"
                        >
                          <template #trigger>
                            <Button
                              variant="destructive"
                              size="sm"
                            >
                              {{ $t('common.delete') }}
                            </Button>
                          </template>
                        </ConfirmPopover>
                      </div>
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </div>

            <div class="space-y-4">
              <section class="rounded-md border p-4 space-y-3">
                <h2 class="text-sm font-medium">
                  {{ groupForm.id ? $t('iam.editGroup') : $t('iam.createGroup') }}
                </h2>
                <div class="space-y-2">
                  <Label>{{ $t('iam.key') }}</Label>
                  <Input v-model="groupForm.key" />
                </div>
                <div class="space-y-2">
                  <Label>{{ $t('iam.displayName') }}</Label>
                  <Input v-model="groupForm.display_name" />
                </div>
                <div class="space-y-2">
                  <Label>{{ $t('iam.source') }}</Label>
                  <Input v-model="groupForm.source" />
                </div>
                <div class="space-y-2">
                  <Label>{{ $t('iam.externalId') }}</Label>
                  <Input v-model="groupForm.external_id" />
                </div>
                <div class="space-y-2">
                  <Label>{{ $t('iam.metadata') }}</Label>
                  <Textarea
                    v-model="groupForm.metadata"
                    class="min-h-24 font-mono text-xs"
                  />
                </div>
                <div class="flex gap-2">
                  <Button
                    size="sm"
                    :disabled="savingGroup"
                    @click="saveGroup"
                  >
                    <Spinner
                      v-if="savingGroup"
                      class="size-3.5"
                    />
                    {{ $t('common.save') }}
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    @click="resetGroupForm"
                  >
                    {{ $t('common.cancel') }}
                  </Button>
                </div>
              </section>

              <section class="rounded-md border p-4 space-y-3">
                <h2 class="text-sm font-medium">
                  {{ $t('iam.groupMembers') }}
                </h2>
                <select
                  v-model="selectedGroupId"
                  class="h-9 w-full rounded-md border border-border bg-background px-3 text-xs"
                  @change="loadGroupMembers"
                >
                  <option value="">
                    {{ $t('iam.selectGroup') }}
                  </option>
                  <option
                    v-for="group in groups"
                    :key="group.id"
                    :value="group.id"
                  >
                    {{ group.display_name || group.key }}
                  </option>
                </select>
                <div class="flex gap-2">
                  <select
                    v-model="memberUserId"
                    class="h-9 min-w-0 flex-1 rounded-md border border-border bg-background px-3 text-xs"
                  >
                    <option value="">
                      {{ $t('iam.selectUser') }}
                    </option>
                    <option
                      v-for="user in users"
                      :key="user.id"
                      :value="user.id"
                    >
                      {{ accountLabel(user) }}
                    </option>
                  </select>
                  <Button
                    size="sm"
                    :disabled="!selectedGroupId || !memberUserId"
                    @click="addGroupMember"
                  >
                    {{ $t('common.add') }}
                  </Button>
                </div>
                <div class="space-y-2">
                  <div
                    v-for="member in groupMembers"
                    :key="member.user_id"
                    class="flex items-center justify-between gap-2 rounded-md border px-3 py-2"
                  >
                    <div class="min-w-0">
                      <p class="truncate text-xs font-medium">
                        {{ member.display_name || member.username || member.email || member.user_id }}
                      </p>
                      <p class="truncate text-[11px] text-muted-foreground">
                        {{ member.email || member.user_id }}
                      </p>
                    </div>
                    <Button
                      variant="ghost"
                      size="sm"
                      @click="removeGroupMember(member.user_id || '')"
                    >
                      <Trash2 class="size-3.5" />
                    </Button>
                  </div>
                </div>
              </section>
            </div>
          </section>
        </TabsContent>

        <TabsContent value="sso">
          <section class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_420px]">
            <div class="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{{ $t('iam.name') }}</TableHead>
                    <TableHead>{{ $t('iam.type') }}</TableHead>
                    <TableHead>{{ $t('iam.key') }}</TableHead>
                    <TableHead>{{ $t('iam.enabled') }}</TableHead>
                    <TableHead class="text-right">
                      {{ $t('iam.actions') }}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow v-if="providers.length === 0">
                    <TableCell
                      colspan="5"
                      class="text-center text-muted-foreground"
                    >
                      {{ $t('iam.empty') }}
                    </TableCell>
                  </TableRow>
                  <TableRow
                    v-for="provider in providers"
                    v-else
                    :key="provider.id"
                  >
                    <TableCell class="font-medium">
                      {{ provider.name }}
                    </TableCell>
                    <TableCell>{{ provider.type }}</TableCell>
                    <TableCell>{{ provider.key }}</TableCell>
                    <TableCell>{{ provider.enabled ? $t('common.enabled') : $t('common.disabled') }}</TableCell>
                    <TableCell>
                      <div class="flex justify-end gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          @click="selectProvider(provider)"
                        >
                          {{ $t('iam.mappings') }}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          @click="editProvider(provider)"
                        >
                          {{ $t('common.edit') }}
                        </Button>
                        <ConfirmPopover
                          :message="$t('iam.deleteProviderConfirm')"
                          @confirm="deleteProvider(provider.id || '')"
                        >
                          <template #trigger>
                            <Button
                              variant="destructive"
                              size="sm"
                            >
                              {{ $t('common.delete') }}
                            </Button>
                          </template>
                        </ConfirmPopover>
                      </div>
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </div>

            <div class="space-y-4">
              <section class="rounded-md border p-4 space-y-3">
                <h2 class="text-sm font-medium">
                  {{ providerForm.id ? $t('iam.editProvider') : $t('iam.createProvider') }}
                </h2>
                <div class="grid grid-cols-2 gap-3">
                  <div class="space-y-2">
                    <Label>{{ $t('iam.type') }}</Label>
                    <select
                      v-model="providerForm.type"
                      class="h-9 w-full rounded-md border border-border bg-background px-3 text-xs"
                      @change="resetProviderConfig"
                    >
                      <option value="oidc">
                        OIDC
                      </option>
                      <option value="saml">
                        SAML
                      </option>
                    </select>
                  </div>
                  <div class="space-y-2">
                    <Label>{{ $t('iam.key') }}</Label>
                    <Input v-model="providerForm.key" />
                  </div>
                </div>
                <div class="space-y-2">
                  <Label>{{ $t('iam.name') }}</Label>
                  <Input v-model="providerForm.name" />
                </div>
                <div class="grid grid-cols-3 gap-3">
                  <label class="flex items-center gap-2 text-xs">
                    <Switch v-model:checked="providerForm.enabled" />
                    {{ $t('iam.enabled') }}
                  </label>
                  <label class="flex items-center gap-2 text-xs">
                    <Switch v-model:checked="providerForm.jit_enabled" />
                    JIT
                  </label>
                  <label class="flex items-center gap-2 text-xs">
                    <Switch v-model:checked="providerForm.trust_email" />
                    {{ $t('iam.trustEmail') }}
                  </label>
                </div>
                <div class="space-y-2">
                  <Label>{{ $t('iam.emailLinkingPolicy') }}</Label>
                  <select
                    v-model="providerForm.email_linking_policy"
                    class="h-9 w-full rounded-md border border-border bg-background px-3 text-xs"
                  >
                    <option value="link_existing">
                      link_existing
                    </option>
                    <option value="reject_existing">
                      reject_existing
                    </option>
                  </select>
                </div>
                <div class="space-y-2">
                  <Label>{{ $t('iam.configJson') }}</Label>
                  <Textarea
                    v-model="providerForm.config"
                    class="min-h-40 font-mono text-xs"
                  />
                </div>
                <div class="space-y-2">
                  <Label>{{ $t('iam.attributeMappingJson') }}</Label>
                  <Textarea
                    v-model="providerForm.attribute_mapping"
                    class="min-h-32 font-mono text-xs"
                  />
                </div>
                <div class="flex gap-2">
                  <Button
                    size="sm"
                    :disabled="savingProvider"
                    @click="saveProvider"
                  >
                    <Spinner
                      v-if="savingProvider"
                      class="size-3.5"
                    />
                    {{ $t('common.save') }}
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    @click="resetProviderForm"
                  >
                    {{ $t('common.cancel') }}
                  </Button>
                </div>
              </section>

              <section class="rounded-md border p-4 space-y-3">
                <h2 class="text-sm font-medium">
                  {{ $t('iam.groupMappings') }}
                </h2>
                <select
                  v-model="selectedProviderId"
                  class="h-9 w-full rounded-md border border-border bg-background px-3 text-xs"
                  @change="loadMappings"
                >
                  <option value="">
                    {{ $t('iam.selectProvider') }}
                  </option>
                  <option
                    v-for="provider in providers"
                    :key="provider.id"
                    :value="provider.id"
                  >
                    {{ provider.name || provider.key }}
                  </option>
                </select>
                <div class="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-2">
                  <Input
                    v-model="mappingExternalGroup"
                    :placeholder="$t('iam.externalGroup')"
                  />
                  <select
                    v-model="mappingGroupId"
                    class="h-9 rounded-md border border-border bg-background px-3 text-xs"
                  >
                    <option value="">
                      {{ $t('iam.selectGroup') }}
                    </option>
                    <option
                      v-for="group in groups"
                      :key="group.id"
                      :value="group.id"
                    >
                      {{ group.display_name || group.key }}
                    </option>
                  </select>
                  <Button
                    size="sm"
                    :disabled="!selectedProviderId || !mappingExternalGroup || !mappingGroupId"
                    @click="saveMapping"
                  >
                    {{ $t('common.add') }}
                  </Button>
                </div>
                <div class="space-y-2">
                  <div
                    v-for="mapping in mappings"
                    :key="mapping.external_group"
                    class="flex items-center justify-between gap-2 rounded-md border px-3 py-2"
                  >
                    <div class="min-w-0">
                      <p class="truncate text-xs font-medium">
                        {{ mapping.external_group }}
                      </p>
                      <p class="truncate text-[11px] text-muted-foreground">
                        {{ mapping.group_display_name || mapping.group_key }}
                      </p>
                    </div>
                    <Button
                      variant="ghost"
                      size="sm"
                      @click="deleteMapping(mapping.external_group || '')"
                    >
                      <Trash2 class="size-3.5" />
                    </Button>
                  </div>
                </div>
              </section>
            </div>
          </section>
        </TabsContent>

        <TabsContent value="botPermissions">
          <section class="grid gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
            <div class="rounded-md border p-4 space-y-3">
              <h2 class="text-sm font-medium">
                {{ $t('iam.assignBotRole') }}
              </h2>
              <div class="space-y-2">
                <Label>{{ $t('iam.bot') }}</Label>
                <select
                  v-model="selectedBotId"
                  class="h-9 w-full rounded-md border border-border bg-background px-3 text-xs"
                  @change="loadPrincipalRoles"
                >
                  <option value="">
                    {{ $t('iam.selectBot') }}
                  </option>
                  <option
                    v-for="bot in bots"
                    :key="bot.id"
                    :value="bot.id"
                  >
                    {{ bot.display_name || bot.id }}
                  </option>
                </select>
              </div>
              <div class="space-y-2">
                <Label>{{ $t('iam.principalType') }}</Label>
                <select
                  v-model="assignmentPrincipalType"
                  class="h-9 w-full rounded-md border border-border bg-background px-3 text-xs"
                >
                  <option value="user">
                    user
                  </option>
                  <option value="group">
                    group
                  </option>
                </select>
              </div>
              <div class="space-y-2">
                <Label>{{ $t('iam.principal') }}</Label>
                <select
                  v-model="assignmentPrincipalId"
                  class="h-9 w-full rounded-md border border-border bg-background px-3 text-xs"
                >
                  <option value="">
                    {{ $t('iam.selectPrincipal') }}
                  </option>
                  <option
                    v-for="principal in principalOptions"
                    :key="principal.id"
                    :value="principal.id"
                  >
                    {{ principal.label }}
                  </option>
                </select>
              </div>
              <div class="space-y-2">
                <Label>{{ $t('iam.role') }}</Label>
                <select
                  v-model="assignmentRoleKey"
                  class="h-9 w-full rounded-md border border-border bg-background px-3 text-xs"
                >
                  <option
                    v-for="role in botRoles"
                    :key="role.key"
                    :value="role.key"
                  >
                    {{ role.key }}
                  </option>
                </select>
              </div>
              <Button
                size="sm"
                :disabled="!selectedBotId || !assignmentPrincipalId || !assignmentRoleKey"
                @click="assignBotRole"
              >
                {{ $t('iam.assign') }}
              </Button>
            </div>

            <div class="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{{ $t('iam.principal') }}</TableHead>
                    <TableHead>{{ $t('iam.role') }}</TableHead>
                    <TableHead>{{ $t('iam.source') }}</TableHead>
                    <TableHead class="text-right">
                      {{ $t('iam.actions') }}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow v-if="principalRoles.length === 0">
                    <TableCell
                      colspan="4"
                      class="text-center text-muted-foreground"
                    >
                      {{ $t('iam.empty') }}
                    </TableCell>
                  </TableRow>
                  <TableRow
                    v-for="assignment in principalRoles"
                    v-else
                    :key="assignment.id"
                  >
                    <TableCell>
                      {{ principalRoleLabel(assignment) }}
                    </TableCell>
                    <TableCell>{{ assignment.role_key }}</TableCell>
                    <TableCell>{{ assignment.source }}</TableCell>
                    <TableCell class="text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        @click="deletePrincipalRole(assignment.id || '')"
                      >
                        <Trash2 class="size-3.5" />
                      </Button>
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </div>
          </section>
        </TabsContent>

        <TabsContent value="roles">
          <div class="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{{ $t('iam.key') }}</TableHead>
                  <TableHead>{{ $t('iam.scope') }}</TableHead>
                  <TableHead>{{ $t('iam.system') }}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow
                  v-for="role in roles"
                  :key="role.id"
                >
                  <TableCell class="font-medium">
                    {{ role.key }}
                  </TableCell>
                  <TableCell>{{ role.scope }}</TableCell>
                  <TableCell>{{ role.is_system ? 'true' : 'false' }}</TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  </SettingsShell>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { RefreshCw, Trash2 } from 'lucide-vue-next'
import { toast } from 'vue-sonner'
import {
  Button,
  Input,
  Label,
  Spinner,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  Textarea,
} from '@memohai/ui'
import {
  deleteIamGroupsById,
  deleteIamGroupsByIdMembersByUserId,
  deleteIamPrincipalRolesById,
  deleteIamSsoProvidersById,
  deleteIamSsoProvidersByIdGroupMappingsByExternalGroup,
  getBots,
  getIamBotsByBotIdPrincipalRoles,
  getIamGroups,
  getIamGroupsByIdMembers,
  getIamRoles,
  getIamSsoProviders,
  getIamSsoProvidersByIdGroupMappings,
  getUsers,
  postIamBotsByBotIdPrincipalRoles,
  postIamGroups,
  postIamGroupsByIdMembers,
  postIamSsoProviders,
  postIamSsoProvidersByIdGroupMappings,
  putIamGroupsById,
  putIamSsoProvidersById,
  type BotsBot,
  type GithubComMemohaiMemohInternalAccountsAccount,
  type HandlersIamGroupMemberResponse,
  type HandlersIamGroupResponse,
  type HandlersIamPrincipalRoleResponse,
  type HandlersIamRoleResponse,
  type HandlersIamssoGroupMappingResponse,
  type HandlersIamssoProviderResponse,
} from '@memohai/sdk'
import SettingsShell from '@/components/settings-shell/index.vue'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'

const loading = ref(false)
const savingGroup = ref(false)
const savingProvider = ref(false)

const users = ref<GithubComMemohaiMemohInternalAccountsAccount[]>([])
const bots = ref<BotsBot[]>([])
const roles = ref<HandlersIamRoleResponse[]>([])
const groups = ref<HandlersIamGroupResponse[]>([])
const providers = ref<HandlersIamssoProviderResponse[]>([])
const groupMembers = ref<HandlersIamGroupMemberResponse[]>([])
const mappings = ref<HandlersIamssoGroupMappingResponse[]>([])
const principalRoles = ref<HandlersIamPrincipalRoleResponse[]>([])

const selectedGroupId = ref('')
const memberUserId = ref('')
const selectedProviderId = ref('')
const mappingExternalGroup = ref('')
const mappingGroupId = ref('')
const selectedBotId = ref('')
const assignmentPrincipalType = ref<'user' | 'group'>('user')
const assignmentPrincipalId = ref('')
const assignmentRoleKey = ref('bot_operator')

const groupForm = reactive({
  id: '',
  key: '',
  display_name: '',
  source: 'manual',
  external_id: '',
  metadata: '{}',
})

const providerForm = reactive({
  id: '',
  type: 'oidc',
  key: '',
  name: '',
  enabled: true,
  config: oidcConfigJSON(),
  attribute_mapping: attributeMappingJSON(),
  jit_enabled: true,
  email_linking_policy: 'link_existing',
  trust_email: true,
})

const botRoles = computed(() => roles.value.filter(role => role.scope === 'bot'))

const principalOptions = computed(() => {
  if (assignmentPrincipalType.value === 'user') {
    return users.value.map(user => ({ id: user.id || '', label: accountLabel(user) })).filter(item => item.id)
  }
  return groups.value.map(group => ({ id: group.id || '', label: group.display_name || group.key || group.id || '' })).filter(item => item.id)
})

onMounted(loadAll)

async function loadAll() {
  loading.value = true
  try {
    const [userResult, botResult, roleResult, groupResult, providerResult] = await Promise.all([
      getUsers({ throwOnError: true }),
      getBots({ throwOnError: true }),
      getIamRoles({ throwOnError: true }),
      getIamGroups({ throwOnError: true }),
      getIamSsoProviders({ throwOnError: true }),
    ])
    users.value = userResult.data.items || []
    bots.value = botResult.data.items || []
    roles.value = roleResult.data.items || []
    groups.value = groupResult.data.items || []
    providers.value = providerResult.data.items || []
    if (!assignmentRoleKey.value && botRoles.value[0]?.key) {
      assignmentRoleKey.value = botRoles.value[0].key
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, 'Failed to load IAM data'))
  } finally {
    loading.value = false
  }
}

async function loadGroups() {
  const { data } = await getIamGroups({ throwOnError: true })
  groups.value = data.items || []
}

async function loadProviders() {
  const { data } = await getIamSsoProviders({ throwOnError: true })
  providers.value = data.items || []
}

async function loadGroupMembers() {
  if (!selectedGroupId.value) {
    groupMembers.value = []
    return
  }
  const { data } = await getIamGroupsByIdMembers({ path: { id: selectedGroupId.value }, throwOnError: true })
  groupMembers.value = data.items || []
}

async function loadMappings() {
  if (!selectedProviderId.value) {
    mappings.value = []
    return
  }
  const { data } = await getIamSsoProvidersByIdGroupMappings({ path: { id: selectedProviderId.value }, throwOnError: true })
  mappings.value = data.items || []
}

async function loadPrincipalRoles() {
  if (!selectedBotId.value) {
    principalRoles.value = []
    return
  }
  const { data } = await getIamBotsByBotIdPrincipalRoles({ path: { bot_id: selectedBotId.value }, throwOnError: true })
  principalRoles.value = data.items || []
}

function resetGroupForm() {
  groupForm.id = ''
  groupForm.key = ''
  groupForm.display_name = ''
  groupForm.source = 'manual'
  groupForm.external_id = ''
  groupForm.metadata = '{}'
}

function editGroup(group: HandlersIamGroupResponse) {
  groupForm.id = group.id || ''
  groupForm.key = group.key || ''
  groupForm.display_name = group.display_name || ''
  groupForm.source = group.source || 'manual'
  groupForm.external_id = group.external_id || ''
  groupForm.metadata = stringifyJSON(group.metadata)
}

async function saveGroup() {
  savingGroup.value = true
  try {
    const body = {
      key: groupForm.key.trim(),
      display_name: groupForm.display_name.trim(),
      source: groupForm.source.trim() || 'manual',
      external_id: groupForm.external_id.trim(),
      metadata: parseJSON(groupForm.metadata, 'metadata'),
    }
    if (groupForm.id) {
      await putIamGroupsById({ path: { id: groupForm.id }, body: body as never, throwOnError: true })
    } else {
      await postIamGroups({ body: body as never, throwOnError: true })
    }
    await loadGroups()
    resetGroupForm()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, 'Failed to save group'))
  } finally {
    savingGroup.value = false
  }
}

async function deleteGroup(id: string) {
  if (!id) return
  try {
    await deleteIamGroupsById({ path: { id }, throwOnError: true })
    if (selectedGroupId.value === id) {
      selectedGroupId.value = ''
      groupMembers.value = []
    }
    await loadGroups()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, 'Failed to delete group'))
  }
}

async function selectGroup(group: HandlersIamGroupResponse) {
  selectedGroupId.value = group.id || ''
  await loadGroupMembers()
}

async function addGroupMember() {
  if (!selectedGroupId.value || !memberUserId.value) return
  try {
    await postIamGroupsByIdMembers({
      path: { id: selectedGroupId.value },
      body: { user_id: memberUserId.value, source: 'manual' },
      throwOnError: true,
    })
    memberUserId.value = ''
    await loadGroupMembers()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, 'Failed to add group member'))
  }
}

async function removeGroupMember(userId: string) {
  if (!selectedGroupId.value || !userId) return
  try {
    await deleteIamGroupsByIdMembersByUserId({ path: { id: selectedGroupId.value, user_id: userId }, throwOnError: true })
    await loadGroupMembers()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, 'Failed to remove group member'))
  }
}

function resetProviderForm() {
  providerForm.id = ''
  providerForm.type = 'oidc'
  providerForm.key = ''
  providerForm.name = ''
  providerForm.enabled = true
  providerForm.config = oidcConfigJSON()
  providerForm.attribute_mapping = attributeMappingJSON()
  providerForm.jit_enabled = true
  providerForm.email_linking_policy = 'link_existing'
  providerForm.trust_email = true
}

function resetProviderConfig() {
  providerForm.config = providerForm.type === 'saml' ? samlConfigJSON() : oidcConfigJSON()
}

function editProvider(provider: HandlersIamssoProviderResponse) {
  providerForm.id = provider.id || ''
  providerForm.type = provider.type || 'oidc'
  providerForm.key = provider.key || ''
  providerForm.name = provider.name || ''
  providerForm.enabled = !!provider.enabled
  providerForm.config = stringifyJSON(provider.config)
  providerForm.attribute_mapping = stringifyJSON(provider.attribute_mapping)
  providerForm.jit_enabled = provider.jit_enabled !== false
  providerForm.email_linking_policy = provider.email_linking_policy || 'link_existing'
  providerForm.trust_email = !!provider.trust_email
}

async function saveProvider() {
  savingProvider.value = true
  try {
    const body = {
      type: providerForm.type,
      key: providerForm.key.trim(),
      name: providerForm.name.trim(),
      enabled: providerForm.enabled,
      config: parseJSON(providerForm.config, 'config'),
      attribute_mapping: parseJSON(providerForm.attribute_mapping, 'attribute_mapping'),
      jit_enabled: providerForm.jit_enabled,
      email_linking_policy: providerForm.email_linking_policy,
      trust_email: providerForm.trust_email,
    }
    if (providerForm.id) {
      await putIamSsoProvidersById({ path: { id: providerForm.id }, body: body as never, throwOnError: true })
    } else {
      await postIamSsoProviders({ body: body as never, throwOnError: true })
    }
    await loadProviders()
    resetProviderForm()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, 'Failed to save SSO provider'))
  } finally {
    savingProvider.value = false
  }
}

async function deleteProvider(id: string) {
  if (!id) return
  try {
    await deleteIamSsoProvidersById({ path: { id }, throwOnError: true })
    if (selectedProviderId.value === id) {
      selectedProviderId.value = ''
      mappings.value = []
    }
    await loadProviders()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, 'Failed to delete SSO provider'))
  }
}

async function selectProvider(provider: HandlersIamssoProviderResponse) {
  selectedProviderId.value = provider.id || ''
  await loadMappings()
}

async function saveMapping() {
  if (!selectedProviderId.value || !mappingExternalGroup.value || !mappingGroupId.value) return
  try {
    await postIamSsoProvidersByIdGroupMappings({
      path: { id: selectedProviderId.value },
      body: { external_group: mappingExternalGroup.value.trim(), group_id: mappingGroupId.value },
      throwOnError: true,
    })
    mappingExternalGroup.value = ''
    mappingGroupId.value = ''
    await loadMappings()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, 'Failed to save group mapping'))
  }
}

async function deleteMapping(externalGroup: string) {
  if (!selectedProviderId.value || !externalGroup) return
  try {
    await deleteIamSsoProvidersByIdGroupMappingsByExternalGroup({
      path: { id: selectedProviderId.value, external_group: externalGroup },
      throwOnError: true,
    })
    await loadMappings()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, 'Failed to delete group mapping'))
  }
}

async function assignBotRole() {
  if (!selectedBotId.value || !assignmentPrincipalId.value || !assignmentRoleKey.value) return
  try {
    await postIamBotsByBotIdPrincipalRoles({
      path: { bot_id: selectedBotId.value },
      body: {
        principal_type: assignmentPrincipalType.value,
        principal_id: assignmentPrincipalId.value,
        role_key: assignmentRoleKey.value,
      },
      throwOnError: true,
    })
    assignmentPrincipalId.value = ''
    await loadPrincipalRoles()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, 'Failed to assign bot role'))
  }
}

async function deletePrincipalRole(id: string) {
  if (!id) return
  try {
    await deleteIamPrincipalRolesById({ path: { id }, throwOnError: true })
    await loadPrincipalRoles()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, 'Failed to delete role assignment'))
  }
}

function accountLabel(user: GithubComMemohaiMemohInternalAccountsAccount) {
  return user.display_name || user.username || user.email || user.id || ''
}

function principalRoleLabel(role: HandlersIamPrincipalRoleResponse) {
  if (role.principal_type === 'group') {
    return `group:${role.group_display_name || role.group_key || role.principal_id}`
  }
  return `user:${role.user_display_name || role.user_username || role.user_email || role.principal_id}`
}

function parseJSON(value: string, field: string): unknown {
  try {
    return JSON.parse(value || '{}')
  } catch {
    throw new Error(`${field} must be valid JSON`)
  }
}

function stringifyJSON(value: unknown) {
  if (!value) return '{}'
  if (typeof value === 'string') return value
  return JSON.stringify(value, null, 2)
}

function attributeMappingJSON() {
  return JSON.stringify({
    subject: 'sub',
    email: 'email',
    username: 'preferred_username',
    display_name: 'name',
    avatar_url: 'picture',
    groups: ['groups'],
  }, null, 2)
}

function oidcConfigJSON() {
  return JSON.stringify({
    issuer: '',
    client_id: '',
    client_secret: '',
    redirect_url: '',
    scopes: ['openid', 'profile', 'email'],
  }, null, 2)
}

function samlConfigJSON() {
  return JSON.stringify({
    entity_id: '',
    metadata_xml: '',
    metadata_url: '',
    acs_url: '',
  }, null, 2)
}
</script>
