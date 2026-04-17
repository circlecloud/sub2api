<template>
  <div class="space-y-4">
    <div>
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.openai.oauthPassthrough') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openai.oauthPassthroughDesc') }}
          </p>
        </div>
        <button
          type="button"
          @click="openaiPassthroughEnabledModel = !openaiPassthroughEnabledModel"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            openaiPassthroughEnabledModel ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              openaiPassthroughEnabledModel ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>
    </div>

    <div>
      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.openai.wsMode') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openai.wsModeDesc') }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t(wsModeHintKey) }}
          </p>
        </div>
        <div class="w-52">
          <Select v-model="openaiResponsesWebSocketV2ModeModel" :options="wsModeOptions" />
        </div>
      </div>
    </div>

    <div>
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.openai.codexCLIOnly') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openai.codexCLIOnlyDesc') }}
          </p>
        </div>
        <button
          type="button"
          @click="codexCliOnlyEnabledModel = !codexCliOnlyEnabledModel"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            codexCliOnlyEnabledModel ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              codexCliOnlyEnabledModel ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>
    </div>

    <div>
      <label class="input-label">{{ t('admin.accounts.modelRestriction') }}</label>

      <div
        v-if="openaiPassthroughEnabledModel"
        class="mb-3 rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20"
      >
        <p class="text-xs text-amber-700 dark:text-amber-400">
          {{ t('admin.accounts.openai.modelRestrictionDisabledByPassthrough') }}
        </p>
      </div>

      <template v-else>
        <div class="mb-4 flex gap-2">
          <button
            type="button"
            @click="modelRestrictionModeModel = 'whitelist'"
            :class="[
              'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
              modelRestrictionModeModel === 'whitelist'
                ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
            ]"
          >
            <Icon name="checkCircle" size="sm" class="mr-1.5 inline" />
            {{ t('admin.accounts.modelWhitelist') }}
          </button>
          <button
            type="button"
            @click="modelRestrictionModeModel = 'mapping'"
            :class="[
              'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
              modelRestrictionModeModel === 'mapping'
                ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
            ]"
          >
            <svg
              class="mr-1.5 inline h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"
              />
            </svg>
            {{ t('admin.accounts.modelMapping') }}
          </button>
        </div>

        <div v-if="modelRestrictionModeModel === 'whitelist'">
          <ModelWhitelistSelector
            v-model="allowedModelsModel"
            platform="openai"
            :available-models="availableOpenAIModels"
          />
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.selectedModels', { count: allowedModelsModel.length }) }}
            <span v-if="allowedModelsModel.length === 0">{{ t('admin.accounts.supportsAllModels') }}</span>
          </p>
        </div>

        <div v-else>
          <div class="mb-3 rounded-lg bg-purple-50 p-3 dark:bg-purple-900/20">
            <p class="text-xs text-purple-700 dark:text-purple-400">
              {{ t('admin.accounts.mapRequestModels') }}
            </p>
          </div>

          <div v-if="modelMappings.length > 0" class="mb-3 space-y-2">
            <div
              v-for="(mapping, index) in modelMappings"
              :key="getModelMappingKey(mapping)"
              class="flex items-center gap-2"
            >
              <div class="flex-1">
                <input
                  :value="mapping.from"
                  type="text"
                  class="input w-full"
                  :class="!isValidWildcardPattern(mapping.from) ? 'border-red-500 dark:border-red-500' : ''"
                  :placeholder="t('admin.accounts.requestModel')"
                  @input="updateModelMappingField(index, 'from', ($event.target as HTMLInputElement).value)"
                />
                <p v-if="!isValidWildcardPattern(mapping.from)" class="mt-1 text-xs text-red-500">
                  {{ t('admin.accounts.wildcardOnlyAtEnd') }}
                </p>
              </div>
              <Icon name="arrowRight" size="sm" class="flex-shrink-0 text-gray-400" />
              <input
                :value="mapping.to"
                type="text"
                class="input flex-1"
                :placeholder="t('admin.accounts.actualModel')"
                @input="updateModelMappingField(index, 'to', ($event.target as HTMLInputElement).value)"
              />
              <button
                type="button"
                @click="removeModelMapping(index)"
                class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
              >
                <Icon name="x" size="sm" :stroke-width="2" />
              </button>
            </div>
          </div>

          <button
            type="button"
            @click="addModelMapping"
            class="mb-3 w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
          >
            + {{ t('admin.accounts.addMapping') }}
          </button>

          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in presetMappings"
              :key="preset.label"
              type="button"
              @click="addPresetMapping(preset.from, preset.to)"
              :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]"
            >
              + {{ preset.label }}
            </button>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import Select from '@/components/common/Select.vue'
