<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.bulkTest.title')"
    width="normal"
    @close="handleClose"
  >
    <div class="space-y-4">
      <div class="rounded-xl border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-500 dark:bg-dark-700/60">
        <div class="text-sm font-medium text-gray-900 dark:text-gray-100">
          {{ selectionInfoText }}
        </div>
        <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.bulkTest.promptEffectHint') }}
        </div>
      </div>

      <div class="space-y-1.5">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.bulkTest.modelLabel') }}
        </label>
        <Select
          v-model="selectedModelId"
          :options="batchModelOptions"
          :disabled="isRunning || loadingModels"
          :placeholder="loadingModels ? t('common.loading') + '...' : t('admin.accounts.bulkTest.modelPlaceholder')"
          searchable
        />
        <div class="text-xs text-gray-500 dark:text-gray-400">
          {{ modelHintText }}
        </div>
      </div>

      <TextArea
        v-model="customPrompt"
        :label="t('admin.accounts.bulkTest.promptLabel')"
        :placeholder="t('admin.accounts.bulkTest.promptPlaceholder')"
        :hint="t('admin.accounts.bulkTest.promptHint')"
        :disabled="isRunning"
        rows="4"
      />

      <div class="space-y-1.5">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.bulkTest.concurrencyLabel') }}
        </label>
        <input
          v-model.number="testConcurrency"
          type="number"
          min="1"
          :max="MAX_BULK_TEST_CONCURRENCY"
          class="input w-full"
          :disabled="isRunning"
          @blur="clampTestConcurrency"
        />
        <div class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.bulkTest.concurrencyHint', { default: DEFAULT_BULK_TEST_CONCURRENCY, max: MAX_BULK_TEST_CONCURRENCY }) }}
        </div>
      </div>

      <div class="rounded-xl border border-primary-200 bg-primary-50 px-4 py-3 dark:border-primary-700/40 dark:bg-primary-900/20">
        <div class="mb-2 flex items-center justify-between gap-3 text-sm text-primary-800 dark:text-primary-200">
          <span>{{ progressStatusText }}</span>
          <span class="font-medium">{{ progressPercent }}%</span>
        </div>
        <div
          class="h-2 overflow-hidden rounded-full bg-primary-100 dark:bg-primary-900/40"
          role="progressbar"
          :aria-valuenow="progressPercent"
          aria-valuemin="0"
          aria-valuemax="100"
          :aria-label="progressStatusText"
        >
          <div
            class="h-full rounded-full bg-primary-500 transition-all duration-300 ease-out dark:bg-primary-400"
            :style="{ width: `${progressPercent}%` }"
          />
        </div>
        <div class="mt-2 flex flex-wrap items-center gap-3 text-xs text-primary-700 dark:text-primary-300">
          <span>{{ t('admin.accounts.bulkTest.successCount', { count: successCount }) }}</span>
          <span>{{ t('admin.accounts.bulkTest.failedCount', { count: failedCount }) }}</span>
        </div>
      </div>

      <div
        v-if="errors.length > 0"
        class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 dark:border-red-700/40 dark:bg-red-900/20"
      >
        <div class="mb-2 text-sm font-medium text-red-700 dark:text-red-300">
          {{ t('admin.accounts.bulkTest.errorListTitle') }}
        </div>
        <ul class="max-h-40 space-y-1 overflow-y-auto text-xs text-red-700 dark:text-red-300">
          <li
            v-for="item in errors"
            :key="`${item.account_id}-${item.error}`"
            class="break-all rounded bg-white/70 px-2 py-1 dark:bg-dark-800/40"
          >
            {{ t('admin.accounts.bulkTest.errorItem', { id: item.account_id, error: item.error }) }}
          </li>
        </ul>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button
          @click="handleClose"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
          :disabled="isRunning"
        >
          {{ t('common.close') }}
        </button>
        <button
          @click="startBatchTest"
          :disabled="isRunning || accountIds.length === 0"
          :class="[
            'rounded-lg px-4 py-2 text-sm font-medium text-white transition-colors',
            isRunning || accountIds.length === 0
              ? 'cursor-not-allowed bg-primary-400'
              : 'bg-primary-500 hover:bg-primary-600'
          ]"
        >
          {{ actionText }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import TextArea from '@/components/common/TextArea.vue'
import { adminAPI } from '@/api/admin'
import type { ClaudeModel } from '@/types'

type BatchTestScope = 'selected' | 'filtered' | 'all'

interface BatchTestErrorItem {
  account_id: number
  error: string
}

interface BatchTestSummary {
  total: number
  success: number
  failed: number
  failedIds: number[]
  errors: BatchTestErrorItem[]
}

const props = withDefaults(defineProps<{
  show: boolean
  accountIds: number[]
  scope?: BatchTestScope
}>(), {
  scope: 'selected'
})

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'completed', summary: BatchTestSummary): void
  (e: 'running-change', running: boolean): void
}>()

const { t } = useI18n()

