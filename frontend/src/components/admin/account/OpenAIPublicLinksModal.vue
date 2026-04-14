<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.publicAddLinks.title')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-6">
      <div class="rounded-xl border border-primary-200 bg-primary-50 p-4 dark:border-primary-700/40 dark:bg-primary-900/20">
        <p class="text-sm text-primary-800 dark:text-primary-200">
          {{ t('admin.accounts.publicAddLinks.description') }}
        </p>
      </div>

      <section class="space-y-3">
        <div class="flex items-center justify-between gap-3">
          <div>
            <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.accounts.publicAddLinks.listTitle') }}
            </h4>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.publicAddLinks.listHint') }}
            </p>
          </div>
          <div class="flex items-center gap-2">
            <button class="btn btn-primary px-3 py-2 text-xs" @click="showCreateModal = true">
              {{ t('admin.accounts.publicAddLinks.createAction') }}
            </button>
            <button class="btn btn-secondary px-3 py-2 text-xs" :disabled="loading" @click="loadLinks">
              {{ t('common.refresh') }}
            </button>
          </div>
        </div>

        <div
          v-if="loading"
          class="rounded-xl border border-dashed border-gray-200 p-6 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400"
        >
          {{ t('common.loading') }}
        </div>

        <div
          v-else-if="links.length === 0"
          class="rounded-xl border border-dashed border-gray-200 p-6 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400"
        >
          {{ t('admin.accounts.publicAddLinks.empty') }}
        </div>

        <div v-else class="space-y-3">
          <div
            v-for="link in links"
            :key="link.token"
            class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800"
          >
            <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div class="min-w-0 flex-1 space-y-3">
                <div class="flex flex-wrap items-center gap-2">
                  <h5 class="truncate text-sm font-semibold text-gray-900 dark:text-white">
                    {{ link.name || t('admin.accounts.publicAddLinks.unnamed') }}
                  </h5>
                  <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-300">
                    {{ link.group_ids.length }} {{ t('admin.accounts.publicAddLinks.groupUnit') }}
                  </span>
                </div>

                <div class="flex flex-wrap gap-2">
                  <GroupBadge
                    v-for="group in resolveGroups(link.group_ids)"
                    :key="group.id"
                    :name="group.name"
                    :platform="group.platform"
                    :subscription-type="group.subscription_type"
                    :rate-multiplier="group.rate_multiplier"
                  />
                  <span
                    v-for="groupId in resolveMissingGroupIds(link.group_ids)"
                    :key="`missing-${groupId}`"
                    class="rounded-full border border-amber-200 bg-amber-50 px-2 py-1 text-xs text-amber-700 dark:border-amber-700/40 dark:bg-amber-900/20 dark:text-amber-300"
                  >
                    ID {{ groupId }}
                  </span>
                </div>

                <div class="space-y-2 rounded-lg border border-dashed border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-900/30">
                  <label class="block text-xs font-medium text-gray-500 dark:text-gray-400">
                    {{ t('admin.accounts.publicAddLinks.configSummaryTitle') }}
                  </label>
                  <div v-if="linkConfigSummary(link).length > 0" class="flex flex-wrap gap-2">
                    <span
                      v-for="item in linkConfigSummary(link)"
                      :key="item"
                      class="rounded-full bg-white px-2 py-1 text-xs text-gray-600 shadow-sm dark:bg-dark-700 dark:text-gray-300"
                    >
                      {{ item }}
                    </span>
                  </div>
                  <p v-else class="text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.accounts.publicAddLinks.configSummaryEmpty') }}
                  </p>
                </div>

                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
                    {{ t('admin.accounts.publicAddLinks.linkLabel') }}
                  </label>
                  <div class="flex gap-2">
                    <input class="input flex-1 text-xs" :value="link.url" readonly />
                    <button class="btn btn-secondary px-3 py-2 text-xs" @click="copyToClipboard(link.url)">
                      {{ t('common.copy') }}
                    </button>
                  </div>
                </div>

                <p class="text-xs text-gray-400 dark:text-gray-500">
                  {{ t('admin.accounts.publicAddLinks.createdAt', { time: formatTime(link.created_at) }) }}
                </p>
              </div>

              <div class="flex shrink-0 flex-wrap gap-2 lg:flex-col">
                <button
                  class="btn btn-secondary px-3 py-2 text-xs"
                  :disabled="rotatingToken === link.token || deletingToken === link.token"
                  @click="editingLink = link"
                >
                  {{ t('common.edit') }}
                </button>
                <button
                  class="btn btn-secondary px-3 py-2 text-xs"
                  :disabled="rotatingToken === link.token"
                  @click="handleRotate(link.token)"
                >
                  {{ rotatingToken === link.token ? t('common.processing') : t('admin.accounts.publicAddLinks.rotate') }}
                </button>
                <button
                  class="btn btn-danger px-3 py-2 text-xs"
                  :disabled="deletingToken === link.token"
                  @click="handleDelete(link.token)"
                >
                  {{ deletingToken === link.token ? t('common.processing') : t('common.delete') }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>

    <OpenAIPublicLinkFormModal
      :show="showCreateModal"
      mode="create"
      :groups="groups"
      :proxies="proxies"
      @close="showCreateModal = false"
      @saved="handleCreated"
    />
    <OpenAIPublicLinkFormModal
      :show="!!editingLink"
      mode="edit"
      :groups="groups"
      :proxies="proxies"
      :initial-link="editingLink"
      @close="editingLink = null"
      @saved="handleUpdated"
    />
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import OpenAIPublicLinkFormModal from '@/components/admin/account/OpenAIPublicLinkFormModal.vue'
import { useClipboard } from '@/composables/useClipboard'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores'
import type { AdminGroup, Proxy as AccountProxy } from '@/types'
import type { OpenAIPublicAddLink, OpenAIPublicAddLinkAccountDefaults } from '@/api/admin/accounts'

interface Props {
  show: boolean
  groups: AdminGroup[]
  proxies: AccountProxy[]
}

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const appStore = useAppStore()

const loading = ref(false)
const rotatingToken = ref('')
const deletingToken = ref('')
const links = ref<OpenAIPublicAddLink[]>([])
const showCreateModal = ref(false)
const editingLink = ref<OpenAIPublicAddLink | null>(null)

const proxyMap = computed(() => {
  const map = new Map<number, AccountProxy>()
  props.proxies.forEach((proxy) => map.set(proxy.id, proxy))
  return map
})

const groupMap = computed(() => {
  const map = new Map<number, AdminGroup>()
  props.groups.forEach((group) => map.set(group.id, group))
  return map
})

const loadLinks = async () => {
  loading.value = true
  try {
    links.value = await adminAPI.accounts.listOpenAIPublicAddLinks()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.publicAddLinks.loadFailed'))
  } finally {
    loading.value = false
  }
}

const handleCreated = (created: OpenAIPublicAddLink) => {
  links.value = [created, ...links.value]
  showCreateModal.value = false
}

const handleUpdated = (updated: OpenAIPublicAddLink) => {
  links.value = links.value.map((item) => (item.token === updated.token ? updated : item))
  editingLink.value = null
}

const handleRotate = async (token: string) => {
  rotatingToken.value = token
  try {
    const updated = await adminAPI.accounts.rotateOpenAIPublicAddLink(token)
    links.value = links.value.map((item) => (item.token === token ? updated : item))
    if (editingLink.value?.token === token) {
      editingLink.value = updated
    }
    await copyToClipboard(updated.url, t('admin.accounts.publicAddLinks.rotatedSuccess'))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.publicAddLinks.rotateFailed'))
  } finally {
    rotatingToken.value = ''
  }
}

const handleDelete = async (token: string) => {
  if (!window.confirm(t('admin.accounts.publicAddLinks.deleteConfirm'))) {
    return
  }
  deletingToken.value = token
  try {
    await adminAPI.accounts.deleteOpenAIPublicAddLink(token)
    links.value = links.value.filter((item) => item.token !== token)
    if (editingLink.value?.token === token) {
      editingLink.value = null
    }
    appStore.showSuccess(t('admin.accounts.publicAddLinks.deletedSuccess'))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.publicAddLinks.deleteFailed'))
  } finally {
    deletingToken.value = ''
  }
}

const resolveGroups = (groupIds: number[]): AdminGroup[] =>
  groupIds
    .map((groupId) => groupMap.value.get(groupId))
    .filter((group): group is AdminGroup => !!group)

const resolveMissingGroupIds = (groupIds: number[]): number[] =>
  groupIds.filter((groupId) => !groupMap.value.has(groupId))

const formatUnixTime = (value: number | null | undefined): string => {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}

const modelMappingCount = (defaults?: OpenAIPublicAddLinkAccountDefaults | null): number => {
  const mapping = defaults?.credentials?.model_mapping
  if (!mapping || typeof mapping !== 'object') return 0
  return Object.keys(mapping as Record<string, unknown>).length
}

const linkConfigSummary = (link: OpenAIPublicAddLink): string[] => {
  const defaults = link.account_defaults
  if (!defaults) return []

  const items: string[] = []
  if (defaults.proxy_id != null) {
    const proxyName = proxyMap.value.get(defaults.proxy_id)?.name || `#${defaults.proxy_id}`
    items.push(`${t('admin.accounts.proxy')}: ${proxyName}`)
  }
  if (defaults.concurrency != null) {
    items.push(`${t('admin.accounts.concurrency')}: ${defaults.concurrency}`)
  }
  if (defaults.load_factor != null) {
    items.push(`${t('admin.accounts.loadFactor')}: ${defaults.load_factor}`)
  }
  if (defaults.priority != null) {
    items.push(`${t('admin.accounts.priority')}: ${defaults.priority}`)
  }
  if (defaults.rate_multiplier != null) {
    items.push(`${t('admin.accounts.billingRateMultiplier')}: ${defaults.rate_multiplier}`)
  }
  if (defaults.expires_at != null) {
    items.push(`${t('admin.accounts.expiresAt')}: ${formatUnixTime(defaults.expires_at)}`)
  }
  if (defaults.auto_pause_on_expired === false) {
    items.push(`${t('admin.accounts.autoPauseOnExpired')}: ${t('common.disabled')}`)
  }
  if (defaults.extra?.openai_passthrough) {
    items.push(t('admin.accounts.openai.oauthPassthrough'))
  }
  if (defaults.extra?.codex_cli_only) {
    items.push(t('admin.accounts.openai.codexCLIOnly'))
  }
  const wsMode = defaults.extra?.openai_oauth_responses_websockets_v2_mode
  if (typeof wsMode === 'string' && wsMode) {
    items.push(`${t('admin.accounts.openai.wsMode')}: ${wsMode}`)
  }
  const mappingCount = modelMappingCount(defaults)
  if (mappingCount > 0) {
    items.push(`${t('admin.accounts.modelRestriction')}: ${mappingCount}`)
  }

  return items
}

const formatTime = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return date.toLocaleString()
}

watch(
  () => props.show,
  (show) => {
    if (show) {
      loadLinks()
      return
    }
    showCreateModal.value = false
    editingLink.value = null
  }
)
</script>
