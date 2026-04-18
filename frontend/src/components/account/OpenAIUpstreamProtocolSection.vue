<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between gap-4">
      <div>
        <label v-if="showLabel" :id="labelId" class="input-label mb-0">
          {{ t('admin.accounts.openai.upstreamProtocol') }}
        </label>
        <p v-if="showDescription" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.upstreamProtocolDesc') }}
        </p>
      </div>

      <div class="w-52">
        <Select :id="selectId" v-model="protocolModel" :options="protocolOptions" />
      </div>
    </div>

    <div
      v-if="isChatCompletionsProtocol"
      class="rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20"
      data-testid="openai-upstream-protocol-chat-completions-hint"
    >
      <p class="text-xs text-amber-700 dark:text-amber-400">
        {{ t('admin.accounts.openai.upstreamProtocolChatCompletionsHint') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpenAIApikeyUpstreamProtocol } from '@/types'
import Select from '@/components/common/Select.vue'
import {
  isOpenAIApikeyChatCompletionsProtocol,
  OPENAI_APIKEY_UPSTREAM_PROTOCOL_CHAT_COMPLETIONS,
  OPENAI_APIKEY_UPSTREAM_PROTOCOL_RESPONSES,
} from '@/utils/openaiApikeyUpstreamProtocol'

interface Props {
  modelValue: OpenAIApikeyUpstreamProtocol
  idPrefix?: string
  showLabel?: boolean
  showDescription?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  idPrefix: 'openai-upstream-protocol',
  showLabel: true,
  showDescription: true
})

const emit = defineEmits<{
  'update:modelValue': [value: OpenAIApikeyUpstreamProtocol]
}>()

const { t } = useI18n()

const labelId = computed(() => `${props.idPrefix}-label`)
const selectId = computed(() => `${props.idPrefix}-select`)

const protocolModel = computed({
  get: () => props.modelValue,
  set: (value: OpenAIApikeyUpstreamProtocol) => emit('update:modelValue', value)
})

const protocolOptions = computed(() => [
  {
    value: OPENAI_APIKEY_UPSTREAM_PROTOCOL_RESPONSES,
    label: t('admin.accounts.openai.upstreamProtocolResponses')
  },
  {
    value: OPENAI_APIKEY_UPSTREAM_PROTOCOL_CHAT_COMPLETIONS,
    label: t('admin.accounts.openai.upstreamProtocolChatCompletions')
  }
])

const isChatCompletionsProtocol = computed(() =>
  isOpenAIApikeyChatCompletionsProtocol(protocolModel.value)
)
</script>
