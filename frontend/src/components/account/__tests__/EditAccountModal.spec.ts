import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'

const {
  updateAccountMock,
  checkMixedChannelRiskMock,
  getWebSearchEmulationConfigMock,
  listTLSFingerprintProfilesMock
} = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  getWebSearchEmulationConfigMock: vi.fn(),
  listTLSFingerprintProfilesMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isSimpleMode: true
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      update: updateAccountMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock
    },
    settings: {
      getWebSearchEmulationConfig: getWebSearchEmulationConfigMock
    },
    tlsFingerprintProfiles: {
      list: listTLSFingerprintProfilesMock
    }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('@/composables/useQuotaNotifyState', async () => {
  const { ref } = await vi.importActual<typeof import('vue')>('vue')

  return {
    useQuotaNotifyState: () => ({
      globalEnabled: ref(false),
      state: {
        daily: { enabled: false, threshold: null, thresholdType: 'percentage' },
        weekly: { enabled: false, threshold: null, thresholdType: 'percentage' },
        total: { enabled: false, threshold: null, thresholdType: 'percentage' }
      },
      loadGlobalState: vi.fn(),
      loadFromExtra: vi.fn(),
      writeToExtra: vi.fn(),
      reset: vi.fn()
    })
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const ModelWhitelistSelectorStub = defineComponent({
  name: 'ModelWhitelistSelector',
  props: {
    modelValue: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: `
    <div>
      <button
        type="button"
        data-testid="rewrite-to-snapshot"
        @click="$emit('update:modelValue', ['gpt-5.2-2025-12-11'])"
      >
        rewrite
      </button>
      <span data-testid="model-whitelist-value">
        {{ Array.isArray(modelValue) ? modelValue.join(',') : '' }}
      </span>
    </div>
  `
})

const SelectStub = defineComponent({
  name: 'Select',
  props: {
    modelValue: {
      type: [String, Number, Boolean],
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: `
    <select :value="modelValue" @change="$emit('update:modelValue', $event.target.value)">
      <option
        v-for="option in options"
        :key="typeof option === 'object' ? option.value : option"
        :value="typeof option === 'object' ? option.value : option"
      >
        {{ typeof option === 'object' ? option.label : option }}
      </option>
    </select>
  `
})

function buildAccount(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    name: 'OpenAI Key',
    notes: '',
    platform: 'openai',
    type: 'apikey',
    credentials: {
      api_key: 'sk-test',
      base_url: 'https://api.openai.com',
      model_mapping: {
        'gpt-5.2': 'gpt-5.2'
      }
    },
    extra: {},
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'active',
    group_ids: [],
    expires_at: null,
    auto_pause_on_expired: false,
    ...overrides
  } as any
}

function mountModal(account = buildAccount()) {
  return mount(EditAccountModal, {
    props: {
      show: true,
      account,
      proxies: [],
      groups: []
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        Select: SelectStub,
        Icon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: ModelWhitelistSelectorStub
      }
    }
  })
}

describe('EditAccountModal', () => {
  it('shows ctx_pool in OpenAI WS mode options', () => {
    getWebSearchEmulationConfigMock.mockResolvedValue({ enabled: false, providers: [] })
    listTLSFingerprintProfilesMock.mockResolvedValue([])
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()

    const wrapper = mountModal(buildAccount({ type: 'oauth', credentials: {} }))

    expect(wrapper.text()).toContain('admin.accounts.openai.wsModeCtxPool')
  })

  it('reopening the same account rehydrates the OpenAI whitelist from props', async () => {
    const account = buildAccount()
    getWebSearchEmulationConfigMock.mockResolvedValue({ enabled: false, providers: [] })
    listTLSFingerprintProfilesMock.mockResolvedValue([])
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2')

    await wrapper.get('[data-testid="rewrite-to-snapshot"]').trigger('click')
    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2-2025-12-11')

    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })

    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.model_mapping).toEqual({
      'gpt-5.2': 'gpt-5.2'
    })
  })

  it('preserves empty OpenAI OAuth model mapping when saving without editing models', async () => {
    const account = buildAccount({
      type: 'oauth',
      credentials: {
        access_token: 'oauth-token',
        base_url: 'https://api.openai.com'
      }
    })
    getWebSearchEmulationConfigMock.mockResolvedValue({ enabled: false, providers: [] })
    listTLSFingerprintProfilesMock.mockResolvedValue([])
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials).not.toHaveProperty('model_mapping')
  })

  it('uses OpenAI API Key upstream protocol to hide conflicting controls and submit explicit chat_completions payload', async () => {
    const account = buildAccount({
      extra: {
        openai_passthrough: true,
        openai_apikey_responses_websockets_v2_mode: 'passthrough',
        openai_apikey_responses_websockets_v2_enabled: true,
        openai_apikey_upstream_protocol: 'chat_completions'
      }
    })
    getWebSearchEmulationConfigMock.mockResolvedValue({ enabled: false, providers: [] })
    listTLSFingerprintProfilesMock.mockResolvedValue([])
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    expect(wrapper.get('#edit-openai-upstream-protocol-select').element.value).toBe('chat_completions')
    expect(wrapper.text()).toContain('admin.accounts.openai.upstreamProtocolChatCompletionsHint')
    expect(wrapper.text()).not.toContain('admin.accounts.openai.oauthPassthrough')
    expect(wrapper.text()).not.toContain('admin.accounts.openai.wsMode')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).toMatchObject({
      openai_apikey_upstream_protocol: 'chat_completions',
      openai_apikey_responses_websockets_v2_mode: 'off',
      openai_apikey_responses_websockets_v2_enabled: false
    })
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('openai_passthrough')
  })
})
