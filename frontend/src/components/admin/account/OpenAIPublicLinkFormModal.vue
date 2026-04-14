<template>
  <BaseDialog :show="show" :title="dialogTitle" width="wide" @close="emit('close')">
    <div class="space-y-6">
      <div class="rounded-xl border border-primary-200 bg-primary-50 p-4 dark:border-primary-700/40 dark:bg-primary-900/20">
        <p class="text-sm text-primary-800 dark:text-primary-200">
          {{ dialogHint }}
        </p>
      </div>

      <section class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-dark-700">
        <div>
          <label class="input-label">{{ t('admin.accounts.publicAddLinks.nameLabel') }}</label>
          <input
            v-model="form.name"
            type="text"
            class="input"
            :placeholder="t('admin.accounts.publicAddLinks.namePlaceholder')"
          />
        </div>

        <div>
          <label class="input-label">
            {{ t('admin.accounts.publicAddLinks.allowedGroups') }}
            <span class="ml-1 text-xs font-normal text-gray-400">
              {{ t('common.selectedCount', { count: form.groupIds.length }) }}
            </span>
          </label>
          <div
            class="grid max-h-48 grid-cols-1 gap-2 overflow-y-auto rounded-lg border border-gray-200 bg-gray-50 p-3 sm:grid-cols-2 dark:border-dark-600 dark:bg-dark-800"
          >
            <label
              v-for="group in openaiGroups"
              :key="group.id"
              class="flex cursor-pointer items-center gap-3 rounded-lg border border-transparent bg-white px-3 py-2 transition hover:border-primary-200 dark:bg-dark-700"
            >
              <input
                type="checkbox"
                :checked="form.groupIds.includes(group.id)"
                class="h-4 w-4 rounded border-gray-300 text-primary-500 focus:ring-primary-500"
                @change="toggleGroup(group.id, ($event.target as HTMLInputElement).checked)"
              />
              <GroupBadge
                :name="group.name"
                :platform="group.platform"
                :subscription-type="group.subscription_type"
                :rate-multiplier="group.rate_multiplier"
                class="min-w-0 flex-1"
              />
            </label>
            <div
              v-if="openaiGroups.length === 0"
              class="rounded-lg border border-dashed border-gray-200 px-3 py-5 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400 sm:col-span-2"
            >
              {{ t('admin.accounts.publicAddLinks.noOpenAIGroups') }}
            </div>
          </div>
        </div>

        <div class="space-y-4 rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800/60">
          <div>
            <h5 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.accounts.publicAddLinks.accountDefaultsTitle') }}
            </h5>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.publicAddLinks.accountDefaultsHint') }}
            </p>
          </div>

          <OpenAIOAuthDefaultsFields
            :openai-passthrough-enabled="form.openaiPassthroughEnabled"
            :openai-responses-web-socket-v2-mode="form.openaiResponsesWebSocketV2Mode"
            :codex-cli-only-enabled="form.codexCLIOnlyEnabled"
            :model-restriction-mode="form.modelRestrictionMode"
            :allowed-models="form.allowedModels"
            :model-mappings="form.modelMappings"
            @update:openai-passthrough-enabled="form.openaiPassthroughEnabled = $event"
            @update:openai-responses-web-socket-v2-mode="form.openaiResponsesWebSocketV2Mode = $event"
            @update:codex-cli-only-enabled="form.codexCLIOnlyEnabled = $event"
            @update:model-restriction-mode="form.modelRestrictionMode = $event"
            @update:allowed-models="form.allowedModels = $event"
            @update:model-mappings="form.modelMappings = $event"
          />

          <AccountRuntimeSettingsFields
            :proxies="proxies"
            :proxy-id="form.proxyId"
            :concurrency="form.concurrency"
            :load-factor="form.loadFactor"
            :priority="form.priority"
            :rate-multiplier="form.rateMultiplier"
            :expires-at="form.expiresAt"
            :auto-pause-on-expired="form.autoPauseOnExpired"
            @update:proxy-id="form.proxyId = $event"
            @update:concurrency="form.concurrency = $event"
            @update:load-factor="form.loadFactor = $event"
            @update:priority="form.priority = $event"
            @update:rate-multiplier="form.rateMultiplier = $event"
            @update:expires-at="form.expiresAt = $event"
            @update:auto-pause-on-expired="form.autoPauseOnExpired = $event"
          />
        </div>

        <div class="flex justify-end gap-3">
          <button class="btn btn-secondary" :disabled="submitting" @click="emit('close')">
            {{ t('common.cancel') }}
          </button>
          <button
            class="btn btn-primary"
            :disabled="submitting || form.groupIds.length === 0"
            @click="handleSubmit"
          >
            <span v-if="submitting">{{ t('common.processing') }}</span>
            <span v-else>{{ submitLabel }}</span>
          </button>
        </div>
      </section>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import AccountRuntimeSettingsFields from '@/components/account/AccountRuntimeSettingsFields.vue'
