// @vitest-environment jsdom
/* eslint-disable vue/one-component-per-file */

import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ToolCallBlock } from '@/store/chat-list'

const respondUserInput = vi.fn()
const respondToolApproval = vi.fn()

const ButtonStub = defineComponent({
  name: 'UiButtonStub',
  inheritAttrs: false,
  setup(_, { attrs, slots }) {
    return () => h('button', attrs, slots.default?.())
  },
})

const InputStub = defineComponent({
  name: 'UiInputStub',
  inheritAttrs: false,
  setup(_, { attrs }) {
    return () => h('input', attrs)
  },
})

const CodeBlockStub = defineComponent({
  name: 'CodeBlockStub',
  inheritAttrs: false,
  props: {
    code: { type: String, required: true },
    lang: { type: String, default: '' },
    filename: { type: String, default: '' },
  },
  setup(props, { attrs }) {
    return () => h('pre', {
      ...attrs,
      'data-code-lang': props.lang,
      'data-code-filename': props.filename,
    }, props.code)
  },
})

const FileDetailStub = defineComponent({
  name: 'FileDetailStub',
  props: {
    block: { type: Object, required: true },
  },
  setup(props) {
    return () => h('pre', { 'data-file-detail': '' }, String(
      (props.block as ToolCallBlock).input
      && ((props.block as ToolCallBlock).input as Record<string, unknown>).new_text,
    ))
  },
})

const WriteDetailStub = defineComponent({
  name: 'WriteDetailStub',
  props: {
    block: { type: Object, required: true },
  },
  setup(props) {
    return () => {
      const block = props.block as ToolCallBlock
      const input = block.input as Record<string, unknown>
      return h(CodeBlockStub, {
        code: String(input.content ?? ''),
        filename: String(input.path ?? ''),
      })
    }
  },
})

