import type { DockviewApi, IDockviewGroupPanel } from 'dockview-vue'

type PositionedGroup = {
  group: IDockviewGroupPanel
  rect: DOMRect
}

function measuredGroups(containerApi: DockviewApi): PositionedGroup[] {
  const measured = containerApi.groups
    .map(group => ({ group, rect: group.element.getBoundingClientRect() }))
    .filter(({ rect }) => rect.width > 0 && rect.height > 0)

  // During restore/jsdom/pre-layout frames dockview may report zero rects for
  // every group. Fall back to creation order so single-group layouts still
  // reserve shell chrome instead of flickering under the buttons.
  if (measured.length > 0) return measured

  return containerApi.groups.map(group => ({
    group,
    rect: {
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      width: 1,
      height: 1,
    } as DOMRect,
  }))
}

function topBand(groups: PositionedGroup[]): PositionedGroup[] {
  if (groups.length <= 1) return groups
  const minTop = Math.min(...groups.map(({ rect }) => rect.top))
  return groups.filter(({ rect }) => Math.abs(rect.top - minTop) <= 1)
}

export function isWorkspaceTopLeftGroup(containerApi: DockviewApi, groupId: string): boolean {
  const topGroups = topBand(measuredGroups(containerApi))
  topGroups.sort((a, b) => {
    const leftDelta = a.rect.left - b.rect.left
    if (Math.abs(leftDelta) > 1) return leftDelta
    return a.group.id.localeCompare(b.group.id)
  })
  return topGroups[0]?.group.id === groupId
}

export function isWorkspaceTopRightGroup(containerApi: DockviewApi, groupId: string): boolean {
  const topGroups = topBand(measuredGroups(containerApi))
  topGroups.sort((a, b) => {
    const rightDelta = b.rect.right - a.rect.right
    if (Math.abs(rightDelta) > 1) return rightDelta
    return a.group.id.localeCompare(b.group.id)
  })
  return topGroups[0]?.group.id === groupId
}
