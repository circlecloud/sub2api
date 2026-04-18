import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'admin.accounts.openai.upstreamProtocol': '上游协议',
    'admin.accounts.openai.upstreamProtocolDesc': '仅对 OpenAI API Key 账号生效。',
    'admin.accounts.openai.upstreamProtocolResponses': 'Responses',
    'admin.accounts.openai.upstreamProtocolChatCompletions': 'Chat Completions',
    'admin.accounts.openai.upstreamProtocolChatCompletionsHint':
      'Chat Completions 模式下，/v1/chat/completions 将直连上游；/v1/responses 仅支持基础无状态兼容。previous_response_id、store=true、include、reasoning.summary、非 function tools、原生 input 项、/v1/messages、/v1/responses/compact 和 Responses WS 不支持。'
  }

  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key
    })
  }
})

import OpenAIUpstreamProtocolSection from '../OpenAIUpstreamProtocolSection.vue'
import zhLocale from '@/i18n/locales/zh'
import enLocale from '@/i18n/locales/en'

const SelectStub = defineComponent({
  name: 'Select',
  props: {
    modelValue: {
      type: String,
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    },
    id: {
      type: String,
      default: ''
    }
  },
  emits: ['update:modelValue'],
  template: `
    <select :id="id" :value="modelValue" @change="$emit('update:modelValue', $event.target.value)">
      <option v-for="option in options" :key="String(option.value)" :value="String(option.value)">{{ option.label }}</option>
    </select>
  `
})

describe('OpenAIUpstreamProtocolSection', () => {
  it('uses simplified locale labels for the upstream protocol dropdown', () => {
    expect(zhLocale.admin.accounts.openai.upstreamProtocolResponses).toBe('Responses')
    expect(zhLocale.admin.accounts.openai.upstreamProtocolChatCompletions).toBe('Chat Completions')
    expect(enLocale.admin.accounts.openai.upstreamProtocolResponses).toBe('Responses')
    expect(enLocale.admin.accounts.openai.upstreamProtocolChatCompletions).toBe('Chat Completions')
  })

  it('uses simplified dropdown labels and renders the control in the same left-right layout as WS mode', () => {
    const wrapper = mount(OpenAIUpstreamProtocolSection, {
      props: {
        modelValue: 'responses',
        idPrefix: 'test-openai-upstream'
      },
      global: {
        stubs: {
          Select: SelectStub
        }
      }
    })

    const select = wrapper.get('#test-openai-upstream-select')
    const options = select.findAll('option').map((option) => option.text())
    expect(options).toEqual(['Responses', 'Chat Completions'])

    const layoutRow = wrapper.find('.flex.items-center.justify-between.gap-4')
    expect(layoutRow.exists()).toBe(true)

    const rootText = wrapper.text()
    expect(rootText).toContain('上游协议')
    expect(rootText).toContain('仅对 OpenAI API Key 账号生效。')

    const rootHtml = wrapper.html()
    expect(rootHtml.indexOf('仅对 OpenAI API Key 账号生效。')).toBeLessThan(rootHtml.indexOf('test-openai-upstream-select'))
  })

  it('renders compatibility hint below the select in chat_completions mode', () => {
    const wrapper = mount(OpenAIUpstreamProtocolSection, {
      props: {
        modelValue: 'chat_completions',
        idPrefix: 'test-openai-upstream'
      },
      global: {
        stubs: {
          Select: SelectStub
        }
      }
    })

    const hint = wrapper.get('[data-testid="openai-upstream-protocol-chat-completions-hint"]')
    expect(hint.text()).toContain('/v1/responses 仅支持基础无状态兼容')

    const rootHtml = wrapper.html()
    expect(rootHtml.indexOf('test-openai-upstream-select')).toBeLessThan(rootHtml.indexOf('/v1/responses 仅支持基础无状态兼容'))
  })
})
