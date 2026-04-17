<template>
  <div class="space-y-6">
    <div class="card">
      <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.openaiUsageProbe.title') }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.openaiUsageProbe.description') }}
        </p>
      </div>
      <div class="p-6">
        <div class="max-w-md">
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.openaiUsageProbe.method') }}
          </label>
          <select
            v-model="form.openai_usage_probe_method"
            data-testid="openai-usage-probe-method"
            class="input"
          >
            <option value="responses">{{ t('admin.settings.openaiUsageProbe.optionResponses') }}</option>
            <option value="wham">{{ t('admin.settings.openaiUsageProbe.optionWham') }}</option>
          </select>
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.openaiUsageProbe.methodHint') }}
          </p>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.openaiRectifier.title') }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.openaiRectifier.description') }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div class="flex items-center justify-between">
          <div>
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.settings.openaiRectifier.enabled') }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.openaiRectifier.enabledHint') }}
            </p>
          </div>
          <Toggle v-model="form.enable_openai_stream_rectifier" />
        </div>

        <div v-if="form.enable_openai_stream_rectifier" class="grid grid-cols-1 gap-6 border-t border-gray-100 pt-4 md:grid-cols-2 dark:border-dark-700">
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.settings.openaiRectifier.responseHeaderTimeouts') }}
            </label>
            <PositiveIntegerTagsInput
              v-model="form.openai_stream_response_header_rectifier_timeouts"
              :placeholder="t('admin.settings.openaiRectifier.responseHeaderPlaceholder')"
              :input-aria-label="t('admin.settings.openaiRectifier.responseHeaderTimeouts')"
              :remove-aria-label="t('common.delete')"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.openaiRectifier.responseHeaderTimeoutsHint') }}
            </p>
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.settings.openaiRectifier.firstTokenTimeouts') }}
            </label>
            <PositiveIntegerTagsInput
              v-model="form.openai_stream_first_token_rectifier_timeouts"
              :placeholder="t('admin.settings.openaiRectifier.firstTokenPlaceholder')"
              :input-aria-label="t('admin.settings.openaiRectifier.firstTokenTimeouts')"
              :remove-aria-label="t('common.delete')"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.openaiRectifier.firstTokenTimeoutsHint') }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.openaiWarmPool.title') }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.openaiWarmPool.description') }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div class="rounded-lg border border-sky-200 bg-sky-50 p-4 text-sm text-sky-800 dark:border-sky-800 dark:bg-sky-900/20 dark:text-sky-200">
          {{ t('admin.settings.openaiWarmPool.sharedHint') }}
        </div>
        <div class="flex items-center justify-between">
          <div>
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.settings.openaiWarmPool.enabled') }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.openaiWarmPool.enabledHint') }}
            </p>
          </div>
          <Toggle v-model="form.openai_warm_pool_enabled" />
        </div>
        <div
          v-if="form.openai_warm_pool_enabled"
          class="space-y-6 border-t border-gray-100 pt-4 dark:border-dark-700"
        >
          <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.openaiWarmPool.startupGroups') }}</h3>
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.openaiWarmPool.startupGroupsHint') }}
            </p>
            <div class="mt-4">
              <GroupSelector
                v-model="form.openai_warm_pool_startup_group_ids"
                :groups="activeGroups"
                :label="t('admin.settings.openaiWarmPool.startupGroups')"
                platform="openai"
              />
            </div>
          </div>
          <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.openaiWarmPool.bucketTitle') }}</h3>
            <div class="mt-4 grid grid-cols-1 gap-6 md:grid-cols-2 xl:grid-cols-3">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.bucketTargetSize') }}</label>
                <input v-model.number="form.openai_warm_pool_bucket_target_size" type="number" min="1" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.bucketTargetSizeHint') }}</p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.bucketRefillBelow') }}</label>
                <input v-model.number="form.openai_warm_pool_bucket_refill_below" type="number" min="1" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.bucketRefillBelowHint') }}</p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.bucketSyncFillMin') }}</label>
                <input v-model.number="form.openai_warm_pool_bucket_sync_fill_min" type="number" min="0" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.bucketSyncFillMinHint') }}</p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.bucketEntryTtlSeconds') }}</label>
                <input v-model.number="form.openai_warm_pool_bucket_entry_ttl_seconds" type="number" min="1" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.bucketEntryTtlSecondsHint') }}</p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.bucketRefillCooldownSeconds') }}</label>
                <input v-model.number="form.openai_warm_pool_bucket_refill_cooldown_seconds" type="number" min="0" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.bucketRefillCooldownSecondsHint') }}</p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.bucketRefillIntervalSeconds') }}</label>
                <input v-model.number="form.openai_warm_pool_bucket_refill_interval_seconds" type="number" min="0" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.bucketRefillIntervalSecondsHint') }}</p>
              </div>
            </div>
          </div>

          <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.openaiWarmPool.globalTitle') }}</h3>
            <div class="mt-4 grid grid-cols-1 gap-6 md:grid-cols-2 xl:grid-cols-3">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.globalTargetSize') }}</label>
                <input v-model.number="form.openai_warm_pool_global_target_size" type="number" min="1" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.globalTargetSizeHint') }}</p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.globalRefillBelow') }}</label>
                <input v-model.number="form.openai_warm_pool_global_refill_below" type="number" min="1" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.globalRefillBelowHint') }}</p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.globalEntryTtlSeconds') }}</label>
                <input v-model.number="form.openai_warm_pool_global_entry_ttl_seconds" type="number" min="1" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.globalEntryTtlSecondsHint') }}</p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.globalRefillCooldownSeconds') }}</label>
                <input v-model.number="form.openai_warm_pool_global_refill_cooldown_seconds" type="number" min="0" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.globalRefillCooldownSecondsHint') }}</p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.globalRefillIntervalSeconds') }}</label>
                <input v-model.number="form.openai_warm_pool_global_refill_interval_seconds" type="number" min="0" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.globalRefillIntervalSecondsHint') }}</p>
              </div>
            </div>
          </div>

          <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.openaiWarmPool.networkErrorTitle') }}</h3>
            <div class="mt-4 grid grid-cols-1 gap-6 md:grid-cols-2 xl:grid-cols-3">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.networkErrorPoolSize') }}</label>
                <input v-model.number="form.openai_warm_pool_network_error_pool_size" type="number" min="0" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.networkErrorPoolSizeHint') }}</p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.networkErrorEntryTtlSeconds') }}</label>
                <input v-model.number="form.openai_warm_pool_network_error_entry_ttl_seconds" type="number" min="0" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.networkErrorEntryTtlSecondsHint') }}</p>
              </div>
            </div>
          </div>

          <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.openaiWarmPool.probeTitle') }}</h3>
            <div class="mt-4 grid grid-cols-1 gap-6 md:grid-cols-2 xl:grid-cols-3">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.probeMaxCandidates') }}</label>
                <input v-model.number="form.openai_warm_pool_probe_max_candidates" type="number" min="1" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.probeMaxCandidatesHint') }}</p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.probeConcurrency') }}</label>
                <input v-model.number="form.openai_warm_pool_probe_concurrency" type="number" min="1" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.probeConcurrencyHint') }}</p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.probeTimeoutSeconds') }}</label>
                <input v-model.number="form.openai_warm_pool_probe_timeout_seconds" type="number" min="1" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.probeTimeoutSecondsHint') }}</p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiWarmPool.probeFailureCooldownSeconds') }}</label>
                <input v-model.number="form.openai_warm_pool_probe_failure_cooldown_seconds" type="number" min="0" class="input max-w-xs" />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiWarmPool.probeFailureCooldownSecondsHint') }}</p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SystemSettings } from '@/api/admin/settings'
import type { AdminGroup } from '@/types'
import Toggle from '@/components/common/Toggle.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import PositiveIntegerTagsInput from '@/components/common/PositiveIntegerTagsInput.vue'

const props = defineProps<{
  form: SystemSettings
  activeGroups: AdminGroup[]
}>()

const form = toRef(props, 'form')
const activeGroups = toRef(props, 'activeGroups')
const { t } = useI18n()
</script>
