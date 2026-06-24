import { describe, expect, it } from 'vitest'
import type { RuntimediagnosticsResponse } from '@memohai/sdk'
import {
  buildRuntimeDiagnosticSections,
  runtimeDiagnosticVisualScenarios,
  selectRuntimeDiagnosticAgents,
  stateBadgeVariant,
} from './runtime-diagnostics'

const baseDiagnostics: RuntimediagnosticsResponse = {
  checked_at: '2026-06-23T12:00:00Z',
  overall_state: 'warn',
  summary: 'Runtime needs attention',
  workspace: {
    state: 'ok',
    label: 'Workspace',
    detail: 'Workspace bridge reachable',
    backend: 'containerd',
    bridge_reachable: true,
    mcp_reachable: true,
  },
  container: {
    state: 'warn',
    label: 'Container',
    detail: 'Runtime exists but task is stopped',
    exists: true,
    runtime_backend: 'containerd',
    status: 'stopped',
    task_running: false,
  },
  display: {
    state: 'disabled',
    label: 'Display',
    detail: 'Desktop is disabled for this bot',
    enabled: false,
    available: false,
  },
  acp_agents: [
    {
      agent_id: 'claude-code',
      display_name: 'Claude Code',
      enabled: true,
      setup_mode: 'oauth',
      workspace_backend: 'local',
      state: 'ok',
      label: 'Claude Code',
      cli: {
        available: true,
        configured_command: 'claude',
        effective_command: 'claude',
        resolved_path: '/usr/local/bin/claude',
        source: 'local_command',
      },
      auth: {
        mode: 'oauth',
        oauth_present: true,
        source: 'local',
      },
      profile: {
        registered: true,
        backend_supported: true,
        session_mode_pin: 'sticky',
      },
      model: {
        state: 'known',
        current_model_id: 'claude-sonnet',
        default_model_id: 'claude-sonnet',
      },
      session_resume: {
        state: 'warm_resumable',
        session_id: 'session-1',
        runtime_id: 'runtime-1',
      },
    },
    {
      agent_id: 'codex',
      display_name: 'Codex',
      enabled: true,
      setup_mode: 'oauth',
      workspace_backend: 'container',
      state: 'error',
      code: 'cli_missing',
      label: 'Codex',
      cli: {
        available: false,
        configured_command: 'codex',
        effective_command: 'codex',
        error: 'executable file not found',
        source: 'profile_command',
      },
      auth: {
        mode: 'oauth',
        oauth_present: false,
        source: 'container',
      },
      session_resume: {
        state: 'blocked',
        detail: 'CLI is missing',
      },
    },
  ],
  recent_events: [
    {
      id: 'event-1',
      scope: 'acp',
      agent_id: 'codex',
      severity: 'error',
      code: 'runtime_start_failed',
      message: 'adapter exited',
      created_at: '2026-06-23T11:55:00Z',
    },
  ],
}

describe('runtime diagnostics view helpers', () => {
  it('focuses the requested ACP agent before rendering a detail drawer', () => {
    const agents = selectRuntimeDiagnosticAgents(baseDiagnostics, 'codex')

    expect(agents).toHaveLength(1)
    expect(agents[0]?.agent_id).toBe('codex')
    expect(agents[0]?.cli?.available).toBe(false)
  })

  it('maps diagnostic state to stable badge variants', () => {
    expect(stateBadgeVariant('ok')).toBe('success')
    expect(stateBadgeVariant('warn')).toBe('warning')
    expect(stateBadgeVariant('error')).toBe('destructive')
    expect(stateBadgeVariant('disabled')).toBe('secondary')
    expect(stateBadgeVariant(undefined)).toBe('secondary')
  })

  it('builds workspace and ACP sections without inventing scheduling actions', () => {
    const sections = buildRuntimeDiagnosticSections(baseDiagnostics, {
      scope: 'workspace',
    })

    expect(sections.map(section => section.title)).toEqual([
      'Startup Decision',
      'Auth',
      'CLI',
      'Model/Profile',
      'Session Resume',
      'Recent Errors',
      'Raw Evidence',
    ])
    expect(sections.flatMap(section => section.rows).some(row => /start|stop|prepare|ensure|resume/i.test(row.label))).toBe(false)
    expect(sections.find(section => section.title === 'Startup Decision')?.rows.map(row => row.label)).toContain('Workspace')
  })

  it('can route all helper-owned drawer copy through an i18n resolver', () => {
    const tx = (key: string, fallback: string) => `tx:${key}:${fallback}`
    const sections = buildRuntimeDiagnosticSections(baseDiagnostics, {
      scope: 'workspace',
      text: tx,
    })

    expect(sections[0]?.title).toBe('tx:bots.runtimeDiagnostics.sections.startup:Startup Decision')
    expect(sections[0]?.rows[0]?.label).toBe('tx:bots.runtimeDiagnostics.rows.overall:Overall')
    expect(sections[0]?.rows[0]?.value).toBe('tx:bots.runtimeDiagnostics.states.warn:Warning')
    expect(sections.find(section => section.id === 'raw')?.rows[0]?.label).toBe('tx:bots.runtimeDiagnostics.raw.checkedAt:Checked at')
  })

  it('splits raw evidence into readable drawer rows instead of one JSON blob', () => {
    const sections = buildRuntimeDiagnosticSections(baseDiagnostics, {
      scope: 'workspace',
    })
    const raw = sections.find(section => section.id === 'raw')

    expect(raw?.rows.length).toBeGreaterThan(1)
    expect(raw?.rows.map(row => row.label)).toContain('Workspace')
    expect(raw?.rows.map(row => row.label)).toContain('Recent events')
    expect(raw?.rows.every(row => row.mono)).toBe(true)
  })

  it('defines stable visual regression scenarios for the runtime diagnostics drawer', () => {
    const scenarios = runtimeDiagnosticVisualScenarios()

    expect(scenarios.map(scenario => scenario.id)).toEqual([
      'acp-desktop-light-en',
      'container-desktop-light-en',
      'desktop-mobile-dark-zh',
    ])
    expect(scenarios.every(scenario => scenario.viewport.width > 0 && scenario.viewport.height > 0)).toBe(true)
    expect(scenarios.some(scenario => scenario.scope === 'acp' && scenario.agentId === 'codex')).toBe(true)
    expect(scenarios.some(scenario => scenario.scope === 'workspace' && scenario.locale === 'zh')).toBe(true)
  })

  it('treats a warm runtime known model state as healthy evidence', () => {
    const diagnostics: RuntimediagnosticsResponse = {
      ...baseDiagnostics,
      acp_agents: [
        {
          ...baseDiagnostics.acp_agents[0]!,
          model: {
            state: 'known',
            current_model_id: 'claude-sonnet',
          },
        },
      ],
    }

    const sections = buildRuntimeDiagnosticSections(diagnostics, {
      scope: 'acp',
      agentId: 'claude-code',
    })
    const modelRow = sections
      .find(section => section.title === 'Model/Profile')
      ?.rows.find(row => row.label === 'Claude Code model')

    expect(modelRow?.value).toBe('claude-sonnet')
    expect(modelRow?.state).toBe('ok')
  })
})
