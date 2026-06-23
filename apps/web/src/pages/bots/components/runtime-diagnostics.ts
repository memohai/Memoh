import type {
  RuntimediagnosticsAcpAgentDiagnostic,
  RuntimediagnosticsResponse,
  RuntimediagnosticsRuntimeEventSummary,
  RuntimediagnosticsState,
} from '@memohai/sdk'

export type RuntimeDiagnosticsScope = 'acp' | 'workspace'

export type RuntimeDiagnosticTextResolver = (
  key: string,
  fallback: string,
  params?: Record<string, number | string>,
) => string

export interface RuntimeDiagnosticBuildOptions {
  scope: RuntimeDiagnosticsScope
  agentId?: string
  text?: RuntimeDiagnosticTextResolver
}

export interface RuntimeDiagnosticVisualScenario {
  id: string
  scope: RuntimeDiagnosticsScope
  agentId?: string
  viewport: {
    width: number
    height: number
  }
  colorScheme: 'dark' | 'light'
  locale: 'en' | 'zh'
}

export type RuntimeDiagnosticBadgeVariant =
  | 'success'
  | 'warning'
  | 'destructive'
  | 'secondary'
  | 'default'

export interface RuntimeDiagnosticRow {
  label: string
  value: string
  detail?: string
  state?: RuntimediagnosticsState
  code?: string
  mono?: boolean
  copyValue?: string
}

export interface RuntimeDiagnosticSection {
  id: 'startup' | 'auth' | 'cli' | 'model_profile' | 'session' | 'events' | 'raw'
  title: string
  rows: RuntimeDiagnosticRow[]
}

export function stateBadgeVariant(state: RuntimediagnosticsState | undefined): RuntimeDiagnosticBadgeVariant {
  switch (state) {
    case 'ok':
      return 'success'
    case 'warn':
      return 'warning'
    case 'error':
      return 'destructive'
    case 'unknown':
      return 'default'
    case 'disabled':
    case 'not_applicable':
    default:
      return 'secondary'
  }
}

export function stateLabel(state: RuntimediagnosticsState | undefined): string {
  switch (state) {
    case 'ok':
      return 'OK'
    case 'warn':
      return 'Warning'
    case 'error':
      return 'Error'
    case 'disabled':
      return 'Disabled'
    case 'not_applicable':
      return 'N/A'
    case 'unknown':
    default:
      return 'Unknown'
  }
}

export function runtimeDiagnosticVisualScenarios(): RuntimeDiagnosticVisualScenario[] {
  return [
    {
      id: 'acp-desktop-light-en',
      scope: 'acp',
      agentId: 'codex',
      viewport: { width: 1440, height: 1000 },
      colorScheme: 'light',
      locale: 'en',
    },
    {
      id: 'container-desktop-light-en',
      scope: 'workspace',
      viewport: { width: 1440, height: 1000 },
      colorScheme: 'light',
      locale: 'en',
    },
    {
      id: 'desktop-mobile-dark-zh',
      scope: 'workspace',
      viewport: { width: 390, height: 920 },
      colorScheme: 'dark',
      locale: 'zh',
    },
  ]
}

export function selectRuntimeDiagnosticAgents(
  diagnostics: RuntimediagnosticsResponse | undefined,
  agentId?: string,
): RuntimediagnosticsAcpAgentDiagnostic[] {
  const agents = diagnostics?.acp_agents ?? []
  const normalized = normalizeID(agentId)
  if (!normalized) return agents
  return agents.filter(agent => normalizeID(agent.agent_id) === normalized)
}

