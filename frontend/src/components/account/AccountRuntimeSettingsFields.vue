<template>
  <div class="space-y-4">
    <div>
      <label class="input-label">{{ t('admin.accounts.proxy') }}</label>
      <ProxySelector v-model="proxyIdModel" :proxies="proxies" />
    </div>

    <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <div>
        <label class="input-label">{{ t('admin.accounts.concurrency') }}</label>
        <input
          v-model.number="concurrencyModel"
          type="number"
          min="1"
          class="input"
          @input="normalizeConcurrency"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.loadFactor') }}</label>
        <input
          v-model.number="loadFactorModel"
          type="number"
          min="1"
          class="input"
          :placeholder="String(concurrencyModel || 1)"
          @input="normalizeLoadFactor"
        />
        <p class="input-hint">{{ t('admin.accounts.loadFactorHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.priority') }}</label>
        <input
          v-model.number="priorityModel"
          type="number"
          min="1"
          class="input"
          @input="normalizePriority"
        />
        <p class="input-hint">{{ t('admin.accounts.priorityHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.billingRateMultiplier') }}</label>
        <input
          v-model.number="rateMultiplierModel"
          type="number"
          min="0"
          step="0.001"
          class="input"
          @input="normalizeRateMultiplier"
        />
        <p class="input-hint">{{ t('admin.accounts.billingRateMultiplierHint') }}</p>
      </div>
    </div>

    <div>
      <label class="input-label">{{ t('admin.accounts.expiresAt') }}</label>
      <input v-model="expiresAtInput" type="datetime-local" class="input" />
      <p class="input-hint">{{ t('admin.accounts.expiresAtHint') }}</p>
    </div>

    <div>
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.autoPauseOnExpired') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.autoPauseOnExpiredDesc') }}
          </p>
        </div>
        <button
          type="button"
          @click="autoPauseOnExpiredModel = !autoPauseOnExpiredModel"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            autoPauseOnExpiredModel ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              autoPauseOnExpiredModel ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ProxySelector from '@/components/common/ProxySelector.vue'
import type { Proxy } from '@/types'
import { formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/utils/format'

interface Props {
  proxies: Proxy[]
  proxyId: number | null
  concurrency: number
  loadFactor: number | null
  priority: number
  rateMultiplier: number
  expiresAt: number | null
  autoPauseOnExpired: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  'update:proxyId': [value: number | null]
  'update:concurrency': [value: number]
  'update:loadFactor': [value: number | null]
  'update:priority': [value: number]
  'update:rateMultiplier': [value: number]
  'update:expiresAt': [value: number | null]
  'update:autoPauseOnExpired': [value: boolean]
}>()

const { t } = useI18n()

const proxyIdModel = computed({
  get: () => props.proxyId,
  set: (value: number | null) => emit('update:proxyId', value)
})

const concurrencyModel = computed({
  get: () => props.concurrency,
  set: (value: number) => emit('update:concurrency', value)
})

const loadFactorModel = computed({
  get: () => props.loadFactor,
  set: (value: number | null) => emit('update:loadFactor', value)
})

const priorityModel = computed({
  get: () => props.priority,
  set: (value: number) => emit('update:priority', value)
})

const rateMultiplierModel = computed({
  get: () => props.rateMultiplier,
  set: (value: number) => emit('update:rateMultiplier', value)
})

const autoPauseOnExpiredModel = computed({
  get: () => props.autoPauseOnExpired,
  set: (value: boolean) => emit('update:autoPauseOnExpired', value)
})

const expiresAtInput = computed({
  get: () => formatDateTimeLocalInput(props.expiresAt),
  set: (value: string) => emit('update:expiresAt', parseDateTimeLocalInput(value))
})

const normalizeConcurrency = () => {
  concurrencyModel.value = Math.max(1, Number(concurrencyModel.value) || 1)
}

const normalizeLoadFactor = () => {
  const value = Number(loadFactorModel.value)
  loadFactorModel.value = Number.isFinite(value) && value >= 1 ? value : null
}

const normalizePriority = () => {
  priorityModel.value = Math.max(1, Number(priorityModel.value) || 1)
}

const normalizeRateMultiplier = () => {
  const value = Number(rateMultiplierModel.value)
  rateMultiplierModel.value = Number.isFinite(value) ? Math.max(0, value) : 0
}
</script>
