import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountTableFilters from '../AccountTableFilters.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'admin.accounts.groupFilterSelectedSummary') {
          return `${String(params?.first ?? '')} +${String(params?.count ?? '')}`
        }
        if (key === 'common.selectedCount') {
          return `selected:${String(params?.count ?? '')}`
        }
        if (key === 'admin.accounts.groupFilterMetaDefault') {
          return 'Disabled'
        }
        if (key === 'admin.accounts.groupFilterMetaContains') {
          return 'Contains match'
        }
        if (key === 'admin.accounts.groupFilterMetaExact') {
          return 'Exact match'
        }
        if (key === 'admin.accounts.groupFilterMetaExcludeCount') {
          return `Exclude ${String(params?.count ?? '')}`
        }
        if (key === 'admin.accounts.groupIncludeSectionLabel') {
          return 'Include Groups'
        }
        if (key === 'admin.accounts.groupExcludeSectionLabel') {
          return 'Exclude Groups'
        }
        if (key === 'admin.accounts.groupExcludeHint') {
          return 'Exclude accounts in selected groups'
        }
        if (key === 'admin.accounts.groupFilterExcludeOnlyHint') {
          return 'Exclude-only filter'
        }
        if (key === 'admin.accounts.groupFilterUngroupedHint') {
          return 'Only ungrouped accounts'
        }
        if (key === 'admin.accounts.lastUsedFilters.all') {
          return 'All Last Used'
        }
        if (key === 'admin.accounts.lastUsedFilters.used') {
          return 'Used'
        }
        if (key === 'admin.accounts.lastUsedFilters.unused') {
          return 'Unused'
        }
        if (key === 'admin.accounts.lastUsedFilters.range') {
          return 'Last Used Time'
        }
        return key
      }
    })
  }
})

const buildFilters = (overrides: Record<string, unknown> = {}) => ({
  platform: '',
  type: '',
  status: '',
  group: '',
  group_exclude: '',
  group_match: '',
  privacy_mode: '',
  ...overrides
})

const buildWrapper = (
  filters: Record<string, unknown>,
  groups: Array<{ id: number; name: string }> = [],
  extraStubs: Record<string, unknown> = {}
) => mount(AccountTableFilters, {
  props: {
    searchQuery: '',
    filters,
    groups
  },
  global: {
    stubs: {
      SearchInput: {
        template: '<div />'
      },
      DateRangePicker: {
        props: ['startDate', 'endDate', 'inline'],
        emits: ['update:startDate', 'update:endDate', 'change'],
        template: `<button class="date-range-stub" @click="$emit('change', { startDate: '2026-01-01', endDate: '2026-01-31', preset: null })">apply-range</button>`
      },
      Select: {
        props: ['modelValue', 'options'],
        emits: ['update:modelValue', 'change'],
        template: '<div class="select-stub" :data-options="JSON.stringify(options)" />'
      },
      ...extraStubs
    }
  }
})