export function buildRuntimeDiagnosticReadouts(
  diagnostics: RuntimediagnosticsResponse | undefined,
  scope: RuntimeDiagnosticsScope,
  agentId?: string,
  text?: RuntimeDiagnosticTextResolver,
): RuntimeDiagnosticRow[] {
  if (!diagnostics) return []
  if (scope === 'acp') {
    return selectRuntimeDiagnosticAgents(diagnostics, agentId).map(agent => ({
      label: agent.display_name || agent.agent_id || tx(text, 'bots.runtimeDiagnostics.rows.acpAgent', 'ACP agent'),
      value: diagnosticStateLabel(agent.state, text),
      detail: diagnosticDetail(agent, text) || sessionStateText(agent.session_resume?.state, text),
      state: agent.state,
      code: agent.code,
    }))
  }

  return [
    diagnostics.workspace
      ? {
          label: tx(text, 'bots.runtimeDiagnostics.rows.workspace', 'Workspace'),
          value: diagnosticStateLabel(diagnostics.workspace.state, text),
          detail: diagnosticDetail(diagnostics.workspace, text) || diagnostics.workspace.backend,
          state: diagnostics.workspace.state,
          code: diagnostics.workspace.code,
        }
      : null,
    diagnostics.container
      ? {
          label: tx(text, 'bots.runtimeDiagnostics.rows.container', 'Container'),
          value: diagnosticStateLabel(diagnostics.container.state, text),
          detail: diagnosticDetail(diagnostics.container, text) || diagnostics.container.status || diagnostics.container.runtime_backend,
          state: diagnostics.container.state,
          code: diagnostics.container.code,
        }
      : null,
    diagnostics.display
      ? {
          label: tx(text, 'bots.runtimeDiagnostics.rows.display', 'Display'),
          value: diagnosticStateLabel(diagnostics.display.state, text),
          detail: diagnosticDetail(diagnostics.display, text) || diagnostics.display.unavailable_reason || diagnostics.display.transport,
          state: diagnostics.display.state,
          code: diagnostics.display.code,
        }
      : null,
  ].filter((row): row is RuntimeDiagnosticRow => row !== null)
}

export function buildRuntimeDiagnosticSections(
  diagnostics: RuntimediagnosticsResponse | undefined,
  options: RuntimeDiagnosticBuildOptions,
): RuntimeDiagnosticSection[] {
  const agents = options.scope === 'acp'
    ? selectRuntimeDiagnosticAgents(diagnostics, options.agentId)
    : (diagnostics?.acp_agents ?? [])
  const events = selectRuntimeEvents(diagnostics?.recent_events ?? [], options.scope, options.agentId)

  const sections: RuntimeDiagnosticSection[] = [
    {
      id: 'startup',
      title: sectionText('startup', 'Startup Decision', options.text),
      rows: buildStartupRows(diagnostics, options.scope, agents, options.text),
    },
    {
      id: 'auth',
      title: sectionText('auth', 'Auth', options.text),
      rows: agents.map(agent => ({
        label: agent.display_name || agent.agent_id || tx(options.text, 'bots.runtimeDiagnostics.rows.acpAgent', 'ACP agent'),
        value: authStateText(agent, options.text),
        detail: [
          agent.auth?.source ? tx(options.text, 'bots.runtimeDiagnostics.detail.source', 'source: {value}', { value: agent.auth.source }) : '',
          missingFieldsText(agent.auth?.missing_fields, options.text),
          agent.auth?.warning_detail || '',
        ].filter(Boolean).join(' · '),
        state: authState(agent),
        code: agent.auth?.warning_code,
      })),
    },
    {
      id: 'cli',
      title: sectionText('cli', 'CLI', options.text),
      rows: agents.map(agent => ({
        label: agent.display_name || agent.agent_id || tx(options.text, 'bots.runtimeDiagnostics.rows.acpAgent', 'ACP agent'),
        value: cliCommandText(agent),
        detail: [
          agent.cli?.resolved_path ? tx(options.text, 'bots.runtimeDiagnostics.detail.path', 'path: {value}', { value: agent.cli.resolved_path }) : '',
          agent.cli?.source ? tx(options.text, 'bots.runtimeDiagnostics.detail.source', 'source: {value}', { value: agent.cli.source }) : '',
          agent.cli?.error || '',
        ].filter(Boolean).join(' · '),
        state: agent.cli?.available === true ? 'ok' : agent.enabled ? 'error' : 'disabled',
        mono: true,
      })),
    },
    {
      id: 'model_profile',
      title: sectionText('model_profile', 'Model/Profile', options.text),
      rows: agents.flatMap(agent => modelProfileRows(agent, options.text)),
    },
    {
      id: 'session',
      title: sectionText('session', 'Session Resume', options.text),
      rows: agents.map(agent => ({
        label: agent.display_name || agent.agent_id || tx(options.text, 'bots.runtimeDiagnostics.rows.acpAgent', 'ACP agent'),
        value: sessionStateText(agent.session_resume?.state, options.text),
        detail: [
          agent.session_resume?.detail || '',
          agent.session_resume?.session_id ? tx(options.text, 'bots.runtimeDiagnostics.detail.session', 'session: {value}', { value: agent.session_resume.session_id }) : '',
          agent.session_resume?.runtime_id ? tx(options.text, 'bots.runtimeDiagnostics.detail.runtime', 'runtime: {value}', { value: agent.session_resume.runtime_id }) : '',
        ].filter(Boolean).join(' · '),
        state: sessionDiagnosticState(agent.session_resume?.state),
      })),
    },
    {
      id: 'events',
      title: sectionText('events', 'Recent Errors', options.text),
      rows: events.map(event => ({
        label: event.code || event.phase || event.scope || tx(options.text, 'bots.runtimeDiagnostics.rows.event', 'event'),
        value: event.severity || tx(options.text, 'bots.runtimeDiagnostics.rows.event', 'event'),
        detail: [event.message || '', event.created_at || ''].filter(Boolean).join(' · '),
        state: event.severity === 'error' ? 'error' : event.severity === 'warn' ? 'warn' : 'unknown',
      })),
    },
    {
      id: 'raw',
      title: sectionText('raw', 'Raw Evidence', options.text),
      rows: rawEvidenceRows(diagnostics, options.scope, options.agentId, options.text),
    },
  ]

  return sections.map(section => ({
    ...section,
    rows: section.rows.length > 0
      ? section.rows
      : [{
          label: section.title,
          value: tx(options.text, 'bots.runtimeDiagnostics.emptyValue', '-'),
          detail: tx(options.text, 'bots.runtimeDiagnostics.emptyGroup', 'No diagnostic evidence for this group yet.'),
          state: 'unknown',
        }],
  }))
}

