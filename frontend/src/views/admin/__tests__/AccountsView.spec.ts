import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const tableLoaderState = vi.hoisted(() => ({
  items: [] as any[],
  params: {
    platform: '',
    type: '',
    status: '',
    privacy_mode: '',
    group: '1,2,3',
    group_exclude: '4,5',
    group_match: 'exact',
    search: '',
    sort_by: 'id',
    sort_order: 'desc',
    last_used_filter: '',
    last_used_start_date: '',
    last_used_end_date: '',
  },
  pagination: {
    page: 1,
    page_size: 20,
    total: 1,
    pages: 1,
  },
}))

const {
  list,
  listWithEtag,
  previewBulkUpdateTargets,
  resolveBulkUpdateTargets,
  getBatchTodayStats,
  getUsage,
  getAllProxies,
  getAllGroups,
  exportData,
  refreshCredentials,
  setSchedulable,
  setPrivacy,
  recoverState,
  showError,
  showSuccess,
  baseLoad,
  baseReload,
  baseDebouncedReload,
  handlePageChange,
  handlePageSizeChange,
} = vi.hoisted(() => {
  vi.stubGlobal('localStorage', {
    getItem: vi.fn(() => null),
    setItem: vi.fn(),
    removeItem: vi.fn(),
    clear: vi.fn(),
  })

  return {
    list: vi.fn(),
    listWithEtag: vi.fn(),
    previewBulkUpdateTargets: vi.fn(),
    resolveBulkUpdateTargets: vi.fn(),
    getBatchTodayStats: vi.fn(),
    getUsage: vi.fn(),
    getAllProxies: vi.fn(),
    getAllGroups: vi.fn(),
    exportData: vi.fn(),
    refreshCredentials: vi.fn(),
    setSchedulable: vi.fn(),
    setPrivacy: vi.fn(),
    recoverState: vi.fn(),
    showError: vi.fn(),
    showSuccess: vi.fn(),
    baseLoad: vi.fn(),
    baseReload: vi.fn(),
    baseDebouncedReload: vi.fn(),
    handlePageChange: vi.fn(),
    handlePageSizeChange: vi.fn(),
  }
})

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list,
      listWithEtag,
      previewBulkUpdateTargets,
      resolveBulkUpdateTargets,
      getBatchTodayStats,
      getUsage,
      exportData,
      refreshCredentials,
      setSchedulable,
      setPrivacy,
      recoverState,
    },
    proxies: {
      getAll: getAllProxies,
    },
    groups: {
      getAll: getAllGroups,
    },
  },
}))

vi.mock('@/composables/useTableLoader', async () => {
  const { reactive, ref } = await vi.importActual<typeof import('vue')>('vue')

  return {
    useTableLoader: vi.fn(() => ({
      items: ref(tableLoaderState.items),
      loading: ref(false),
      params: reactive({ ...tableLoaderState.params }),
      pagination: reactive({ ...tableLoaderState.pagination }),
      load: baseLoad,
      reload: baseReload,
      debouncedReload: baseDebouncedReload,
      handlePageChange,
      handlePageSizeChange,
    })),
  }
})

vi.mock('@/composables/useSwipeSelect', () => ({
  useSwipeSelect: vi.fn(),
}))

