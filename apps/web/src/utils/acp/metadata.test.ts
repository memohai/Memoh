import { describe, expect, it } from 'vitest'
import type { AcpprofilePublicProfile } from '@memohai/sdk'
import {
  ensureACPAgentForm,
  findMissingRequiredACPField,
  findMissingRequiredManagedField,
  isACPAgentEnabled,
  normalizeACPForm,
  readACPAgentConfig,
  readACPConfig,
  withACPMetadata,
  type ACPForm,
} from './metadata'

const codexProfile: AcpprofilePublicProfile = {
  id: 'codex',
  display_name: 'Codex',
  setup_modes: ['api_key', 'oauth', 'self'],
  managed_fields: [
    {
      id: 'api_key',
      label: 'OpenAI API key',
      type: 'password',
      required: true,
      sensitive: true,
    },
    {
      id: 'base_url',
      label: 'OpenAI base URL',
      type: 'url',
    },
  ],
}

const claudeCodeProfile: AcpprofilePublicProfile = {
  id: 'claude-code',
  display_name: 'Claude Code',
  setup_modes: ['api_key', 'oauth', 'self'],
  managed_fields: [
    {
      id: 'api_key',
      label: 'Anthropic API key',
      type: 'password',
      required: true,
      sensitive: true,
    },
    {
      id: 'base_url',
      label: 'Anthropic base URL',
      type: 'url',
    },
    {
      id: 'oauth_token',
      label: 'Claude Code OAuth token',
      type: 'password',
      required: true,
      sensitive: true,
    },
  ],
}

describe('acp-metadata', () => {
  it('builds ACP form state from profile schema and metadata', () => {
    const metadata = {
      acp: {
        agents: {
          codex: {
            enabled: true,
          },
        },
      },
    }

    expect(isACPAgentEnabled(metadata, 'Codex')).toBe(true)
    expect(readACPConfig(metadata, [codexProfile])).toEqual({
      agents: {
        codex: {
          enabled: true,
          setup_mode: 'api_key',
          managed: {
            api_key: '',
            base_url: '',
          },
        },
      },
    })
  })

  it('keeps only schema-managed fields and normalizes enabled agent form', () => {
    const form = readACPConfig({
      acp: {
        agents: {
          codex: {
            enabled: true,
            setup_mode: 'api_key',
            managed: {
              api_key: 'sk-...cret',
              base_url: 'https://api.example.test/v1',
              extra: 'ignored',
            },
          },
        },
      },
    }, [codexProfile])

    expect(normalizeACPForm(form, [codexProfile])).toEqual({
      agents: {
        codex: {
          enabled: true,
          setup_mode: 'api_key',
          managed: {
            api_key: 'sk-...cret',
            base_url: 'https://api.example.test/v1',
          },
        },
      },
    })
  })

  it('initializes missing agent form entries from profile schema', () => {
    const form: ACPForm = { agents: {} }

    const agent = ensureACPAgentForm(form, codexProfile)
    agent.enabled = true
    agent.managed.api_key = 'sk-test'

    expect(form.agents.codex).toEqual({
      enabled: true,
      setup_mode: 'api_key',
      managed: {
        api_key: 'sk-test',
        base_url: '',
      },
    })
  })

  it('finds required setup fields and skips local/self modes', () => {
    const value: ACPForm = {
      agents: {
        codex: {
          enabled: true,
          setup_mode: 'api_key',
          managed: {
            api_key: '',
            base_url: 'https://api.example.test/v1',
          },
        },
      },
    }

    expect(findMissingRequiredACPField(value, [codexProfile])?.field.id).toBe('api_key')
    // `self` mode needs no managed credentials and is skipped per-agent.
    expect(findMissingRequiredACPField({
      agents: { codex: { enabled: true, setup_mode: 'self', managed: {} } },
    }, [codexProfile])).toBeNull()
    expect(findMissingRequiredManagedField(codexProfile, {}, 'self')).toBeNull()
  })

  it('validates Codex setup mode required fields', () => {
    expect(findMissingRequiredManagedField(codexProfile, {}, 'oauth')).toBeNull()
    expect(findMissingRequiredManagedField(codexProfile, {
      api_key: '',
    }, 'api_key')?.id).toBe('api_key')
  })

  it('validates Claude Code setup mode required fields', () => {
    expect(findMissingRequiredManagedField(claudeCodeProfile, {
      api_key: '',
    }, 'api_key')?.id).toBe('api_key')
    expect(findMissingRequiredManagedField(claudeCodeProfile, {
      api_key: 'sk-ant-test',
      oauth_token: '',
    }, 'api_key')).toBeNull()
    expect(findMissingRequiredManagedField(claudeCodeProfile, {
      oauth_token: '',
    }, 'oauth')?.id).toBe('oauth_token')
    expect(findMissingRequiredManagedField(claudeCodeProfile, {
      oauth_token: 'oauth-token',
    }, 'oauth')).toBeNull()
    expect(findMissingRequiredManagedField(claudeCodeProfile, {}, 'self')).toBeNull()
  })

  it('writes ACP metadata into the agents map', () => {
    const next = withACPMetadata({
      workspace: { backend: 'docker' },
    }, {
      agents: {
        codex: {
          enabled: true,
          setup_mode: 'self',
          managed: {
            api_key: '',
            base_url: '',
          },
        },
      },
    })

    expect(next).toEqual({
      workspace: { backend: 'docker' },
      acp: {
        agents: {
          codex: {
            enabled: true,
            setup_mode: 'self',
            managed: {
              api_key: '',
              base_url: '',
            },
          },
        },
      },
    })
  })

  it('serializes cleared sensitive managed fields as null for backend three-state PUT', () => {
    const next = withACPMetadata({
      acp: {
        agents: {
          codex: {
            enabled: true,
            setup_mode: 'api_key',
            managed: {
              api_key: 'sk-...cret',
              base_url: 'https://api.example.test/v1',
            },
          },
        },
      },
    }, {
      agents: {
        codex: {
          enabled: false,
          setup_mode: 'self',
          managed: {
            api_key: '',
            base_url: '',
          },
        },
      },
    }, [codexProfile])

    expect(next).toEqual({
      acp: {
        agents: {
          codex: {
            enabled: false,
            setup_mode: 'self',
            managed: {
              api_key: null,
              base_url: '',
            },
          },
        },
      },
    })
  })

  it('preserves masked sensitive managed fields when switching setup modes', () => {
    const next = withACPMetadata({
      acp: {
        agents: {
          codex: {
            enabled: true,
            setup_mode: 'api_key',
            managed: {
              api_key: 'sk-...cret',
              base_url: 'https://api.example.test/v1',
            },
          },
        },
      },
    }, {
      agents: {
        codex: {
          enabled: true,
          setup_mode: 'self',
          managed: {
            api_key: 'sk-...cret',
            base_url: 'https://api.example.test/v1',
          },
        },
      },
    }, [codexProfile])

    expect(next).toEqual({
      acp: {
        agents: {
          codex: {
            enabled: true,
            setup_mode: 'self',
            managed: {
              api_key: 'sk-...cret',
              base_url: 'https://api.example.test/v1',
            },
          },
        },
      },
    })
  })

  it('reads one agent config for ACP session creation validation', () => {
    const config = readACPAgentConfig({
      acp: {
        agents: {
          codex: {
            setup_mode: 'api_key',
            managed: {
              api_key: 'sk-...cret',
            },
          },
        },
      },
    }, 'CODEX')

    expect(config).toEqual({
      setupMode: 'api_key',
      managed: {
        api_key: 'sk-...cret',
      },
    })
  })
})