function buildStartupRows(
  diagnostics: RuntimediagnosticsResponse | undefined,
  scope: RuntimeDiagnosticsScope,
  agents: RuntimediagnosticsAcpAgentDiagnostic[],
  text?: RuntimeDiagnosticTextResolver,
): RuntimeDiagnosticRow[] {
  if (!diagnostics) return []

  const rows: RuntimeDiagnosticRow[] = [
    {
      label: tx(text, 'bots.runtimeDiagnostics.rows.overall', 'Overall'),
      value: diagnosticStateLabel(diagnostics.overall_state, text),
      detail: runtimeDiagnosticSummaryText(diagnostics, text),
      state: diagnostics.overall_state,
    },
  ]

  if (scope === 'workspace') {
    if (diagnostics.workspace) {
      rows.push({
        label: tx(text, 'bots.runtimeDiagnostics.rows.workspace', 'Workspace'),
        value: diagnostics.workspace.backend || diagnosticStateLabel(diagnostics.workspace.state, text),
        detail: diagnosticDetail(diagnostics.workspace, text),
        state: diagnostics.workspace.state,
        code: diagnostics.workspace.code,
      })
    }
    if (diagnostics.container) {
      rows.push({
        label: tx(text, 'bots.runtimeDiagnostics.rows.container', 'Container'),
        value: diagnostics.container.status || diagnosticStateLabel(diagnostics.container.state, text),
        detail: diagnosticDetail(diagnostics.container, text),
        state: diagnostics.container.state,
        code: diagnostics.container.code,
      })
    }
    if (diagnostics.display) {
      rows.push({
        label: tx(text, 'bots.runtimeDiagnostics.rows.display', 'Display'),
        value: diagnostics.display.enabled ? diagnosticStateLabel(diagnostics.display.state, text) : tx(text, 'bots.runtimeDiagnostics.states.disabled', 'Disabled'),
        detail: diagnosticDetail(diagnostics.display, text),
        state: diagnostics.display.state,
        code: diagnostics.display.code,
      })
    }
  } else {
    rows.push(...agents.map(agent => ({
      label: agent.display_name || agent.agent_id || tx(text, 'bots.runtimeDiagnostics.rows.acpAgent', 'ACP agent'),
      value: agent.enabled ? diagnosticStateLabel(agent.state, text) : tx(text, 'bots.runtimeDiagnostics.states.disabled', 'Disabled'),
      detail: diagnosticDetail(agent, text) || diagnosticNextAction(agent, text),
      state: agent.state,
      code: agent.code,
    })))
  }

  return rows
}