vi.mock('@felinic/ui', () => ({
  Button: ButtonStub,
  Input: InputStub,
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('@/store/chat-list', () => ({
  useChatStore: () => ({ respondUserInput, respondToolApproval }),
}))

vi.mock('../composables/useChatViewContext', () => ({
  useChatViewTarget: () => ref({ botId: 'bot-1', sessionId: 'session-1', viewId: 'view-1' }),
}))

vi.mock('./code-block.vue', () => ({
  default: CodeBlockStub,
}))

vi.mock('./tool-call-registry', () => ({
  getToolDisplay: (block: ToolCallBlock) => {
    const input = block.input as Record<string, unknown>
    const path = String(input?.path ?? '')
    return {
      actionKey: block.toolName,
      target: block.toolName === 'exec'
        ? String(input?.command ?? '')
        : path.split('/').pop() ?? '',
      fullTarget: path,
      detail: block.toolName === 'write'
        ? WriteDetailStub
        : block.toolName === 'edit'
          ? FileDetailStub
          : undefined,
    }
  },
}))

describe('chat decision forms', () => {
  let app: ReturnType<typeof createApp> | undefined
  let root: HTMLDivElement | undefined

  beforeEach(() => {
    respondUserInput.mockReset()
    respondToolApproval.mockReset()
  })

  afterEach(() => {
    app?.unmount()
    root?.remove()
    app = undefined
    root = undefined
  })

  async function mount(component: Parameters<typeof createApp>[0], props: Record<string, unknown>) {
    root = document.createElement('div')
    document.body.append(root)
    app = createApp(component, props)
    app.config.globalProperties.$t = (key: string) => key
    app.mount(root)
    await nextTick()
    return root
  }

  it('does not ask the parent to reveal the composer when an option is selected', async () => {
    const focusComposer = vi.fn()
    const ChatUserInputForm = (await import('./chat-user-input-form.vue')).default
    const el = await mount(ChatUserInputForm, {
      userInput: {
        user_input_id: 'input-1',
        status: 'pending',
        questions: [{
          id: 'q1',
          text: 'Choose one',
          kind: 'single_select',
          options: [{ id: 'q1.o1', label: 'One' }, { id: 'q1.o2', label: 'Two' }],
        }],
      },
      onFocusComposer: focusComposer,
    })

    const firstOption = el.querySelector<HTMLButtonElement>('[role="radio"]')
    expect(firstOption).not.toBeNull()
    firstOption!.click()
    await nextTick()

    expect(firstOption!.getAttribute('aria-checked')).toBe('true')
    expect(focusComposer).not.toHaveBeenCalled()
    expect(respondUserInput).not.toHaveBeenCalled()
  })

  it('sends approvals from the composer-style decision panel', async () => {
    const ChatToolApprovalForm = (await import('./chat-tool-approval-form.vue')).default
    const block: ToolCallBlock = {
      id: 1,
      type: 'tool',
      name: 'exec',
      input: { command: 'git status' },
      tool_call_id: 'tool-call-1',
      running: false,
      toolCallId: 'tool-call-1',
      toolName: 'exec',
      result: null,
      done: false,
      approval: { approval_id: 'approval-1', short_id: 3, status: 'pending', can_approve: true },
    }
    const el = await mount(ChatToolApprovalForm, { block })
    const buttons = [...el.querySelectorAll<HTMLButtonElement>('button')]
    const preview = el.querySelector<HTMLElement>('[data-slot="tool-approval-preview"]')
    const code = preview?.querySelector<HTMLElement>('pre')

    expect(el.querySelector('[data-slot="input-group"]')).not.toBeNull()
    expect(el.querySelector('[data-slot="tool-approval-title"]')?.textContent).toBe('bots.toolApproval.toolNames.exec')
    expect(code?.textContent).toBe('git status')
    expect(code?.dataset.codeLang).toBe('bash')
    expect(buttons).toHaveLength(2)
    buttons[0]!.click()

    expect(respondToolApproval).toHaveBeenCalledWith(
      block.approval,
      'approve',
      { botId: 'bot-1', sessionId: 'session-1', viewId: 'view-1' },
    )
  })

  it('renders write content as filename-aware code', async () => {
    const ChatToolApprovalForm = (await import('./chat-tool-approval-form.vue')).default
    const block: ToolCallBlock = {
      id: 2,
      type: 'tool',
      name: 'write',
      input: { path: '/workspace/config/app.ts', content: 'export const enabled = true' },
      tool_call_id: 'tool-call-2',
      running: false,
      toolCallId: 'tool-call-2',
      toolName: 'write',
      result: null,
      done: false,
      approval: { approval_id: 'approval-2', short_id: 4, status: 'pending', can_approve: true },
    }
    const el = await mount(ChatToolApprovalForm, { block })
    const preview = el.querySelector<HTMLElement>('[data-slot="tool-approval-preview"]')
    const code = preview?.querySelector<HTMLElement>('pre')

    expect(el.querySelector('[data-slot="tool-approval-title"]')?.textContent)
      .toContain('bots.toolApproval.toolNames.write')
    expect(el.querySelector('[data-slot="tool-approval-title"]')?.textContent)
      .toContain('app.ts')
    expect(code?.textContent).toBe('export const enabled = true')
    expect(code?.dataset.codeFilename).toBe('/workspace/config/app.ts')
  })

  it('reuses the existing diff detail for edit approvals', async () => {
    const ChatToolApprovalForm = (await import('./chat-tool-approval-form.vue')).default
    const block: ToolCallBlock = {
      id: 3,
      type: 'tool',
      name: 'edit',
      input: { path: '/workspace/config/app.ts', old_text: 'false', new_text: 'true' },
      tool_call_id: 'tool-call-3',
      running: false,
      toolCallId: 'tool-call-3',
      toolName: 'edit',
      result: null,
      done: false,
      approval: { approval_id: 'approval-3', short_id: 5, status: 'pending', can_approve: true },
    }
    const el = await mount(ChatToolApprovalForm, { block })

    expect(el.querySelector('[data-file-detail]')?.textContent).toBe('true')
  })
})
