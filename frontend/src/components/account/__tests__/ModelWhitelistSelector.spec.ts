import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showInfo: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'admin.accounts.modelCount') {
          return `${key}:${params?.count ?? ''}`
        }
        return key
      }
    })
  }
})

const IconStub = defineComponent({
  name: 'Icon',
  template: '<span />'
})

const ModelIconStub = defineComponent({
  name: 'ModelIcon',
  template: '<span />'
})

describe('ModelWhitelistSelector', () => {
  it('显式 availableModels 会覆盖平台默认模型列表', async () => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: ['gpt-5.4'],
        platform: 'openai',
        availableModels: ['gpt-5.4', 'gpt-5.4-mini', 'gpt-5.3-codex', 'gpt-5.2']
      },
      global: {
        stubs: {
          Icon: IconStub,
          ModelIcon: ModelIconStub
        }
      }
    })

    await wrapper.get('div.cursor-pointer').trigger('click')

    const text = wrapper.text()
    expect(text).toContain('gpt-5.4-mini')
    expect(text).toContain('gpt-5.3-codex')
    expect(text).toContain('gpt-5.2')
    expect(text).not.toContain('gpt-4o')
    expect(text).not.toContain('gpt-5.4-nano')
  })
})