const DEFAULT_BULK_TEST_CONCURRENCY = 5
const MAX_BULK_TEST_CONCURRENCY = 10
const BULK_MODEL_LOAD_CONCURRENCY = 5
const DEFAULT_BATCH_TEST_PROMPT = '只回复OK'
const phase = ref<'idle' | 'running' | 'completed'>('idle')
const customPrompt = ref(DEFAULT_BATCH_TEST_PROMPT)
const testConcurrency = ref(DEFAULT_BULK_TEST_CONCURRENCY)
const selectedModelId = ref('')
const loadingModels = ref(false)
const modelOptions = ref<Array<{ value: string; label: string }>>([])
const modelLoadFailed = ref(false)
const processedCount = ref(0)
const successCount = ref(0)
const failedCount = ref(0)
const errors = ref<BatchTestErrorItem[]>([])
const failedIds = ref<number[]>([])
let modelLoadSeq = 0

const isRunning = computed(() => phase.value === 'running')
const hasCompleted = computed(() => phase.value === 'completed')
const resolvedTestConcurrency = computed(() => {
  const raw = Number(testConcurrency.value)
  if (!Number.isFinite(raw)) return DEFAULT_BULK_TEST_CONCURRENCY
  return Math.max(1, Math.min(MAX_BULK_TEST_CONCURRENCY, Math.trunc(raw)))
})
const scopeLabel = computed(() => {
  if (props.scope === 'filtered') {
    return t('admin.accounts.bulkTest.filteredScopeLabel', { count: props.accountIds.length })
  }
  if (props.scope === 'all') {
    return t('admin.accounts.bulkTest.allScopeLabel', { count: props.accountIds.length })
  }
  return t('admin.accounts.bulkTest.selectedScopeLabel', { count: props.accountIds.length })
})
const selectionInfoText = computed(() => {
  return t('admin.accounts.bulkTest.selectionInfo', { scope: scopeLabel.value })
})
const progressPercent = computed(() => {
  const total = props.accountIds.length
  if (total <= 0) return 0
  return Math.max(0, Math.min(100, Math.round((processedCount.value / total) * 100)))
})
const progressStatusText = computed(() => {
  if (phase.value === 'idle') {
    return t('admin.accounts.bulkTest.readyStatus', { count: props.accountIds.length })
  }
  if (phase.value === 'completed') {
    return t('admin.accounts.bulkTest.completedStatus', {
      processed: processedCount.value,
      total: props.accountIds.length
    })
  }
  return t('admin.accounts.bulkTest.progressStatus', {
    processed: processedCount.value,
    total: props.accountIds.length
  })
})
const actionText = computed(() => {
  if (isRunning.value) return t('admin.accounts.bulkTest.testingAction')
  if (hasCompleted.value) return t('admin.accounts.retry')
  return t('admin.accounts.bulkTest.startAction')
})
const batchModelOptions = computed(() => [
  {
    value: '',
    label: t('admin.accounts.bulkTest.modelDefaultOption')
  },
  ...modelOptions.value
])
const modelHintText = computed(() => {
  if (loadingModels.value) return t('admin.accounts.bulkTest.modelLoadingHint')
  if (modelLoadFailed.value) return t('admin.accounts.bulkTest.modelLoadFailedHint')
  if (modelOptions.value.length === 0) return t('admin.accounts.bulkTest.modelUnavailableHint')
  return t('admin.accounts.bulkTest.modelHint')
})

const resetState = () => {
  phase.value = 'idle'
  processedCount.value = 0
  successCount.value = 0
  failedCount.value = 0
  errors.value = []
  failedIds.value = []
}

const clampTestConcurrency = () => {
  testConcurrency.value = resolvedTestConcurrency.value
}

const normalizeModelOptions = (models: ClaudeModel[]) => {
  const options: Array<{ value: string; label: string }> = []
  const seen = new Set<string>()
  for (const model of models) {
    const id = typeof model?.id === 'string' ? model.id.trim() : ''
    if (!id || seen.has(id)) continue
    seen.add(id)
    options.push({
      value: id,
      label: typeof model?.display_name === 'string' && model.display_name.trim() ? model.display_name : id
    })
  }
  return options
}