function modelProfileRows(agent: RuntimediagnosticsAcpAgentDiagnostic, text?: RuntimeDiagnosticTextResolver): RuntimeDiagnosticRow[] {
  const name = agent.display_name || agent.agent_id || tx(text, 'bots.runtimeDiagnostics.rows.acpAgent', 'ACP agent')
  return [
    {
      label: tx(text, 'bots.runtimeDiagnostics.rows.providerProfile', '{name} profile', { name }),
      value: agent.profile?.registered === false
        ? tx(text, 'bots.runtimeDiagnostics.values.notRegistered', 'not registered')
        : tx(text, 'bots.runtimeDiagnostics.values.registered', 'registered'),
      detail: [
        agent.profile?.backend_supported === false
          ? tx(text, 'bots.runtimeDiagnostics.values.backendUnsupported', 'backend unsupported')
          : tx(text, 'bots.runtimeDiagnostics.values.backendSupported', 'backend supported'),
        agent.profile?.session_mode_pin ? tx(text, 'bots.runtimeDiagnostics.detail.mode', 'mode: {value}', { value: agent.profile.session_mode_pin }) : '',
        pinText(agent.profile?.session_config_pins, text),
      ].filter(Boolean).join(' · '),
      state: agent.profile?.registered === false || agent.profile?.backend_supported === false ? 'error' : 'ok',
    },
    {
      label: tx(text, 'bots.runtimeDiagnostics.rows.providerModel', '{name} model', { name }),
      value: modelStateText(agent, text),
      detail: [
        agent.model?.current_model_id ? tx(text, 'bots.runtimeDiagnostics.detail.current', 'current: {value}', { value: agent.model.current_model_id }) : '',
        agent.model?.default_model_id ? tx(text, 'bots.runtimeDiagnostics.detail.default', 'default: {value}', { value: agent.model.default_model_id }) : '',
        agent.model?.detail || '',
      ].filter(Boolean).join(' · '),
      state: agent.model?.state === 'known' ? 'ok' : agent.model?.state === 'unsupported' ? 'warn' : 'unknown',
    },
  ]
}

function authState(agent: RuntimediagnosticsAcpAgentDiagnostic): RuntimediagnosticsState {
  if (!agent.enabled) return 'disabled'
  if (agent.auth?.warning_code) return 'warn'
  if (agent.auth?.self_managed) return 'ok'
  if (agent.auth?.api_key_present || agent.auth?.oauth_present) return 'ok'
  return 'error'
}

function authStateText(agent: RuntimediagnosticsAcpAgentDiagnostic, text?: RuntimeDiagnosticTextResolver): string {
  const auth = agent.auth
  if (!agent.enabled) return tx(text, 'bots.runtimeDiagnostics.states.disabled', 'Disabled')
  if (!auth) return tx(text, 'bots.runtimeDiagnostics.states.unknown', 'Unknown')
  if (auth.warning_code) return tx(text, 'bots.runtimeDiagnostics.values.authWarning', '{mode} warning', { mode: auth.mode || 'auth' })
  if (auth.self_managed) return tx(text, 'bots.runtimeDiagnostics.values.selfManaged', 'self-managed')
  if (auth.api_key_present) return tx(text, 'bots.runtimeDiagnostics.values.authReady', '{mode} ready', { mode: auth.mode || 'api key' })
  if (auth.oauth_present) return tx(text, 'bots.runtimeDiagnostics.values.authReady', '{mode} ready', { mode: auth.mode || 'oauth' })
  return tx(text, 'bots.runtimeDiagnostics.values.authMissing', '{mode} missing', { mode: auth.mode || 'auth' })
}

