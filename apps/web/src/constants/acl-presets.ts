export type AclPresetKey =
  | 'allow_all'
  | 'private_only'
  | 'group_only'
  | 'group_and_thread_only'
  | 'deny_all'

export interface AclPresetOption {
  value: AclPresetKey
  titleKey: string
  descriptionKey: string
}

export const defaultAclPreset: AclPresetKey = 'allow_all'

export const aclPresetOptions: AclPresetOption[] = [
  {
    value: 'allow_all',
    titleKey: 'bots.aclPresets.allowAll.title',
    descriptionKey: 'bots.aclPresets.allowAll.description',
  },
  {
    value: 'private_only',
    titleKey: 'bots.aclPresets.privateOnly.title',
    descriptionKey: 'bots.aclPresets.privateOnly.description',
  },
  {
    value: 'group_only',
    titleKey: 'bots.aclPresets.groupOnly.title',
    descriptionKey: 'bots.aclPresets.groupOnly.description',
  },
  {
    value: 'group_and_thread_only',
    titleKey: 'bots.aclPresets.groupAndThreadOnly.title',
    descriptionKey: 'bots.aclPresets.groupAndThreadOnly.description',
  },
  {
    value: 'deny_all',
    titleKey: 'bots.aclPresets.denyAll.title',
    descriptionKey: 'bots.aclPresets.denyAll.description',
  },
]