const loadCommonModels = async () => {
  const requestSeq = ++modelLoadSeq
  selectedModelId.value = ''
  modelOptions.value = []
  modelLoadFailed.value = false

  const uniqueAccountIds = [...new Set(props.accountIds.filter((id) => Number.isFinite(id) && id > 0))]
  if (uniqueAccountIds.length === 0) return

  loadingModels.value = true
  const results: Array<Array<{ value: string; label: string }> | null> = new Array(uniqueAccountIds.length).fill(null)
  let nextIndex = 0
  let failed = false

  const worker = async () => {
    while (true) {
      const currentIndex = nextIndex
      if (currentIndex >= uniqueAccountIds.length) return
      nextIndex += 1

      try {
        const models = await adminAPI.accounts.getAvailableModels(uniqueAccountIds[currentIndex])
        if (requestSeq !== modelLoadSeq) return
        results[currentIndex] = normalizeModelOptions(models)
      } catch {
        failed = true
        results[currentIndex] = []
      }
    }
  }

  try {
    await Promise.all(
      Array.from(
        { length: Math.min(BULK_MODEL_LOAD_CONCURRENCY, uniqueAccountIds.length) },
        () => worker()
      )
    )

    if (requestSeq !== modelLoadSeq) return

    if (failed || results.some((item) => item === null)) {
      modelLoadFailed.value = true
      modelOptions.value = []
      return
    }

    const loadedOptions = results as Array<Array<{ value: string; label: string }>>
    if (loadedOptions.length === 0) {
      modelOptions.value = []
      return
    }

    const commonValueSets = loadedOptions.slice(1).map((options) => new Set(options.map((item) => item.value)))
    modelOptions.value = loadedOptions[0].filter((option) => commonValueSets.every((set) => set.has(option.value)))
  } finally {
    if (requestSeq === modelLoadSeq) {
      loadingModels.value = false
    }
  }
}

watch(
  () => props.show,
  (visible) => {
    modelLoadSeq += 1
    if (visible) {
      customPrompt.value = DEFAULT_BATCH_TEST_PROMPT
      testConcurrency.value = DEFAULT_BULK_TEST_CONCURRENCY
      resetState()
      emit('running-change', false)
      void loadCommonModels()
    } else {
      loadingModels.value = false
    }
  },
  { immediate: true }
)

watch(isRunning, (running) => {
  emit('running-change', running)
})

const handleClose = () => {
  if (isRunning.value) return
  emit('close')
}

const buildRequestBody = () => {
  const payload: Record<string, string> = {}
  const prompt = customPrompt.value.trim()
  const modelID = selectedModelId.value.trim()
  if (prompt) {
    payload.prompt = prompt
  }
  if (modelID) {
    payload.model_id = modelID
  }
  return payload
}

const parseErrorResponse = async (response: Response) => {
  const text = await response.text()
  if (!text) return `HTTP error! status: ${response.status}`
  try {
    const parsed = JSON.parse(text) as { message?: string; detail?: string }
    return parsed.message || parsed.detail || text
  } catch {
    return text
  }
}

const parseEventLine = (
  line: string,
  state: { done: boolean; success: boolean; error: string }
) => {
  if (!line.startsWith('data: ')) return
  const jsonStr = line.slice(6).trim()
  if (!jsonStr) return

  try {
    const event = JSON.parse(jsonStr) as {
      type?: string
      success?: boolean
      error?: string
    }

    if (event.type === 'test_complete') {
      state.done = true
      state.success = event.success === true
      state.error = event.success === true ? '' : (event.error || t('admin.accounts.testFailed'))
    }

    if (event.type === 'error') {
      state.done = true
      state.success = false
      state.error = event.error || t('admin.accounts.testFailed')
    }
  } catch {
    // ignore invalid SSE lines
  }
}

const runSingleTest = async (accountId: number) => {
  const response = await fetch(`/api/v1/admin/accounts/${accountId}/test`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${localStorage.getItem('auth_token')}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(buildRequestBody())
  })

  if (!response.ok) {
    throw new Error(await parseErrorResponse(response))
  }

  const reader = response.body?.getReader()
  if (!reader) {
    throw new Error(t('admin.accounts.bulkTest.emptyResponse'))
  }

  const decoder = new TextDecoder()
  let buffer = ''
  const state = { done: false, success: false, error: '' }

  while (!state.done) {
    const { done, value } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop() || ''
    lines.forEach((line) => parseEventLine(line, state))
  }

  if (!state.done && buffer.trim()) {
    parseEventLine(buffer.trim(), state)
  }

  if (!state.done) {
    throw new Error(t('admin.accounts.bulkTest.incompleteResponse'))
  }
  if (!state.success) {
    throw new Error(state.error || t('admin.accounts.testFailed'))
  }
}

const startBatchTest = async () => {
  if (isRunning.value || props.accountIds.length === 0) return

  clampTestConcurrency()
  resetState()
  phase.value = 'running'

  const ids = [...props.accountIds]
  let index = 0

  const worker = async () => {
    while (index < ids.length) {
      const currentId = ids[index]
      index += 1

      try {
        await runSingleTest(currentId)
        successCount.value += 1
      } catch (error: any) {
        failedCount.value += 1
        failedIds.value.push(currentId)
        errors.value.push({
          account_id: currentId,
          error: error?.message || t('admin.accounts.testFailed')
        })
      } finally {
        processedCount.value += 1
      }
    }
  }

  try {
    const workers = Array.from(
      { length: Math.min(resolvedTestConcurrency.value, ids.length) },
      () => worker()
    )
    await Promise.all(workers)
  } finally {
    phase.value = 'completed'
    emit('completed', {
      total: ids.length,
      success: successCount.value,
      failed: failedCount.value,
      failedIds: [...failedIds.value],
      errors: [...errors.value]
    })
  }
}
</script>