describe('AccountTableFilters', () => {
  it('bridges extracted popover events through update:filters and change', async () => {
    const wrapper = buildWrapper(buildFilters(), [], {
      AccountGroupFilterPopover: {
        name: 'AccountGroupFilterPopover',
        emits: ['update:filters', 'change'],
        template: '<div class="group-filter-popover-stub"></div>'
      },
      AccountLastUsedFilterPopover: {
        name: 'AccountLastUsedFilterPopover',
        emits: ['update:filters', 'change'],
        template: '<div class="last-used-filter-popover-stub"></div>'
      }
    })

    const groupPopover = wrapper.findComponent({ name: 'AccountGroupFilterPopover' })
    const lastUsedPopover = wrapper.findComponent({ name: 'AccountLastUsedFilterPopover' })

    expect(groupPopover.exists()).toBe(true)
    expect(lastUsedPopover.exists()).toBe(true)

    groupPopover.vm.$emit('update:filters', { group: '1,2', group_exclude: '', group_match: '' })
    groupPopover.vm.$emit('change')
    lastUsedPopover.vm.$emit('update:filters', { last_used_filter: 'unused', last_used_start_date: '', last_used_end_date: '' })
    lastUsedPopover.vm.$emit('change')

    expect(wrapper.emitted('update:filters')).toEqual([
      [{ group: '1,2', group_exclude: '', group_match: '' }],
      [{ last_used_filter: 'unused', last_used_start_date: '', last_used_end_date: '' }]
    ])
    expect(wrapper.emitted('change')).toHaveLength(2)
  })

  it('renders privacy mode options', () => {
    const wrapper = buildWrapper(buildFilters())

    const selects = wrapper.findAll('.select-stub')
    const privacyOptions = JSON.parse(selects[3].attributes('data-options'))
    expect(privacyOptions).toEqual([
      { value: '', label: 'admin.accounts.allPrivacyModes' },
      { value: '__unset__', label: 'admin.accounts.privacyUnset' },
      { value: 'training_off', label: 'Privacy' },
      { value: 'training_set_cf_blocked', label: 'CF' },
      { value: 'training_set_failed', label: 'Fail' }
    ])
  })

  it('supports switching between contains and exact group matching', async () => {
    const wrapper = buildWrapper(buildFilters(), [
      { id: 3, name: 'Group C' },
      { id: 1, name: 'Group A' },
      { id: 2, name: 'Group B' }
    ])

    const groupButton = wrapper.findAll('button').find((node) => node.text().includes('admin.accounts.allGroups'))
    const disabledModeButton = wrapper.findAll('button').find((node) => node.text() === 'Disabled')
    expect(groupButton).toBeTruthy()
    expect(disabledModeButton?.attributes('disabled')).toBeDefined()
    await groupButton!.trigger('click')

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(6)

    await checkboxes[0].setValue(true)
    await wrapper.setProps({ filters: buildFilters({ group: '3' }) })

    await wrapper.findAll('input[type="checkbox"]')[1].setValue(true)
    await wrapper.setProps({ filters: buildFilters({ group: '1,3' }) })

    await wrapper.findAll('input[type="checkbox"]')[2].setValue(true)
    await wrapper.setProps({ filters: buildFilters({ group: '1,2,3' }) })

    const containsModeButton = wrapper.findAll('button').find((node) => node.text() === 'Contains match')
    expect(containsModeButton).toBeTruthy()
    await containsModeButton!.trigger('click')

    await wrapper.setProps({ filters: buildFilters({ group: '1,2,3', group_match: 'exact' }) })
    const exactModeButton = wrapper.findAll('button').find((node) => node.text() === 'Exact match')
    expect(exactModeButton).toBeTruthy()
    await exactModeButton!.trigger('click')

    const updateFiltersEvents = wrapper.emitted('update:filters') || []
    expect(updateFiltersEvents).toHaveLength(5)
    expect(updateFiltersEvents[0][0]).toMatchObject({ group: '3', group_exclude: '', group_match: '' })
    expect(updateFiltersEvents[1][0]).toMatchObject({ group: '1,3', group_exclude: '', group_match: '' })
    expect(updateFiltersEvents[2][0]).toMatchObject({ group: '1,2,3', group_exclude: '', group_match: '' })
    expect(updateFiltersEvents[3][0]).toMatchObject({ group: '1,2,3', group_exclude: '', group_match: 'exact' })
    expect(updateFiltersEvents[4][0]).toMatchObject({ group: '1,2,3', group_exclude: '', group_match: '' })

    const changeEvents = wrapper.emitted('change') || []
    expect(changeEvents).toHaveLength(5)
  })

  it('supports excluding groups and removes overlap with included groups', async () => {
    const wrapper = buildWrapper(buildFilters(), [
      { id: 3, name: 'Group C' },
      { id: 1, name: 'Group A' },
      { id: 2, name: 'Group B' }
    ])

    const groupButton = wrapper.findAll('button').find((node) => node.text().includes('admin.accounts.allGroups'))
    expect(groupButton).toBeTruthy()
    await groupButton!.trigger('click')

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(6)

    await checkboxes[0].setValue(true)
    await wrapper.setProps({ filters: buildFilters({ group: '3' }) })

    await checkboxes[4].setValue(true)
    await wrapper.setProps({ filters: buildFilters({ group: '3', group_exclude: '1' }) })

    await checkboxes[3].setValue(true)

    const updateFiltersEvents = wrapper.emitted('update:filters') || []
    expect(updateFiltersEvents).toHaveLength(3)
    expect(updateFiltersEvents[0][0]).toMatchObject({ group: '3', group_exclude: '', group_match: '' })
    expect(updateFiltersEvents[1][0]).toMatchObject({ group: '3', group_exclude: '1', group_match: '' })
    expect(updateFiltersEvents[2][0]).toMatchObject({ group: '', group_exclude: '1,3', group_match: '' })

    const changeEvents = wrapper.emitted('change') || []
    expect(changeEvents).toHaveLength(3)
  })

  it('keeps exact matching when excluded groups are added', async () => {
    const wrapper = buildWrapper(buildFilters(), [
      { id: 1, name: 'Group A' },
      { id: 2, name: 'Group B' },
      { id: 3, name: 'Group C' }
    ])

    const groupButton = wrapper.findAll('button').find((node) => node.text().includes('admin.accounts.allGroups'))
    expect(groupButton).toBeTruthy()
    await groupButton!.trigger('click')

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(6)

    await checkboxes[0].setValue(true)
    await wrapper.setProps({ filters: buildFilters({ group: '1' }) })

    await wrapper.findAll('input[type="checkbox"]')[1].setValue(true)
    await wrapper.setProps({ filters: buildFilters({ group: '1,2' }) })

    const containsModeButton = wrapper.findAll('button').find((node) => node.text() === 'Contains match')
    expect(containsModeButton).toBeTruthy()
    await containsModeButton!.trigger('click')
    await wrapper.setProps({ filters: buildFilters({ group: '1,2', group_match: 'exact' }) })

    await wrapper.findAll('input[type="checkbox"]')[4].setValue(true)

    const updateFiltersEvents = wrapper.emitted('update:filters') || []
    expect(updateFiltersEvents).toHaveLength(4)
    expect(updateFiltersEvents[0][0]).toEqual({ group: '1', group_exclude: '', group_match: '' })
    expect(updateFiltersEvents[1][0]).toEqual({ group: '1,2', group_exclude: '', group_match: '' })
    expect(updateFiltersEvents[2][0]).toEqual({ group: '1,2', group_exclude: '', group_match: 'exact' })
    expect(updateFiltersEvents[3][0]).toEqual({ group: '1', group_exclude: '2', group_match: 'exact' })

    const changeEvents = wrapper.emitted('change') || []
    expect(changeEvents).toHaveLength(4)
  })

  it('uses split trigger for last-used status and date range', async () => {
    const wrapper = buildWrapper(buildFilters({
      last_used_filter: '',
      last_used_start_date: '',
      last_used_end_date: ''
    }))

    const lastUsedLeftButton = wrapper.findAll('button').find((node) => node.text().includes('All Last Used'))
    expect(lastUsedLeftButton).toBeTruthy()
    await lastUsedLeftButton!.trigger('click')

    const unusedButton = wrapper.findAll('button').find((node) => node.text() === 'Unused')
    expect(unusedButton).toBeTruthy()
    await unusedButton!.trigger('click')

    const firstFiltersEvent = (wrapper.emitted('update:filters') || [])[0]?.[0]
    expect(firstFiltersEvent).toEqual({
      last_used_filter: 'unused',
      last_used_start_date: '',
      last_used_end_date: ''
    })

    const lastUsedRightButton = wrapper.findAll('button').find((node) => node.text().includes('Last Used Time'))
    expect(lastUsedRightButton).toBeTruthy()
    await lastUsedRightButton!.trigger('click')

    const rangeButton = wrapper.find('.date-range-stub')
    expect(rangeButton.exists()).toBe(true)
    await rangeButton.trigger('click')

    const updateFiltersEvents = wrapper.emitted('update:filters') || []
    expect(updateFiltersEvents[1][0]).toEqual({
      last_used_filter: 'range',
      last_used_start_date: '2026-01-01',
      last_used_end_date: '2026-01-31'
    })

    const changeEvents = wrapper.emitted('change') || []
    expect(changeEvents).toHaveLength(2)
  })
})
