import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import SettingsView from '../SettingsView.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'

const {
  getSettings,
  updateSettings,
  getAllGroups,
  getAdminApiKey,
  getOverloadCooldownSettings,
  getStreamTimeoutSettings,
  getRectifierSettings,
  getBetaPolicySettings,
  showSuccess,
  showError,
  fetchPublicSettings,
  fetchAdminSettings,
} = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  getAllGroups: vi.fn(),
  getAdminApiKey: vi.fn(),
  getOverloadCooldownSettings: vi.fn(),
  getStreamTimeoutSettings: vi.fn(),
  getRectifierSettings: vi.fn(),
  getBetaPolicySettings: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  fetchPublicSettings: vi.fn(),
  fetchAdminSettings: vi.fn(),
}))

vi.mock('@/api', () => ({
  adminAPI: {
    settings: {
      getSettings,
      updateSettings,
      getAdminApiKey,
      regenerateAdminApiKey: vi.fn(),
      deleteAdminApiKey: vi.fn(),
      getOverloadCooldownSettings,
      updateOverloadCooldownSettings: vi.fn(),
      getStreamTimeoutSettings,
      updateStreamTimeoutSettings: vi.fn(),
      getRectifierSettings,
      updateRectifierSettings: vi.fn(),
      getBetaPolicySettings,
      updateBetaPolicySettings: vi.fn(),
      testSmtpConnection: vi.fn(),
      sendTestEmail: vi.fn(),
    },
    groups: {
      getAll: getAllGroups,
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showSuccess,
    showError,
    fetchPublicSettings,
  }),
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    fetch: fetchAdminSettings,
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn(),
  }),
}))

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

const baseSettings = {
  registration_enabled: true,
  email_verify_enabled: false,
  registration_email_suffix_whitelist: [],
  promo_code_enabled: true,
  password_reset_enabled: false,
  frontend_url: '',
  invitation_code_enabled: false,
  totp_enabled: false,
  totp_encryption_key_configured: false,
  default_balance: 0,
  default_concurrency: 1,
  default_subscriptions: [],
  site_name: 'Sub2API',
  site_logo: '',
  site_subtitle: '',
  api_base_url: '',
  contact_info: '',
  doc_url: '',
  home_content: '',
  hide_ccs_import_button: false,
  purchase_subscription_enabled: false,
  purchase_subscription_url: '',
  backend_mode_enabled: false,
  custom_menu_items: [],
  custom_endpoints: [],
  smtp_host: '',
  smtp_port: 587,
  smtp_username: '',
  smtp_password_configured: false,
  smtp_from_email: '',
  smtp_from_name: '',
  smtp_use_tls: true,
  turnstile_enabled: false,
  turnstile_site_key: '',
  turnstile_secret_key_configured: false,
  geetest_enabled: false,
  geetest_captcha_id: '',
  geetest_captcha_key_configured: false,
  geetest_popup_on_submit: false,
  linuxdo_connect_enabled: false,
  linuxdo_connect_client_id: '',
  linuxdo_connect_client_secret_configured: false,
  linuxdo_connect_redirect_url: '',
  enable_model_fallback: false,
  fallback_model_anthropic: 'claude-3-5-sonnet-20241022',
  fallback_model_openai: 'gpt-4o',
  fallback_model_gemini: 'gemini-2.5-pro',
  fallback_model_antigravity: 'gemini-2.5-pro',
  enable_identity_patch: true,
  identity_patch_prompt: '',
  ops_monitoring_enabled: true,
  ops_realtime_monitoring_enabled: true,
  ops_query_mode_default: 'auto',
  ops_metrics_interval_seconds: 60,
  min_claude_code_version: '',
  max_claude_code_version: '',
  allow_ungrouped_key_scheduling: false,
  enable_fingerprint_unification: true,
  enable_metadata_passthrough: false,
  enable_cch_signing: false,
  enable_openai_stream_rectifier: true,
  openai_stream_response_header_rectifier_timeouts: [8, 10, 12],
  openai_stream_first_token_rectifier_timeouts: [5, 8, 10],
  openai_warm_pool_enabled: true,
  openai_warm_pool_bucket_target_size: 10,
  openai_warm_pool_bucket_refill_below: 3,
  openai_warm_pool_bucket_sync_fill_min: 1,
  openai_warm_pool_bucket_entry_ttl_seconds: 30,
  openai_warm_pool_bucket_refill_cooldown_seconds: 15,
  openai_warm_pool_bucket_refill_interval_seconds: 30,
  openai_warm_pool_global_target_size: 30,
  openai_warm_pool_global_refill_below: 10,
  openai_warm_pool_global_entry_ttl_seconds: 300,
  openai_warm_pool_global_refill_cooldown_seconds: 60,
  openai_warm_pool_global_refill_interval_seconds: 300,
  openai_warm_pool_network_error_pool_size: 3,
  openai_warm_pool_network_error_entry_ttl_seconds: 120,
  openai_warm_pool_probe_max_candidates: 24,
  openai_warm_pool_probe_concurrency: 4,
  openai_warm_pool_probe_timeout_seconds: 15,
  openai_warm_pool_probe_failure_cooldown_seconds: 120,
  openai_warm_pool_startup_group_ids: [],
  openai_usage_probe_method: 'responses',
}