function cliCommandText(agent: RuntimediagnosticsAcpAgentDiagnostic): string {
  const cli = agent.cli
  if (!cli) return 'unknown'
  const command = cli.effective_command || cli.configured_command || 'unknown'
  const args = cli.effective_args?.length ? ` ${cli.effective_args.join(' ')}` : ''
  return `${command}${args}`
}

function modelStateText(agent: RuntimediagnosticsAcpAgentDiagnostic, text?: RuntimeDiagnosticTextResolver): string {
  const model = agent.model
  if (!model) return tx(text, 'bots.runtimeDiagnostics.states.unknown', 'Unknown')
  if (model.state === 'unknown_until_runtime_start') return tx(text, 'bots.runtimeDiagnostics.modelStates.unknown_until_runtime_start', 'unknown until runtime start')
  if (model.current_model_id || model.default_model_id) return model.current_model_id || model.default_model_id || ''
  return tx(text, `bots.runtimeDiagnostics.modelStates.${model.state || 'unknown'}`, model.state || 'unknown')
}

function sessionStateText(state: string | undefined, text?: RuntimeDiagnosticTextResolver): string {
  switch (state) {
    case 'warm_resumable':
      return tx(text, 'bots.runtimeDiagnostics.sessionStates.warm_resumable', 'warm reusable')
    case 'cold_start_required':
      return tx(text, 'bots.runtimeDiagnostics.sessionStates.cold_start_required', 'cold start required')
    case 'no_acp_session':
      return tx(text, 'bots.runtimeDiagnostics.sessionStates.no_acp_session', 'no ACP session')
    case 'disabled':
      return tx(text, 'bots.runtimeDiagnostics.states.disabled', 'Disabled')
    case 'blocked':
      return tx(text, 'bots.runtimeDiagnostics.sessionStates.blocked', 'blocked')
    default:
      return tx(text, 'bots.runtimeDiagnostics.states.unknown', 'Unknown')
  }
}

function sessionDiagnosticState(state: string | undefined): RuntimediagnosticsState {
  switch (state) {
    case 'warm_resumable':
    case 'no_acp_session':
      return 'ok'
    case 'cold_start_required':
      return 'warn'
    case 'blocked':
      return 'error'
    case 'disabled':
      return 'disabled'
    default:
      return 'unknown'
  }
}

function missingFieldsText(fields: string[] | undefined, text?: RuntimeDiagnosticTextResolver): string {
  return fields?.length
    ? tx(text, 'bots.runtimeDiagnostics.detail.missing', 'missing: {value}', { value: fields.join(', ') })
    : ''
}

function pinText(pins: Record<string, string> | undefined, text?: RuntimeDiagnosticTextResolver): string {
  if (!pins || Object.keys(pins).length === 0) return ''
  return tx(text, 'bots.runtimeDiagnostics.detail.pins', 'pins: {value}', {
    value: Object.entries(pins).map(([key, value]) => `${key}=${value}`).join(', '),
  })
}

export function runtimeDiagnosticSummaryText(
  diagnostics: RuntimediagnosticsResponse | undefined,
  text?: RuntimeDiagnosticTextResolver,
): string {
  if (!diagnostics) return tx(text, 'bots.runtimeDiagnostics.loadingSummary', 'Checking runtime diagnostics...')
  const enabled = (diagnostics.acp_agents ?? []).filter(agent => agent.enabled).length
  const blocked = (diagnostics.acp_agents ?? []).filter(agent => agent.enabled && agent.state === 'error').length
  const warm = (diagnostics.acp_agents ?? []).filter(agent => agent.enabled && agent.session_resume?.state === 'warm_resumable').length
  return tx(
    text,
    'bots.runtimeDiagnostics.summaryTemplate',
    'Workspace {workspace}, container {container}, display {display}. ACP providers: {enabled} enabled, {blocked} blocked, {warm} warm resumable.',
    {
      workspace: diagnosticStateLabel(diagnostics.workspace?.state, text),
      container: diagnosticStateLabel(diagnostics.container?.state, text),
      display: diagnosticStateLabel(diagnostics.display?.state, text),
      enabled,
      blocked,
      warm,
    },
  )
}

