import { describe, expect, it } from 'vitest'
import {
  commandResultQuickActionText,
  isCommandResultItemSelectable,
} from './slash-command-result'

describe('slash command result helpers', () => {
  it('maps known help quick actions to sendable slash text', () => {
    expect(commandResultQuickActionText({ id: 'help', title: 'Help', kind: 'quick_action' })).toBe('/help')
    expect(commandResultQuickActionText({ id: 'skill.list', title: 'Skills', kind: 'quick_action' })).toBe('/skill list')
  })

  it('allows server-provided slash titles for unknown quick actions', () => {
    expect(commandResultQuickActionText({ id: 'custom', title: '/custom action', kind: 'quick_action' })).toBe('/custom action')
  })

  it('does not make the current quick action selectable from its own result', () => {
    const item = { id: 'help', title: '/help', kind: 'quick_action' }

    expect(commandResultQuickActionText(item, 'help')).toBe('')
    expect(isCommandResultItemSelectable(item, 'help')).toBe(false)
  })

  it('marks only executable command result rows as selectable', () => {
    expect(isCommandResultItemSelectable({ id: 'skill.list', title: '/skill list', kind: 'quick_action' })).toBe(true)
    expect(isCommandResultItemSelectable({ id: 'skill-a', title: 'skill-a', kind: 'skill' })).toBe(true)
    expect(isCommandResultItemSelectable({ id: 'note', title: 'Note', kind: 'quick_action' })).toBe(false)
    expect(isCommandResultItemSelectable({ id: 'note', title: 'Note' })).toBe(false)
  })
})
