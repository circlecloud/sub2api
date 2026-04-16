import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import Select from '../Select.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

describe('Select dropdown positioning', () => {
  const originalInnerWidth = window.innerWidth
  const originalInnerHeight = window.innerHeight

  beforeEach(() => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1280 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 900 })
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function () {
      if ((this as HTMLElement).tagName === 'BUTTON') {
        return {
          x: 320,
          y: 160,
          top: 160,
          left: 320,
          right: 520,
          bottom: 204,
          width: 200,
          height: 44,
          toJSON: () => ({}),
        } as DOMRect
      }
      return {
        x: 0,
        y: 0,
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        width: 0,
        height: 0,
        toJSON: () => ({}),
      } as DOMRect
    })
    Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {
      configurable: true,
      get() {
        const element = this as HTMLElement
        if (element.classList?.contains('select-dropdown-portal')) {
          return 260
        }
        return 200
      },
    })
    Object.defineProperty(HTMLElement.prototype, 'offsetHeight', {
      configurable: true,
      get() {
        const element = this as HTMLElement
        if (element.classList?.contains('select-dropdown-portal')) {
          return 240
        }
        return 44
      },
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: originalInnerWidth })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: originalInnerHeight })
  })

  it('anchors teleported dropdown to the trigger instead of the viewport left edge', async () => {
    const wrapper = mount(Select, {
      attachTo: document.body,
      props: {
        modelValue: null,
        options: [
          { value: 'a', label: 'Option A' },
          { value: 'b', label: 'Option B' },
        ],
      },
      global: {
        stubs: {
          Icon: { template: '<span />' },
          Teleport: false,
        },
      },
    })

    await wrapper.get('button').trigger('click')
    await nextTick()
    await nextTick()

    const dropdown = document.body.querySelector('.select-dropdown-portal') as HTMLElement | null
    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('320px')
    expect(dropdown?.style.top).toBe('208px')

    wrapper.unmount()
  })
})