function diagnosticStateLabel(state: RuntimediagnosticsState | undefined, text?: RuntimeDiagnosticTextResolver): string {
  const key = state || 'unknown'
  return tx(text, `bots.runtimeDiagnostics.states.${key}`, stateLabel(state))
}

function sectionText(id: RuntimeDiagnosticSection['id'], fallback: string, text?: RuntimeDiagnosticTextResolver): string {
  return tx(text, `bots.runtimeDiagnostics.sections.${id}`, fallback)
}

function diagnosticDetail(item: { code?: string; detail?: string }, text?: RuntimeDiagnosticTextResolver): string {
  const fallback = item.detail || item.code || ''
  if (!item.code) return fallback
  return tx(text, `bots.runtimeDiagnostics.codes.${item.code}.detail`, fallback)
}

function diagnosticNextAction(item: { code?: string; next_action?: string }, text?: RuntimeDiagnosticTextResolver): string {
  const fallback = item.next_action || ''
  if (!item.code) return fallback
  return tx(text, `bots.runtimeDiagnostics.codes.${item.code}.nextAction`, fallback)
}

function rawEvidenceRows(
  diagnostics: RuntimediagnosticsResponse | undefined,
  scope: RuntimeDiagnosticsScope,
  agentId?: string,
  text?: RuntimeDiagnosticTextResolver,
): RuntimeDiagnosticRow[] {
  if (!diagnostics) return []
  const rows: RuntimeDiagnosticRow[] = []
  const add = (key: string, fallback: string, value: unknown) => {
    const detail = JSON.stringify(value, null, 2)
    rows.push({
      label: tx(text, `bots.runtimeDiagnostics.raw.${key}`, fallback),
      value: tx(text, 'bots.runtimeDiagnostics.values.available', 'available'),
      detail,
      mono: true,
      copyValue: detail,
    })
  }

  add('checkedAt', 'Checked at', diagnostics.checked_at)
  add('overall', 'Overall state', {
    overall_state: diagnostics.overall_state,
    summary: runtimeDiagnosticSummaryText(diagnostics, text),
  })
  if (scope === 'acp') {
    add('acpAgents', 'ACP agents', selectRuntimeDiagnosticAgents(diagnostics, agentId))
    add('recentEvents', 'Recent events', selectRuntimeEvents(diagnostics.recent_events ?? [], scope, agentId))
    return rows
  }
  add('workspace', 'Workspace', diagnostics.workspace)
  add('container', 'Container', diagnostics.container)
  add('display', 'Display', diagnostics.display)
  add('recentEvents', 'Recent events', selectRuntimeEvents(diagnostics.recent_events ?? [], scope))
  return rows
}

function tx(
  text: RuntimeDiagnosticTextResolver | undefined,
  key: string,
  fallback: string,
  params?: Record<string, number | string>,
): string {
  if (text) return text(key, fallback, params)
  return interpolate(fallback, params)
}

function interpolate(fallback: string, params?: Record<string, number | string>): string {
  if (!params) return fallback
  return Object.entries(params).reduce(
    (value, [key, replacement]) => value.replaceAll(`{${key}}`, String(replacement)),
    fallback,
  )
}

function selectRuntimeEvents(
  events: RuntimediagnosticsRuntimeEventSummary[],
  scope: RuntimeDiagnosticsScope,
  agentId?: string,
): RuntimediagnosticsRuntimeEventSummary[] {
  const normalized = normalizeID(agentId)
  return events.filter((event) => {
    if (scope === 'workspace') return event.scope !== 'acp'
    if (!normalized) return event.scope === 'acp'
    return event.scope === 'acp' && normalizeID(event.agent_id) === normalized
  })
}

function normalizeID(value: string | undefined): string {
  return (value ?? '').trim().toLowerCase()
}