import { useAppStore } from '@/stores'
import { createStableObjectKeyResolver } from '@/utils/stableObjectKey'
import {
  getDefaultModelSelection,
  getPresetMappingsByPlatform,
  isValidWildcardPattern
} from '@/composables/useModelWhitelist'
import {
  OPENAI_WS_MODE_CTX_POOL,
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_PASSTHROUGH,
  resolveOpenAIWSModeConcurrencyHintKey,
  type OpenAIWSMode
} from '@/utils/openaiWsMode'

interface ModelMapping {
  from: string
  to: string
}

interface Props {
  openaiPassthroughEnabled: boolean
  openaiResponsesWebSocketV2Mode: OpenAIWSMode
  codexCliOnlyEnabled: boolean
  modelRestrictionMode: 'whitelist' | 'mapping'
  allowedModels: string[]
  modelMappings: ModelMapping[]
}

const props = defineProps<Props>()
const emit = defineEmits<{
  'update:openaiPassthroughEnabled': [value: boolean]
  'update:openaiResponsesWebSocketV2Mode': [value: OpenAIWSMode]
  'update:codexCliOnlyEnabled': [value: boolean]
  'update:modelRestrictionMode': [value: 'whitelist' | 'mapping']
  'update:allowedModels': [value: string[]]
  'update:modelMappings': [value: ModelMapping[]]
}>()

const { t } = useI18n()
const appStore = useAppStore()
const getModelMappingKey = createStableObjectKeyResolver<ModelMapping>('openai-link-model-mapping')

const openaiPassthroughEnabledModel = computed({
  get: () => props.openaiPassthroughEnabled,
  set: (value: boolean) => emit('update:openaiPassthroughEnabled', value)
})

const openaiResponsesWebSocketV2ModeModel = computed({
  get: () => props.openaiResponsesWebSocketV2Mode,
  set: (value: OpenAIWSMode) => emit('update:openaiResponsesWebSocketV2Mode', value)
})

const codexCliOnlyEnabledModel = computed({
  get: () => props.codexCliOnlyEnabled,
  set: (value: boolean) => emit('update:codexCliOnlyEnabled', value)
})

const modelRestrictionModeModel = computed({
  get: () => props.modelRestrictionMode,
  set: (value: 'whitelist' | 'mapping') => emit('update:modelRestrictionMode', value)
})

const allowedModelsModel = computed({
  get: () => props.allowedModels,
  set: (value: string[]) => emit('update:allowedModels', value)
})

const wsModeOptions = computed(() => [
  { value: OPENAI_WS_MODE_OFF, label: t('admin.accounts.openai.wsModeOff') },
  { value: OPENAI_WS_MODE_CTX_POOL, label: t('admin.accounts.openai.wsModeCtxPool') },
  { value: OPENAI_WS_MODE_PASSTHROUGH, label: t('admin.accounts.openai.wsModePassthrough') }
])

const wsModeHintKey = computed(() =>
  resolveOpenAIWSModeConcurrencyHintKey(openaiResponsesWebSocketV2ModeModel.value)
)

const availableOpenAIModels = computed(() => getDefaultModelSelection('openai', { accountType: 'oauth' }))

const presetMappings = computed(() => getPresetMappingsByPlatform('openai'))

const emitModelMappings = (nextMappings: ModelMapping[]) => {
  emit('update:modelMappings', nextMappings)
}

const addModelMapping = () => {
  emitModelMappings([...props.modelMappings, { from: '', to: '' }])
}

const removeModelMapping = (index: number) => {
  emitModelMappings(props.modelMappings.filter((_, itemIndex) => itemIndex !== index))
}

const updateModelMappingField = (index: number, key: 'from' | 'to', value: string) => {
  emitModelMappings(
    props.modelMappings.map((mapping, itemIndex) =>
      itemIndex === index ? { ...mapping, [key]: value } : mapping
    )
  )
}

const addPresetMapping = (from: string, to: string) => {
  if (props.modelMappings.some((mapping) => mapping.from === from)) {
    appStore.showInfo(t('admin.accounts.mappingExists', { model: from }))
    return
  }
  emitModelMappings([...props.modelMappings, { from, to }])
}

const modelMappings = computed(() => props.modelMappings)
</script>
