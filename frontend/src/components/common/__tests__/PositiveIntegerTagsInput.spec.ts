import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, nextTick, ref } from 'vue'

import PositiveIntegerTagsInput from '../PositiveIntegerTagsInput.vue'

function mountHarness(initialValue: number[] = []) {
  return mount(
    defineComponent({
      components: { PositiveIntegerTagsInput },
      props: {
        initialValue: {
          type: Array,
          default: () => []
        }
      },
      setup(props) {
        const value = ref<number[]>([...props.initialValue])
        return { value }
      },
      template: '<PositiveIntegerTagsInput v-model="value" placeholder="输入超时" />'
    }),
    {
      props: { initialValue }
    }
  )
}

function getTagValues(wrapper: ReturnType<typeof mountHarness>) {
  return wrapper
    .findAll('[data-testid="positive-integer-tag-value"]')
    .map((node) => Number(node.text()))
}

describe('PositiveIntegerTagsInput', () => {
  it('支持按 Enter 和 Tab 逐项添加并保持顺序', async () => {
    const wrapper = mountHarness([8])
    const input = wrapper.get('[data-testid="positive-integer-tags-input"]')

    await input.setValue('10')
    await input.trigger('keydown', { key: 'Enter' })
    await nextTick()

    await input.setValue('12')
    await input.trigger('keydown', { key: 'Tab' })
    await nextTick()

    expect(getTagValues(wrapper)).toEqual([8, 10, 12])
  })

  it('支持删除单个数值项', async () => {
    const wrapper = mountHarness([5, 7, 9])

    await wrapper.findAll('[data-testid="positive-integer-tag-remove"]')[1].trigger('click')
    await nextTick()

    expect(getTagValues(wrapper)).toEqual([5, 9])
  })

  it('支持将逗号或换行粘贴内容拆分为正整数项', async () => {
    const wrapper = mountHarness()
    const input = wrapper.get('[data-testid="positive-integer-tags-input"]')
    const pasteEvent = new Event('paste', { bubbles: true, cancelable: true })

    Object.defineProperty(pasteEvent, 'clipboardData', {
      value: {
        getData: () => '5,7\n9,0'
      }
    })

    input.element.dispatchEvent(pasteEvent)
    await nextTick()

    expect(getTagValues(wrapper)).toEqual([5, 7, 9])
  })
})