vi.mock('@vueuse/core', () => ({
  useIntervalFn: vi.fn(() => ({
    pause: vi.fn(),
    resume: vi.fn(),
  })),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    withLoading: (task: () => Promise<unknown>) => task(),
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isSimpleMode: false,
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const resetTableLoaderState = (overrides?: {
  items?: any[]
  params?: Record<string, string>
  pagination?: Partial<{ page: number; page_size: number; total: number; pages: number }>
}) => {
  tableLoaderState.items = overrides?.items ? [...overrides.items] : []
  tableLoaderState.params = {
    platform: '',
    type: '',
    status: '',
    privacy_mode: '',
    group: '1,2,3',
    group_exclude: '4,5',
    group_match: 'exact',
    search: '',
    sort_by: 'id',
    sort_order: 'desc',
    last_used_filter: '',
    last_used_start_date: '',
    last_used_end_date: '',
    ...(overrides?.params ?? {}),
  }
  tableLoaderState.pagination = {
    page: 1,
    page_size: 20,
    total: Math.max(tableLoaderState.items.length, 1),
    pages: 1,
    ...(overrides?.pagination ?? {}),
  }
}

const buildAccount = (overrides: Record<string, unknown> = {}) => {
  const groupIds = Array.isArray(overrides.group_ids)
    ? (overrides.group_ids as number[])
    : [1]

  return {
    id: 101,
    name: 'Account 101',
    platform: 'anthropic',
    type: 'oauth',
    status: 'active',
    schedulable: true,
    credentials: {},
    extra: {},
    group_ids: groupIds,
    groups: groupIds.map((id) => ({ id, name: `Group ${id}` })),
    ...overrides,
  }
}

const AppLayoutStub = { template: '<div><slot /></div>' }
const TablePageLayoutStub = { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' }
const AccountTableActionsStub = { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' }
const DataTableStub = {
  name: 'DataTable',
  props: ['data'],
  methods: {
    serializeRowIds(data: Array<{ id: number }> | undefined) {
      return JSON.stringify((data ?? []).map((row) => row.id))
    },
  },
  template: '<div class="data-table-stub" :data-row-ids="serializeRowIds(data)"></div>',
}
const EditAccountModalStub = {
  name: 'EditAccountModal',
  props: ['show', 'account'],
  emits: ['updated', 'close'],
  template: '<div class="edit-account-modal-stub"></div>',
}
const ConfirmDialogStub = {
  name: 'ConfirmDialog',
  props: ['show'],
  emits: ['confirm', 'cancel'],
  template: '<div v-if="show" class="confirm-dialog-stub"><button class="confirm-dialog-confirm" @click="$emit(\'confirm\')">confirm</button><slot /></div>',
}
const AccountBulkOperationsPanelStub = {
  name: 'AccountBulkOperationsPanel',
  props: ['bulkActionMenuLabel', 'bulkActionMenuDisabled', 'selectedCount'],
  emits: ['edit-filtered', 'test-filtered', 'refresh-usage-filtered', 'open-import', 'open-public-links', 'open-export'],
  template: `
    <div
      class="bulk-operations-panel-stub"
      :data-bulk-label="bulkActionMenuLabel"
      :data-disabled="String(bulkActionMenuDisabled)"
      :data-selected-count="String(selectedCount)"
    >
      <button class="bulk-panel-edit" @click="$emit('edit-filtered')">edit-filtered</button>
      <button class="bulk-panel-test" @click="$emit('test-filtered')">test-filtered</button>
      <button class="bulk-panel-refresh-usage" @click="$emit('refresh-usage-filtered')">refresh-usage-filtered</button>
      <button class="bulk-panel-import" @click="$emit('open-import')">open-import</button>
      <button class="bulk-panel-public-links" @click="$emit('open-public-links')">open-public-links</button>
      <button class="bulk-panel-export" @click="$emit('open-export')">open-export</button>
    </div>
  `,
}

const AccountBulkOperationProgressListStub = {
  name: 'AccountBulkOperationProgressList',
  props: ['progressItems'],
  methods: {
    serializeProgressItems(items: unknown) {
      return JSON.stringify(items ?? [])
    },
  },
  template: `
    <div
      class="bulk-progress-list-stub"
      :data-progress-items="serializeProgressItems(progressItems)"
    />
  `,
}

const mountView = (extraStubs: Record<string, unknown> = {}) => mount(AccountsView, {
  global: {
    stubs: {
      AppLayout: AppLayoutStub,
      TablePageLayout: TablePageLayoutStub,
      AccountTableActions: AccountTableActionsStub,
      AccountTableFilters: true,
      AccountBulkActionsBar: true,
      DataTable: DataTableStub,
      Pagination: true,
      ConfirmDialog: ConfirmDialogStub,
      CreateAccountModal: true,
      EditAccountModal: EditAccountModalStub,
      ReAuthAccountModal: true,
      AccountTestModal: true,
      AccountBatchTestModal: true,
      AccountStatsModal: true,
      ScheduledTestsPanel: true,
      AccountActionMenu: true,
      SyncFromCrsModal: true,
      ImportDataModal: true,
      OpenAIPublicLinksModal: true,
      BulkEditAccountModal: true,
      TempUnschedStatusModal: true,
      ErrorPassthroughRulesModal: true,
      TLSFingerprintProfilesModal: true,
      AccountStatusIndicator: true,
      AccountUsageCell: true,
      AccountTodayStatsCell: true,
      AccountGroupsCell: true,
      AccountCapacityCell: true,
      PlatformTypeBadge: true,
      Icon: true,
      AccountBulkOperationProgressList: true,
      ...extraStubs,
    },
  },
})

const getRenderedRowIds = (wrapper: ReturnType<typeof mountView>) =>
  JSON.parse(wrapper.find('.data-table-stub').attributes('data-row-ids') || '[]')

describe('admin AccountsView filtered bulk params', () => {
  beforeEach(() => {
    list.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getUsage.mockReset()
    previewBulkUpdateTargets.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    exportData.mockReset()
    setSchedulable.mockReset()
    recoverState.mockReset()
    baseLoad.mockReset()
    baseReload.mockReset()
    baseDebouncedReload.mockReset()
    handlePageChange.mockReset()
    handlePageSizeChange.mockReset()

    resetTableLoaderState()

    baseLoad.mockResolvedValue(undefined)
    baseReload.mockResolvedValue(undefined)
    baseDebouncedReload.mockResolvedValue(undefined)
    handlePageChange.mockReturnValue(undefined)
    handlePageSizeChange.mockReturnValue(undefined)

    list.mockResolvedValue({
      items: [
        { id: 101, platform: 'anthropic', type: 'oauth' },
      ],
      total: 1,
      pages: 1,
    })
    listWithEtag.mockResolvedValue({
      notModified: false,
      etag: null,
      data: {
        items: [],
        total: 0,
        pages: 0,
      },
    })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getUsage.mockResolvedValue({})
    previewBulkUpdateTargets.mockResolvedValue({
      count: 1,
      platforms: ['anthropic'],
      types: ['oauth'],
    })
    resolveBulkUpdateTargets.mockResolvedValue({
      count: 1,
      account_ids: [101],
    })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    exportData.mockResolvedValue({
      exported_at: '2026-01-31T00:00:00Z',
      proxies: [],
      accounts: [],
    })
    setSchedulable.mockImplementation(async (id: number, schedulable: boolean) => buildAccount({ id, schedulable }))
    recoverState.mockImplementation(async (id: number) => buildAccount({ id, status: 'active' }))

    vi.stubGlobal('confirm', vi.fn(() => true))

    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      value: vi.fn(() => 'blob:mock'),
    })
    Object.defineProperty(URL, 'revokeObjectURL', {
      configurable: true,
      value: vi.fn(),
    })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  })

  it('renders bulk progress below the action panel and above the pending sync hint', async () => {
    const wrapper = mountView({
      AccountBulkOperationsPanel: AccountBulkOperationsPanelStub,
      AccountBulkOperationProgressList: AccountBulkOperationProgressListStub,
    })
    await flushPromises()

    const panel = wrapper.findComponent({ name: 'AccountBulkOperationsPanel' })
    const progressList = wrapper.findComponent({ name: 'AccountBulkOperationProgressList' })
    expect(panel.exists()).toBe(true)
    expect(progressList.exists()).toBe(true)
    expect(panel.attributes('data-bulk-label')).toContain('admin.accounts.bulkActions.menuWithCount')
    expect(panel.attributes('data-selected-count')).toBe('0')
    expect(JSON.parse(progressList.attributes('data-progress-items') || '[]')).toEqual([])

    ;(wrapper.vm.$.setupState as any).preparingFilteredBatchTest = true
    ;(wrapper.vm.$.setupState as any).filteredBatchTestPreparationLoaded = 1
    ;(wrapper.vm.$.setupState as any).filteredBatchTestPreparationTotal = 3
    await flushPromises()

    const progressItems = JSON.parse(progressList.attributes('data-progress-items') || '[]')
    expect(progressItems).toHaveLength(1)
    expect(progressItems[0]).toMatchObject({
      key: 'filtered-batch-test',
      percent: 33,
    })

    ;(wrapper.vm.$.setupState as any).hasPendingListSync = true
    await flushPromises()

    const html = wrapper.html()
    expect(html.indexOf('bulk-operations-panel-stub')).toBeLessThan(html.indexOf('bulk-progress-list-stub'))
    expect(html.indexOf('bulk-progress-list-stub')).toBeLessThan(html.indexOf('admin.accounts.listPendingSyncHint'))

    ;(wrapper.vm.$.setupState as any).preparingFilteredBatchTest = false
    await flushPromises()

    await wrapper.find('.bulk-panel-import').trigger('click')
    await wrapper.find('.bulk-panel-public-links').trigger('click')
    await wrapper.find('.bulk-panel-export').trigger('click')
    await wrapper.find('.bulk-panel-edit').trigger('click')
    await wrapper.find('.bulk-panel-test').trigger('click')
    await wrapper.find('.bulk-panel-refresh-usage').trigger('click')
    await flushPromises()

    expect((wrapper.vm.$.setupState as any).showImportData).toBe(true)
    expect((wrapper.vm.$.setupState as any).showOpenAIPublicLinks).toBe(true)
    expect((wrapper.vm.$.setupState as any).showExportDataDialog).toBe(true)
    expect(previewBulkUpdateTargets).toHaveBeenCalledTimes(1)
    expect(resolveBulkUpdateTargets).toHaveBeenCalledTimes(2)
  })

  it('passes filtered bulk preview params without leaking sort fields', async () => {
    resetTableLoaderState({
      params: {
        group: '1,2,3',
        group_exclude: '4,5',
        group_match: 'exact',
        sort_by: 'priority,name',
        sort_order: 'desc,asc',
      },
    })

    const wrapper = mountView()

    await flushPromises()

    const bulkMenuButton = wrapper.findAll('button').find((node) => node.text().includes('admin.accounts.bulkActions.menuWithCount'))
    expect(bulkMenuButton).toBeTruthy()
    await bulkMenuButton!.trigger('click')
    await flushPromises()

    const editButton = wrapper.findAll('button').find((node) => node.text().includes('admin.accounts.bulkActions.editAccount'))
    expect(editButton).toBeTruthy()
    await editButton!.trigger('click')
    await flushPromises()

    expect(previewBulkUpdateTargets).toHaveBeenCalledTimes(1)
    const filters = previewBulkUpdateTargets.mock.calls[0][0]
    expect(filters).toMatchObject({
      group: '1,2,3',
      group_exclude: '4,5',
      group_match: 'exact',
    })
    expect(filters).not.toHaveProperty('sort_by')
    expect(filters).not.toHaveProperty('sort_order')
    expect(list).not.toHaveBeenCalled()
  })

  it('passes export filters and sort in separate payload fields', async () => {
    resetTableLoaderState({
      params: {
        group: '1,2,3',
        group_exclude: '4,5',
        group_match: 'exact',
        last_used_filter: 'range',
        last_used_start_date: '2026-01-01',
        last_used_end_date: '2026-01-31',
        sort_by: 'priority,name',
        sort_order: 'desc,asc',
      },
    })

    const wrapper = mountView()
    await flushPromises()

    const exportButton = wrapper.findAll('button').find((node) => node.text().includes('admin.accounts.dataExport'))
    expect(exportButton).toBeTruthy()
    await exportButton!.trigger('click')
    await flushPromises()

    await wrapper.find('.confirm-dialog-confirm').trigger('click')
    await flushPromises()

    expect(exportData).toHaveBeenCalledTimes(1)
    const payload = exportData.mock.calls[0][0]
    expect(payload).toMatchObject({
      includeProxies: true,
      filters: {
        group: '1,2,3',
        group_exclude: '4,5',
        group_match: 'exact',
        last_used_filter: 'range',
        last_used_start_date: '2026-01-01',
        last_used_end_date: '2026-01-31',
      },
      sort: {
        sort_by: 'priority,name',
        sort_order: 'desc,asc',
      },
    })
    expect(payload.filters).not.toHaveProperty('sort_by')
    expect(payload.filters).not.toHaveProperty('sort_order')
  })

  it('resolves filtered batch test targets without leaking sort fields', async () => {
    resetTableLoaderState({
      params: {
        platform: 'anthropic',
        group: '1,2,3',
        group_exclude: '4,5',
        group_match: 'exact',
        sort_by: 'priority,name',
        sort_order: 'desc,asc',
      },
    })

    const wrapper = mountView()
    await flushPromises()

    const bulkMenuButton = wrapper.findAll('button').find((node) => node.text().includes('admin.accounts.bulkActions.menuWithCount'))
    expect(bulkMenuButton).toBeTruthy()
    await bulkMenuButton!.trigger('click')
    await flushPromises()

    const testButton = wrapper.findAll('button').find((node) => node.text().includes('admin.accounts.bulkActions.testAccount'))
    expect(testButton).toBeTruthy()
    await testButton!.trigger('click')
    await flushPromises()

    expect(resolveBulkUpdateTargets).toHaveBeenCalled()
    const requestFilters = resolveBulkUpdateTargets.mock.calls.at(-1)?.[0]

    expect(requestFilters).toMatchObject({
      platform: 'anthropic',
      group: '1,2,3',
      group_exclude: '4,5',
      group_match: 'exact',
    })
    expect(requestFilters).not.toHaveProperty('sort_by')
    expect(requestFilters).not.toHaveProperty('sort_order')
    expect(list).not.toHaveBeenCalled()
  })

  it('resolves filtered usage refresh targets without leaking sort fields', async () => {
    resetTableLoaderState({
      params: {
        platform: 'anthropic',
        group: '1,2,3',
        group_exclude: '4,5',
        group_match: 'exact',
        sort_by: 'priority,name',
        sort_order: 'desc,asc',
      },
    })

    const wrapper = mountView()
    await flushPromises()

    const bulkMenuButton = wrapper.findAll('button').find((node) => node.text().includes('admin.accounts.bulkActions.menuWithCount'))
    expect(bulkMenuButton).toBeTruthy()
    await bulkMenuButton!.trigger('click')
    await flushPromises()

    const refreshUsageButton = wrapper.findAll('button').find((node) => node.text().includes('admin.accounts.bulkActions.refreshUsage'))
    expect(refreshUsageButton).toBeTruthy()
    await refreshUsageButton!.trigger('click')
    await flushPromises()

    expect(resolveBulkUpdateTargets).toHaveBeenCalled()
    const requestFilters = resolveBulkUpdateTargets.mock.calls.at(-1)?.[0]
    expect(requestFilters).toMatchObject({
      platform: 'anthropic',
      group: '1,2,3',
      group_exclude: '4,5',
      group_match: 'exact',
    })
    expect(requestFilters).not.toHaveProperty('sort_by')
    expect(requestFilters).not.toHaveProperty('sort_order')
    expect(list).not.toHaveBeenCalled()
  })

  it('bulk set privacy calls existing single-account API concurrently for selected accounts', async () => {
    resetTableLoaderState({
      items: [
        buildAccount({ id: 201, platform: 'openai', type: 'oauth' }),
        buildAccount({ id: 202, platform: 'antigravity', type: 'oauth' }),
      ],
      pagination: {
        total: 2,
      },
    })
    setPrivacy.mockImplementation(async (id: number) => buildAccount({ id, extra: { privacy_mode: 'training_off' } }))

    const wrapper = mountView()
    await flushPromises()

    ;(wrapper.vm.$.setupState as any).setSelectedIds([201, 202])
    await (wrapper.vm.$.setupState as any).handleBulkSetPrivacy()
    await flushPromises()

    expect(setPrivacy).toHaveBeenCalledTimes(2)
    expect(setPrivacy).toHaveBeenCalledWith(201)
    expect(setPrivacy).toHaveBeenCalledWith(202)
    expect(showSuccess).toHaveBeenCalled()
    expect(baseReload).toHaveBeenCalled()
  })

  it('removes rows that stop matching filters after toggling schedulable', async () => {
    resetTableLoaderState({
      items: [buildAccount({ id: 301, status: 'active', schedulable: false })],
      params: {
        status: 'unschedulable',
      },
    })

    const wrapper = mountView()
    await flushPromises()

    expect(getRenderedRowIds(wrapper)).toEqual([301])

    await (wrapper.vm.$.setupState as any).handleToggleSchedulable(buildAccount({ id: 301, status: 'active', schedulable: false }))
    await flushPromises()

    expect(setSchedulable).toHaveBeenCalledWith(301, true)
    expect(getRenderedRowIds(wrapper)).toEqual([])
  })

  it('keeps locally patched rows when they still match exact group filters', async () => {
    resetTableLoaderState({
      items: [buildAccount({ group_ids: [3, 2, 1] })],
      params: {
        group: '1,2,3',
        group_exclude: '4,5',
        group_match: 'exact',
      },
      pagination: {
        total: 1,
      },
    })

    const wrapper = mountView()
    await flushPromises()
    baseReload.mockClear()

    wrapper.findComponent({ name: 'EditAccountModal' }).vm.$emit('updated', buildAccount({ group_ids: [1, 2, 3] }))
    await flushPromises()

    expect(baseReload).not.toHaveBeenCalled()
    expect(getRenderedRowIds(wrapper)).toEqual([101])
    expect(wrapper.text()).not.toContain('admin.accounts.listPendingSyncHint')
  })

  it('reloads instead of local patching when edit updates a sort-dependent field', async () => {
    resetTableLoaderState({
      items: [buildAccount({ id: 401, name: 'Old Name' })],
      params: {
        sort_by: 'name',
        sort_order: 'asc',
      },
      pagination: {
        total: 1,
      },
    })

    const wrapper = mountView()
    await flushPromises()
    baseReload.mockClear()

    wrapper.findComponent({ name: 'EditAccountModal' }).vm.$emit('updated', buildAccount({ id: 401, name: 'New Name' }))
    await flushPromises()

    expect(baseReload).toHaveBeenCalledTimes(1)
    expect((wrapper.vm.$.setupState as any).accounts[0].name).toBe('Old Name')
  })

  it('exposes usage windows as the usage_7d_remaining sortable column', async () => {
    const wrapper = mountView()
    await flushPromises()

    const usageColumn = (wrapper.vm.$.setupState as any).cols.find((column: { key: string }) => column.key === 'usage_7d_remaining')
    expect(usageColumn).toMatchObject({
      key: 'usage_7d_remaining',
      sortable: true,
    })
  })

  it('reloads instead of local patching when usage window sorting is active', async () => {
    resetTableLoaderState({
      items: [buildAccount({ id: 451, extra: { codex_7d_used_percent: 42 } })],
      params: {
        sort_by: 'usage_7d_remaining',
        sort_order: 'asc',
      },
      pagination: {
        total: 1,
      },
    })

    const wrapper = mountView()
    await flushPromises()
    baseReload.mockClear()

    wrapper.findComponent({ name: 'EditAccountModal' }).vm.$emit('updated', buildAccount({ id: 451, extra: { codex_7d_used_percent: 12 } }))
    await flushPromises()

    expect(baseReload).toHaveBeenCalledTimes(1)
    expect((wrapper.vm.$.setupState as any).accounts[0].extra.codex_7d_used_percent).toBe(42)
  })

  it('reloads instead of local patching when temp unsched reset affects a sort-dependent field', async () => {
    resetTableLoaderState({
      items: [buildAccount({ id: 501, status: 'temp_unschedulable' })],
      params: {
        sort_by: 'status',
        sort_order: 'asc',
      },
      pagination: {
        total: 1,
      },
    })

    const wrapper = mountView()
    await flushPromises()
    baseReload.mockClear()

    await (wrapper.vm.$.setupState as any).handleTempUnschedReset(buildAccount({ id: 501, status: 'active' }))
    await flushPromises()

    expect(baseReload).toHaveBeenCalledTimes(1)
    expect((wrapper.vm.$.setupState as any).accounts[0].status).toBe('temp_unschedulable')
  })

  it('reloads instead of local patching when recover-state affects a sort-dependent field', async () => {
    resetTableLoaderState({
      items: [buildAccount({ id: 601, status: 'error' })],
      params: {
        sort_by: 'status',
        sort_order: 'asc',
      },
      pagination: {
        total: 1,
      },
    })

    const wrapper = mountView()
    await flushPromises()
    baseReload.mockClear()

    await (wrapper.vm.$.setupState as any).handleRecoverState(buildAccount({ id: 601, status: 'error' }))
    await flushPromises()

    expect(recoverState).toHaveBeenCalledWith(601)
    expect(baseReload).toHaveBeenCalledTimes(1)
    expect((wrapper.vm.$.setupState as any).accounts[0].status).toBe('error')
  })

  it('reloads instead of local patching when single refresh affects a sort-dependent field', async () => {
    resetTableLoaderState({
      items: [buildAccount({ id: 701, expires_at: 1600000000 })],
      params: {
        sort_by: 'expires_at',
        sort_order: 'asc',
      },
      pagination: {
        total: 1,
      },
    })

    const wrapper = mountView()
    await flushPromises()
    baseReload.mockClear()

    await (wrapper.vm.$.setupState as any).handleRefresh(buildAccount({ id: 701, expires_at: 1600000000 }))
    await flushPromises()

    expect(refreshCredentials).toHaveBeenCalledWith(701)
    expect(baseReload).toHaveBeenCalledTimes(1)
    expect((wrapper.vm.$.setupState as any).accounts[0].expires_at).toBe(1600000000)
  })

  it('removes locally patched rows when excluded groups are added', async () => {
    resetTableLoaderState({
      items: [buildAccount({ group_ids: [1] })],
      params: {
        group: '1',
        group_exclude: '4',
        group_match: 'exact',
      },
      pagination: {
        total: 2,
      },
    })

    const wrapper = mountView()
    await flushPromises()

    wrapper.findComponent({ name: 'EditAccountModal' }).vm.$emit('updated', buildAccount({ group_ids: [1, 4] }))
    await flushPromises()

    expect(getRenderedRowIds(wrapper)).toEqual([])
    expect(wrapper.text()).toContain('admin.accounts.listPendingSyncHint')
  })
})