import OpenAIOAuthDefaultsFields from '@/components/account/OpenAIOAuthDefaultsFields.vue'
import { buildModelMappingObject } from '@/composables/useModelWhitelist'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores'
import type { AdminGroup, Proxy as AccountProxy } from '@/types'
import type { OpenAIPublicAddLink, OpenAIPublicAddLinkAccountDefaults } from '@/api/admin/accounts'
import { OPENAI_WS_MODE_OFF, isOpenAIWSModeEnabled, type OpenAIWSMode } from '@/utils/openaiWsMode'

interface ModelMapping {
  from: string
  to: string
}

interface Props {
  show: boolean
  mode: 'create' | 'edit'
  groups: AdminGroup[]
  proxies: AccountProxy[]
  initialLink?: OpenAIPublicAddLink | null
}

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
  saved: [link: OpenAIPublicAddLink]
}>()

const { t } = useI18n()
const appStore = useAppStore()
const submitting = ref(false)

const form = reactive({
  name: '',
  groupIds: [] as number[],
  proxyId: null as number | null,
  concurrency: 10,
  loadFactor: null as number | null,
  priority: 1,
  rateMultiplier: 1,
  expiresAt: null as number | null,
  autoPauseOnExpired: true,
  openaiPassthroughEnabled: false,
  openaiResponsesWebSocketV2Mode: OPENAI_WS_MODE_OFF as OpenAIWSMode,
  codexCLIOnlyEnabled: false,
  modelRestrictionMode: 'whitelist' as 'whitelist' | 'mapping',
  allowedModels: [] as string[],
  modelMappings: [] as ModelMapping[]
})

const openaiGroups = computed(() => props.groups.filter((group) => group.platform === 'openai'))
const isEditing = computed(() => props.mode === 'edit')
const dialogTitle = computed(() =>
  t(isEditing.value ? 'admin.accounts.publicAddLinks.editTitle' : 'admin.accounts.publicAddLinks.createTitle')
)
const dialogHint = computed(() =>
  t(isEditing.value ? 'admin.accounts.publicAddLinks.editHint' : 'admin.accounts.publicAddLinks.createHint')
)
const submitLabel = computed(() =>
  isEditing.value ? t('common.save') : t('admin.accounts.publicAddLinks.createAction')
)

const resetForm = () => {
  form.name = ''
  form.groupIds = openaiGroups.value.length > 0 ? [openaiGroups.value[0].id] : []
  form.proxyId = null
  form.concurrency = 10
  form.loadFactor = null
  form.priority = 1
  form.rateMultiplier = 1
  form.expiresAt = null
  form.autoPauseOnExpired = true
  form.openaiPassthroughEnabled = false
  form.openaiResponsesWebSocketV2Mode = OPENAI_WS_MODE_OFF
  form.codexCLIOnlyEnabled = false
  form.modelRestrictionMode = 'whitelist'
  form.allowedModels = []
  form.modelMappings = []
}

const ensureDefaultSelection = () => {
  if (form.groupIds.length === 0 && openaiGroups.value.length > 0) {
    form.groupIds = [openaiGroups.value[0].id]
  }
}

const applyLinkToForm = (link: OpenAIPublicAddLink) => {
  const defaults = link.account_defaults
  const rawModelMapping = defaults?.credentials?.model_mapping
  const mappingEntries = rawModelMapping && typeof rawModelMapping === 'object'
    ? Object.entries(rawModelMapping as Record<string, unknown>)
        .filter(([from, to]) => from.trim() && typeof to === 'string' && to.trim())
        .map(([from, to]) => ({ from: from.trim(), to: String(to).trim() }))
    : []
  const isWhitelistMode =
    mappingEntries.length > 0 &&
    mappingEntries.every((item) => item.from === item.to && !item.from.includes('*'))

  form.name = link.name || ''
  form.groupIds = [...link.group_ids]
  form.proxyId = defaults?.proxy_id ?? null
  form.concurrency = defaults?.concurrency ?? 10
  form.loadFactor = defaults?.load_factor ?? null
  form.priority = defaults?.priority ?? 1
  form.rateMultiplier = defaults?.rate_multiplier ?? 1
  form.expiresAt = defaults?.expires_at ?? null
  form.autoPauseOnExpired = defaults?.auto_pause_on_expired ?? true
  form.openaiPassthroughEnabled = defaults?.extra?.openai_passthrough === true
  form.openaiResponsesWebSocketV2Mode =
    typeof defaults?.extra?.openai_oauth_responses_websockets_v2_mode === 'string'
      ? (defaults.extra.openai_oauth_responses_websockets_v2_mode as OpenAIWSMode)
      : OPENAI_WS_MODE_OFF
  form.codexCLIOnlyEnabled = defaults?.extra?.codex_cli_only === true
  form.modelRestrictionMode = isWhitelistMode ? 'whitelist' : 'mapping'
  form.allowedModels = isWhitelistMode ? mappingEntries.map((item) => item.from) : []
  form.modelMappings = isWhitelistMode ? [] : mappingEntries
  ensureDefaultSelection()
}

