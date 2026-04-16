import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'

import AccountBulkActionsBar from '../AccountBulkActionsBar.vue'

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: {
    en: {
      'admin.accounts.bulkActions.setPrivacy': 'Set Privacy',
      'admin.accounts.bulkActions.selected': 'Selected {count}',
      'admin.accounts.bulkActions.selectCurrentPage': 'Select current page',
      'admin.accounts.bulkActions.clear': 'Clear',
      'admin.accounts.bulkActions.delete': 'Delete',
      'admin.accounts.bulkActions.resetStatus': 'Reset status',
      'admin.accounts.bulkActions.refreshToken': 'Refresh token',
      'admin.accounts.bulkActions.refreshingAction': 'Loading',
      'admin.accounts.bulkTest.action': 'Test',
      'admin.accounts.bulkActions.refreshUsageWindow': 'Refresh usage',
      'admin.accounts.bulkActions.enableScheduling': 'Enable scheduling',
      'admin.accounts.bulkActions.disableScheduling': 'Disable scheduling',
      'admin.accounts.bulkActions.edit': 'Edit',
    },
  },
})

describe('AccountBulkActionsBar', () => {
  it('renders set privacy action and emits set-privacy', async () => {
    const wrapper = mount(AccountBulkActionsBar, {
      props: {
        selectedIds: [101, 102],
      },
      global: {
        plugins: [i18n],
      },
    })

    const button = wrapper.findAll('button').find((node) => node.text().includes('admin.accounts.bulkActions.setPrivacy'))
    expect(button).toBeTruthy()
    await button!.trigger('click')

    expect(wrapper.emitted('set-privacy')).toHaveLength(1)
  })
})
