import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountBatchTestModal from '../AccountBatchTestModal.vue'

const { getAvailableModels, getBatchAvailableModels } = vi.hoisted(() => ({
  getAvailableModels: vi.fn(),
  getBatchAvailableModels: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getAvailableModels,
      getBatchAvailableModels
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'admin.accounts.bulkTest.startAction': '开始批量测试',
    'admin.accounts.bulkTest.testingAction': '测试中...',
    'admin.accounts.bulkTest.selectionInfo': '将测试{scope}。',
    'admin.accounts.bulkTest.selectedScopeLabel': '已选中的 {count} 个账号',
    'admin.accounts.bulkTest.filteredScopeLabel': '筛选结果中的 {count} 个账号',
    'admin.accounts.bulkTest.allScopeLabel': '全部 {count} 个账号',
    'common.close': '关闭'
  }

  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        const template = messages[key] || key
        if (!params) return template
        return Object.entries(params).reduce((text, [name, value]) => {
          return text.replaceAll(`{${name}}`, String(value))
        }, template)
      }
    })
  }
})

function createStreamResponse(lines: string[]) {
  const encoder = new TextEncoder()
  const chunks = lines.map((line) => encoder.encode(line))
  let index = 0

  return {
    ok: true,
    body: {
      getReader: () => ({
        read: vi.fn().mockImplementation(async () => {
          if (index < chunks.length) {
            return { done: false, value: chunks[index++] }
          }
          return { done: true, value: undefined }
        })
      })
    }
  } as Response
}

describe('AccountBatchTestModal', () => {
  beforeEach(() => {
    getAvailableModels.mockReset()
    getAvailableModels
      .mockResolvedValueOnce([
        { id: 'claude-3-5-sonnet', display_name: 'Claude 3.5 Sonnet' },
        { id: 'claude-3-7-sonnet', display_name: 'Claude 3.7 Sonnet' }
      ])
      .mockResolvedValueOnce([
        { id: 'claude-3-5-sonnet', display_name: 'Claude 3.5 Sonnet' },
        { id: 'claude-opus-4', display_name: 'Claude Opus 4' }
      ])

    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: vi.fn((key: string) => (key === 'auth_token' ? 'test-token' : null)),
        setItem: vi.fn(),
        removeItem: vi.fn(),
        clear: vi.fn()
      },
      configurable: true
    })

    global.fetch = vi.fn()
      .mockResolvedValueOnce(createStreamResponse([
        'data: {"type":"test_start"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ]))
      .mockResolvedValueOnce(createStreamResponse([
        'data: {"type":"test_start"}\n',
        'data: {"type":"error","error":"boom"}\n'
      ])) as any
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('批量测试支持选择共同模型，并默认携带“只回复OK”提示词', async () => {
    const wrapper = mount(AccountBatchTestModal, {
      props: {
        show: true,
        accountIds: [11, 22]
      },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Select: {
            name: 'Select',
            props: ['modelValue', 'options'],
            emits: ['update:modelValue'],
            template: '<select class="select-stub" :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="option in options" :key="String(option.value)" :value="option.value">{{ option.label }}</option></select>'
          },
          TextArea: {
            name: 'TextArea',
            props: ['modelValue'],
            emits: ['update:modelValue'],
            template: '<textarea class="textarea-stub" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
          },
          Icon: true
        }
      }
    })

    await flushPromises()

    const promptInput = wrapper.find('textarea.textarea-stub')
    expect((promptInput.element as HTMLTextAreaElement).value).toBe('只回复OK')

    const concurrencyInput = wrapper.find('input[type="number"]')
    expect((concurrencyInput.element as HTMLInputElement).value).toBe('5')

    const modelSelect = wrapper.findComponent({ name: 'Select' })
    modelSelect.vm.$emit('update:modelValue', 'claude-3-5-sonnet')
    await flushPromises()
    await promptInput.setValue('custom batch prompt')

    const startButton = wrapper.findAll('button').find((button) => button.text().includes('开始批量测试'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(getBatchAvailableModels).toHaveBeenCalledTimes(1)
    expect(getBatchAvailableModels).toHaveBeenCalledWith([11, 22])
    expect(getAvailableModels).not.toHaveBeenCalled()

    expect(global.fetch).toHaveBeenCalledTimes(2)
    expect((global.fetch as any).mock.calls[0][0]).toBe('/api/v1/admin/accounts/11/test')
    expect((global.fetch as any).mock.calls[1][0]).toBe('/api/v1/admin/accounts/22/test')
    expect(JSON.parse((global.fetch as any).mock.calls[0][1].body)).toEqual({ model_id: 'claude-3-5-sonnet', prompt: 'custom batch prompt' })
    expect(JSON.parse((global.fetch as any).mock.calls[1][1].body)).toEqual({ model_id: 'claude-3-5-sonnet', prompt: 'custom batch prompt' })

    const completedEvents = wrapper.emitted('completed')
    expect(completedEvents).toBeTruthy()
    expect(completedEvents![0][0]).toMatchObject({
      total: 2,
      success: 1,
      failed: 1,
      failedIds: [22]
    })
    expect(completedEvents![0][0].errors).toEqual([
      {
        account_id: 22,
        error: 'boom'
      }
    ])
  })

  it('根据范围展示批量测试对象说明', async () => {
    getAvailableModels.mockResolvedValue([])

    const wrapper = mount(AccountBatchTestModal, {
      props: {
        show: true,
        accountIds: [11, 22, 33],
        scope: 'filtered'
      },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Select: true,
          TextArea: true,
          Icon: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('将测试筛选结果中的 3 个账号。')
  })
})