const buildAccountDefaultsPayload = (): OpenAIPublicAddLinkAccountDefaults | undefined => {
  const payload: OpenAIPublicAddLinkAccountDefaults = {}

  if (form.proxyId != null) {
    payload.proxy_id = form.proxyId
  }
  if (form.concurrency !== 10) {
    payload.concurrency = form.concurrency
  }
  if (form.loadFactor != null) {
    payload.load_factor = form.loadFactor
  }
  if (form.priority !== 1) {
    payload.priority = form.priority
  }
  if (form.rateMultiplier !== 1) {
    payload.rate_multiplier = form.rateMultiplier
  }
  if (form.expiresAt != null) {
    payload.expires_at = form.expiresAt
  }
  if (!form.autoPauseOnExpired) {
    payload.auto_pause_on_expired = false
  }

  const credentials: Record<string, unknown> = {}
  if (!form.openaiPassthroughEnabled) {
    const modelMapping = buildModelMappingObject(
      form.modelRestrictionMode,
      form.allowedModels,
      form.modelMappings
    )
    if (modelMapping) {
      credentials.model_mapping = modelMapping
    }
  }
  if (Object.keys(credentials).length > 0) {
    payload.credentials = credentials
  }

  const extra: Record<string, unknown> = {}
  if (form.openaiPassthroughEnabled) {
    extra.openai_passthrough = true
  }
  if (form.codexCLIOnlyEnabled) {
    extra.codex_cli_only = true
  }
  if (form.openaiResponsesWebSocketV2Mode !== OPENAI_WS_MODE_OFF) {
    extra.openai_oauth_responses_websockets_v2_mode = form.openaiResponsesWebSocketV2Mode
    extra.openai_oauth_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(
      form.openaiResponsesWebSocketV2Mode
    )
  }
  if (Object.keys(extra).length > 0) {
    payload.extra = extra
  }

  return Object.keys(payload).length > 0 ? payload : undefined
}

const toggleGroup = (groupId: number, checked: boolean) => {
  if (checked) {
    if (!form.groupIds.includes(groupId)) {
      form.groupIds = [...form.groupIds, groupId]
    }
    return
  }
  form.groupIds = form.groupIds.filter((id) => id !== groupId)
}

const handleSubmit = async () => {
  if (form.groupIds.length === 0) return
  submitting.value = true
  try {
    const payload = {
      name: form.name.trim() || undefined,
      group_ids: form.groupIds,
      account_defaults: buildAccountDefaultsPayload()
    }

    const saved = isEditing.value && props.initialLink?.token
      ? await adminAPI.accounts.updateOpenAIPublicAddLink(props.initialLink.token, payload)
      : await adminAPI.accounts.createOpenAIPublicAddLink(payload)

    appStore.showSuccess(
      t(
        isEditing.value
          ? 'admin.accounts.publicAddLinks.updatedSuccess'
          : 'admin.accounts.publicAddLinks.createdSuccess'
      )
    )
    emit('saved', saved)
    emit('close')
  } catch (error: any) {
    appStore.showError(
      error?.message ||
        t(
          isEditing.value
            ? 'admin.accounts.publicAddLinks.updateFailed'
            : 'admin.accounts.publicAddLinks.createFailed'
        )
    )
  } finally {
    submitting.value = false
  }
}

watch(
  () => props.show,
  (show) => {
    if (!show) return
    if (isEditing.value && props.initialLink) {
      applyLinkToForm(props.initialLink)
      return
    }
    resetForm()
  }
)

watch(openaiGroups, () => {
  if (!props.show) return
  const validIds = new Set(openaiGroups.value.map((group) => group.id))
  form.groupIds = form.groupIds.filter((id) => validIds.has(id))
  ensureDefaultSelection()
})
</script>