describe('SettingsView openai warm pool startup groups', () => {
  const mountSettingsView = () => mount(SettingsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: { template: '<span />' },
        Select: true,
        GroupBadge: { props: ['name'], template: '<span>{{ name }}</span>' },
        GroupOptionItem: true,
        Toggle: {
          props: ['modelValue'],
          emits: ['update:modelValue'],
          template: '<input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" />',
        },
        ImageUpload: true,
        PositiveIntegerTagsInput: true,
        BackupSettings: true,
      },
    },
  })

  beforeEach(() => {
    getSettings.mockReset()
    updateSettings.mockReset()
    getAllGroups.mockReset()
    getAdminApiKey.mockReset()
    getOverloadCooldownSettings.mockReset()
    getStreamTimeoutSettings.mockReset()
    getRectifierSettings.mockReset()
    getBetaPolicySettings.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    fetchPublicSettings.mockReset()
    fetchAdminSettings.mockReset()

    getSettings.mockResolvedValue({ ...baseSettings })
    updateSettings.mockImplementation(async (payload: Record<string, unknown>) => ({
      ...baseSettings,
      ...payload,
      totp_encryption_key_configured: false,
      smtp_password_configured: false,
      turnstile_secret_key_configured: false,
      geetest_captcha_key_configured: false,
      linuxdo_connect_client_secret_configured: false,
    }))
    getAllGroups.mockResolvedValue([
      {
        id: 101,
        name: 'OpenAI Group',
        description: 'openai',
        platform: 'openai',
        subscription_type: 'shared',
        rate_multiplier: 1,
        account_count: 3,
        status: 'active',
      },
      {
        id: 202,
        name: 'Anthropic Group',
        description: 'anthropic',
        platform: 'anthropic',
        subscription_type: 'shared',
        rate_multiplier: 1,
        account_count: 2,
        status: 'active',
      },
    ])
    getAdminApiKey.mockResolvedValue({ exists: false, masked_key: '' })
    getOverloadCooldownSettings.mockResolvedValue({ enabled: true, cooldown_minutes: 10 })
    getStreamTimeoutSettings.mockResolvedValue({
      enabled: true,
      action: 'temp_unsched',
      temp_unsched_minutes: 5,
      threshold_count: 3,
      threshold_window_minutes: 10,
    })
    getRectifierSettings.mockResolvedValue({
      enabled: true,
      thinking_signature_enabled: true,
      thinking_budget_enabled: true,
      apikey_signature_enabled: false,
      apikey_signature_patterns: [],
    })
    getBetaPolicySettings.mockResolvedValue({ rules: [] })
    fetchPublicSettings.mockResolvedValue(undefined)
    fetchAdminSettings.mockResolvedValue(undefined)
  })

  it('suppresses browser autofill across the settings form', async () => {
    const wrapper = mountSettingsView()

    await flushPromises()
    await flushPromises()

    const form = wrapper.get('form')
    expect(form.attributes('autocomplete')).toBe('off')
    expect(form.attributes('data-lpignore')).toBe('true')
    expect(form.attributes('data-1p-ignore')).toBe('true')
    expect(form.attributes('data-form-type')).toBe('other')

    const fakeUsername = wrapper.get('input[name="settings-autofill-username"]')
    expect(fakeUsername.attributes('autocomplete')).toBe('username')

    const fakePassword = wrapper.get('input[name="settings-autofill-password"]')
    expect(fakePassword.attributes('autocomplete')).toBe('current-password')

    const purchaseUrlInput = wrapper.get('input[placeholder="admin.settings.purchase.urlPlaceholder"]')
    expect(purchaseUrlInput.attributes('autocomplete')).toBe('off')
    expect(purchaseUrlInput.attributes('data-lpignore')).toBe('true')
    expect(purchaseUrlInput.attributes('data-1p-ignore')).toBe('true')
  })

  it('renders a fixed shell around the horizontally scrollable settings tabs', async () => {
    const wrapper = mountSettingsView()

    await flushPromises()
    await flushPromises()

    const shell = wrapper.find('.settings-tabs-shell')
    expect(shell.exists()).toBe(true)

    const scroll = shell.find('.settings-tabs-scroll')
    expect(scroll.exists()).toBe(true)
    expect(scroll.find('nav.settings-tabs').exists()).toBe(true)
  })

  it('normalizes null startup warm groups from settings without crashing', async () => {
    getSettings.mockResolvedValueOnce({
      ...baseSettings,
      openai_warm_pool_startup_group_ids: null,
    })

    const wrapper = mountSettingsView()

    await flushPromises()
    await flushPromises()

    const openaiTab = wrapper.findAll('button').find((node) => node.text().includes('admin.settings.tabs.openai'))
    expect(openaiTab).toBeTruthy()
    await openaiTab!.trigger('click')
    await flushPromises()

    const selector = wrapper.findComponent(GroupSelector)
    expect(selector.exists()).toBe(true)
    expect(selector.text()).toContain('common.selectedCount:0')

    const checkboxes = selector.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(1)
  })

  it('saves selected existing OpenAI groups as startup warm groups', async () => {
    const wrapper = mount(SettingsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: { template: '<span />' },
          Select: true,
          GroupBadge: { props: ['name'], template: '<span>{{ name }}</span>' },
          GroupOptionItem: true,
          Toggle: {
            props: ['modelValue'],
            emits: ['update:modelValue'],
            template: '<input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" />',
          },
          ImageUpload: true,
          PositiveIntegerTagsInput: true,
          BackupSettings: true,
        },
      },
    })

    await flushPromises()
    await flushPromises()

    const openaiTab = wrapper.findAll('button').find((node) => node.text().includes('admin.settings.tabs.openai'))
    expect(openaiTab).toBeTruthy()
    await openaiTab!.trigger('click')
    await flushPromises()

    const selector = wrapper.findComponent(GroupSelector)
    expect(selector.exists()).toBe(true)
    expect(selector.text()).toContain('admin.settings.openaiWarmPool.startupGroups')

    const checkboxes = selector.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(1)
    await checkboxes[0].setValue(true)

    const saveButton = wrapper.findAll('button').find((node) => node.text().includes('admin.settings.saveSettings'))
    expect(saveButton).toBeTruthy()
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledTimes(1)
    expect(updateSettings.mock.calls[0][0]).toMatchObject({
      openai_warm_pool_startup_group_ids: [101],
    })
  })

  it('submits openai usage probe method from the openai tab', async () => {
    const wrapper = mountSettingsView()

    await flushPromises()
    await flushPromises()

    const openaiTab = wrapper.findAll('button').find((node) => node.text().includes('admin.settings.tabs.openai'))
    expect(openaiTab).toBeTruthy()
    await openaiTab!.trigger('click')
    await flushPromises()

    const methodSelect = wrapper.findAll('select').find((node) => node.find('option[value="wham"]').exists())
    expect(methodSelect).toBeTruthy()
    await methodSelect!.setValue('wham')

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledTimes(1)
    expect(updateSettings.mock.calls[0][0]).toMatchObject({
      openai_usage_probe_method: 'wham',
    })
  })

  it('submits geetest captcha key when the field has a value', async () => {
    const wrapper = mountSettingsView()

    await flushPromises()
    await flushPromises()

    const setupState = wrapper.vm.$.setupState as any
    setupState.form.geetest_enabled = true
    setupState.form.geetest_captcha_key = 'geetest-secret'

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledTimes(1)
    expect(updateSettings.mock.calls[0][0]).toMatchObject({
      geetest_captcha_key: 'geetest-secret',
    })
  })
})
