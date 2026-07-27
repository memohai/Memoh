import type { HandlersSupermarketCatalogSkill } from '@memohai/sdk'

/** Prefer the stable registry_id — it matches install_id's leading segment. */
export function registryDisplayPrefix(registryId?: string, _registryName?: string): string {
  return registryId?.trim() || ''
}

export function formatNamespacedSkillName(
  skill: Pick<HandlersSupermarketCatalogSkill, 'name' | 'skill_id'>,
  registryPrefix: string,
): string {
  const skillName = skill.name || skill.skill_id || ''
  if (!registryPrefix) return skillName
  return `${registryPrefix}/${skillName}`
}
