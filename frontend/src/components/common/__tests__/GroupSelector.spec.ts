import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import GroupSelector from '../GroupSelector.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'common.selectedCount') {
          return `${key}:${String(params?.count ?? '')}`
        }
        return key
      },
    }),
  }
})

describe('GroupSelector', () => {
  it('supports custom label and platform-filtered existing groups', async () => {
    const wrapper = mount(GroupSelector, {
      props: {
        modelValue: [],
        label: 'admin.settings.openaiWarmPool.startupGroups',
        groups: [
          {
            id: 11,
            name: 'OpenAI Group',
            platform: 'openai',
            subscription_type: 'shared',
            rate_multiplier: 1,
            account_count: 3,
          },
          {
            id: 22,
            name: 'Anthropic Group',
            platform: 'anthropic',
            subscription_type: 'shared',
            rate_multiplier: 1,
            account_count: 2,
          },
        ],
        platform: 'openai',
      },
      global: {
        stubs: {
          GroupBadge: {
            props: ['name'],
            template: '<span>{{ name }}</span>',
          },
        },
      },
    })

    expect(wrapper.text()).toContain('admin.settings.openaiWarmPool.startupGroups')
    expect(wrapper.text()).toContain('common.selectedCount:0')

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(1)

    await checkboxes[0].setValue(true)

    expect(wrapper.emitted('update:modelValue')).toEqual([[[11]]])
  })
})
