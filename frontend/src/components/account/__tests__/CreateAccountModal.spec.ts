import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const {
  createAccountMock,
  checkMixedChannelRiskMock,
  getWebSearchEmulationConfigMock,
  listTLSFingerprintProfilesMock,
} = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  getWebSearchEmulationConfigMock: vi.fn(),
  listTLSFingerprintProfilesMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
    showWarning: vi.fn()
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
      create: createAccountMock,
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
      writeToExtra: vi.fn()
    })
  }
})

function createOAuthComposableMock() {
  return {
    authUrl: ref(''),
    sessionId: ref(''),
    oauthState: ref(''),
    state: ref(''),
    loading: ref(false),
    error: ref(''),
    resetState: vi.fn(),
    generateAuthUrl: vi.fn(),
    buildCredentials: vi.fn(() => ({})),
    buildExtraInfo: vi.fn(() => ({})),
    exchangeAuthCode: vi.fn(),
    validateRefreshToken: vi.fn(),
    parseSessionKeys: vi.fn(() => [])
  }
}

vi.mock('@/composables/useAccountOAuth', () => ({
  useAccountOAuth: () => createOAuthComposableMock()
}))

vi.mock('@/composables/useOpenAIOAuth', () => ({
  useOpenAIOAuth: () => createOAuthComposableMock()
}))

vi.mock('@/composables/useGeminiOAuth', () => ({
  useGeminiOAuth: () => createOAuthComposableMock()
}))

vi.mock('@/composables/useAntigravityOAuth', () => ({
  useAntigravityOAuth: () => createOAuthComposableMock()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

import CreateAccountModal from '../CreateAccountModal.vue'

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

const SelectStub = defineComponent({
  name: 'Select',
  inheritAttrs: false,
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
    <select
      v-bind="$attrs"
      :value="modelValue"
      @change="$emit('update:modelValue', $event.target.value)"
    >
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

function mountModal() {
  return mount(CreateAccountModal, {
    props: {
      show: true,
      proxies: [],
      groups: []
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        Select: SelectStub,
        Icon: true,
        GroupSelector: true,
        ModelWhitelistSelector: true,
        QuotaLimitCard: true,
        AccountRuntimeSettingsFields: true,
        OpenAIOAuthDefaultsFields: true,
        OAuthAuthorizationFlow: true
      }
    }
  })
}

function findButtonByText(wrapper: ReturnType<typeof mount>, text: string) {
  const button = wrapper.findAll('button').find((node) => node.text().includes(text))
  expect(button, `button containing ${text}`).toBeTruthy()
  return button!
}

describe('CreateAccountModal', () => {
  beforeEach(() => {
    createAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    getWebSearchEmulationConfigMock.mockReset()
    listTLSFingerprintProfilesMock.mockReset()

    createAccountMock.mockResolvedValue({ id: 1 })
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    getWebSearchEmulationConfigMock.mockResolvedValue({ enabled: false, providers: [] })
    listTLSFingerprintProfilesMock.mockResolvedValue([])
  })

  it('only shows OpenAI API Key upstream protocol selector and writes explicit responses payload after switching back from chat_completions', async () => {
    const wrapper = mountModal()

    await findButtonByText(wrapper, 'OpenAI').trigger('click')

    expect(wrapper.find('#create-openai-upstream-protocol-select').exists()).toBe(false)

    await findButtonByText(wrapper, 'API Key').trigger('click')

    expect(wrapper.find('#create-openai-upstream-protocol-select').exists()).toBe(true)

    await wrapper.get('#create-openai-upstream-protocol-select').setValue('chat_completions')

    expect(wrapper.text()).toContain('admin.accounts.openai.upstreamProtocolChatCompletionsHint')
    expect(wrapper.text()).not.toContain('admin.accounts.openai.oauthPassthrough')
    expect(wrapper.text()).not.toContain('admin.accounts.openai.wsMode')

    await wrapper.get('#create-openai-upstream-protocol-select').setValue('responses')

    expect(wrapper.text()).not.toContain('admin.accounts.openai.upstreamProtocolChatCompletionsHint')
    expect(wrapper.text()).toContain('admin.accounts.openai.oauthPassthrough')
    expect(wrapper.text()).toContain('admin.accounts.openai.wsMode')

    await wrapper.get('form#create-account-form input[type="text"]').setValue('OpenAI API Key')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('sk-test')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]).toMatchObject({
      platform: 'openai',
      type: 'apikey',
      extra: {
        openai_apikey_upstream_protocol: 'responses'
      }
    })
  })
})
