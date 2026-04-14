<template>
  <AuthLayout max-width="7xl" vertical-align="top" card-padding="compact">
    <div class="space-y-6">
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('admin.accounts.publicAddPage.title') }}
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('admin.accounts.publicAddPage.subtitle') }}
        </p>
      </div>

      <div
        v-if="pageError"
        class="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-800/50 dark:bg-red-900/20 dark:text-red-300"
      >
        {{ pageError }}
      </div>

      <template v-else>
        <div
          v-if="successState.count > 0"
          class="space-y-4 rounded-xl border border-emerald-200 bg-emerald-50 p-5 dark:border-emerald-800/50 dark:bg-emerald-900/20"
        >
          <div class="flex items-start gap-3">
            <Icon name="checkCircle" size="lg" class="mt-0.5 text-emerald-500" />
            <div>
              <h3 class="text-base font-semibold text-emerald-900 dark:text-emerald-200">
                {{ t('admin.accounts.publicAddPage.successTitle') }}
              </h3>
              <p v-if="successState.count > 1" class="mt-1 text-sm text-emerald-700 dark:text-emerald-300">
                {{ t('admin.accounts.publicAddPage.successBatchDesc', { count: successState.count }) }}
              </p>
              <p v-else class="mt-1 text-sm text-emerald-700 dark:text-emerald-300">
                {{ t('admin.accounts.publicAddPage.successDesc', { name: successState.name }) }}
              </p>
            </div>
          </div>
          <button class="btn btn-secondary" @click="resetAfterSuccess">
            {{ t('admin.accounts.publicAddPage.addAnother') }}
          </button>
        </div>

        <div class="space-y-6">
          <div class="space-y-4 rounded-xl border border-gray-200 p-5 dark:border-dark-700">
            <div class="flex flex-col gap-2 lg:flex-row lg:items-end lg:justify-between">
              <div>
                <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.accounts.publicAddPage.groupLabel') }}
                </h3>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.publicAddPage.groupLayoutHint') }}
                </p>
                <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
                  {{ t('admin.accounts.publicAddPage.autoNameHint') }}
                </p>
              </div>
              <span class="text-xs text-gray-400">
                {{ t('admin.accounts.publicAddPage.fixedGroupCount', { count: allowedGroups.length }) }}
              </span>
            </div>

            <div>
              <label class="input-label">{{ t('admin.accounts.publicAddPage.groupLabel') }}</label>
              <div
                class="grid max-h-80 grid-cols-1 gap-3 overflow-y-auto rounded-xl border border-gray-200 bg-gray-50 p-3 lg:grid-cols-2 2xl:grid-cols-3 dark:border-dark-700 dark:bg-dark-800"
              >
                <div
                  v-for="group in allowedGroups"
                  :key="group.id"
                  class="rounded-xl border border-transparent bg-white px-3 py-3 dark:bg-dark-700"
                >
                  <GroupBadge
                    :name="group.name"
                    :platform="group.platform"
                    :subscription-type="group.subscription_type"
                    :rate-multiplier="group.rate_multiplier"
                    class="min-w-0"
                  />
                </div>
                <div
                  v-if="loadingGroups"
                  class="rounded-xl border border-dashed border-gray-200 px-3 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400 lg:col-span-2 2xl:col-span-3"
                >
                  {{ t('common.loading') }}
                </div>
                <div
                  v-else-if="allowedGroups.length === 0"
                  class="rounded-xl border border-dashed border-gray-200 px-3 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400 lg:col-span-2 2xl:col-span-3"
                >
                  {{ t('admin.accounts.publicAddPage.emptyGroups') }}
                </div>
              </div>
            </div>
          </div>

          <OAuthAuthorizationFlow
            ref="oauthFlowRef"
            :add-method="openaiAddMethod"
            :auth-url="authUrl"
            :session-id="sessionId"
            :loading="oauthLoading || submitting"
            :error="oauthError"
            :show-cookie-option="false"
            :show-refresh-token-option="true"
            :show-mobile-refresh-token-option="true"
            :show-token-file-option="true"
            :show-session-token-option="false"
            :show-access-token-option="false"
            :token-file-preview-items="tokenFilePreviewItems"
            :token-file-ready-count="importReadyCount"
            :token-file-invalid-count="importInvalidCount"
            :token-file-created-count="importCreatedCount"
            :token-file-busy="tokenFileBusy"
            :token-file-progress-visible="tokenFileProgress.visible"
            :token-file-progress-phase="tokenFileProgress.phase"
            :token-file-progress-current="tokenFileProgress.current"
            :token-file-progress-total="tokenFileProgress.total"
            platform="openai"
            @generate-url="handleGenerateUrl"
            @exchange-code="handleExchangeCode"
            @validate-refresh-token="handleValidateRefreshToken"
            @validate-mobile-refresh-token="handleValidateMobileRefreshToken"
            @import-token-files="handleImportTokenFiles"
            @remove-token-file-item="removeImportItem"
            @clear-token-file-items="clearImportItems"
            @confirm-token-file-create="handleConfirmImportCreate"
          />
        </div>

      </template>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import Icon from '@/components/icons/Icon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import OAuthAuthorizationFlow from '@/components/account/OAuthAuthorizationFlow.vue'
import { openaiPublicAPI } from '@/api'
import { useAppStore } from '@/stores'
import type { AddMethod, OAuthTokenFilePreviewItem } from '@/composables/useAccountOAuth'
import type { Account, Group } from '@/types'
import {
  parseOpenAITokenImport,
  openAITokenImportConstants,
  type ParsedOpenAITokenImport
} from '@/utils/openaiTokenImport'

interface OAuthFlowExposed {
  authCode: string
  oauthState: string
  reset: () => void
}

interface ImportedTokenPreviewItem {
  id: string
  fileName: string
  parsed: ParsedOpenAITokenImport | null
  error: string
  createdAccount: Account | null
}

type TokenFileProgressPhase = 'parsing' | 'creating'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()

const openaiAddMethod: AddMethod = 'oauth'
const oauthFlowRef = ref<OAuthFlowExposed | null>(null)

const token = ref('')
const loadingGroups = ref(false)
const oauthLoading = ref(false)
const submitting = ref(false)
const importParsing = ref(false)
const pageError = ref('')
const oauthError = ref('')
const authUrl = ref('')
const sessionId = ref('')
const generatedState = ref('')
const allowedGroups = ref<Group[]>([])
const importItems = ref<ImportedTokenPreviewItem[]>([])
const successState = reactive({
  count: 0,
  name: ''
})
const tokenFileProgress = reactive<{
  visible: boolean
  phase: TokenFileProgressPhase
  current: number
  total: number
}>({
  visible: false,
  phase: 'parsing',
  current: 0,
  total: 0
})

const createPreviewId = (): string => `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`

const importReadyItems = computed(() =>
  importItems.value.filter((item) => item.parsed && !item.createdAccount)
)
const importReadyCount = computed(() => importReadyItems.value.length)
const importInvalidCount = computed(() => importItems.value.filter((item) => !item.parsed).length)
const importCreatedCount = computed(() => importItems.value.filter((item) => item.createdAccount).length)
const linkedGroupIds = computed(() => allowedGroups.value.map((group) => group.id))
const tokenFileBusy = computed(() => importParsing.value || submitting.value)
const tokenFilePreviewItems = computed<OAuthTokenFilePreviewItem[]>(() =>
  importItems.value.map((item) => ({
    id: item.id,
    fileName: item.fileName,
    modeLabel: item.parsed ? previewModeText(item.parsed.mode) : '',
    clientId: item.parsed?.clientId,
    email: item.parsed?.email,
    status: item.createdAccount ? 'created' : !item.parsed ? 'invalid' : item.error ? 'failed' : 'ready',
    error: item.error || undefined,
    createdAccountName: item.createdAccount?.name
  }))
)

const parseStateFromAuthUrl = (value: string): string => {
  try {
    return new URL(value).searchParams.get('state') || ''
  } catch {
    return ''
  }
}

const splitNonEmptyLines = (input: string): string[] =>
  input
    .split('\n')
    .map((item) => item.trim())
    .filter(Boolean)

const updateSuccessState = (accounts: Account[]) => {
  successState.count = accounts.length
  successState.name = accounts[accounts.length - 1]?.name || ''
}

const resetTokenFileProgress = () => {
  tokenFileProgress.visible = false
  tokenFileProgress.phase = 'parsing'
  tokenFileProgress.current = 0
  tokenFileProgress.total = 0
}

const beginTokenFileProgress = (phase: TokenFileProgressPhase, total: number) => {
  tokenFileProgress.visible = total > 0
  tokenFileProgress.phase = phase
  tokenFileProgress.current = 0
  tokenFileProgress.total = total
}

const advanceTokenFileProgress = (current: number) => {
  tokenFileProgress.current = Math.min(current, tokenFileProgress.total)
}

const ensureLinkedGroupsAvailable = (): boolean => {
  if (linkedGroupIds.value.length > 0) return true
  const message = t('admin.accounts.publicAddPage.emptyGroups')
  oauthError.value = message
  appStore.showError(message)
  return false
}

const loadAllowedGroups = async () => {
  if (!token.value) {
    pageError.value = t('admin.accounts.publicAddPage.invalidLink')
    return
  }
  loadingGroups.value = true
  pageError.value = ''
  try {
    const groups = await openaiPublicAPI.getAllowedGroups(token.value)
    allowedGroups.value = groups
  } catch (error: any) {
    allowedGroups.value = []
    pageError.value = error?.message || t('admin.accounts.publicAddPage.invalidLink')
  } finally {
    loadingGroups.value = false
  }
}

const handleGenerateUrl = async () => {
  oauthLoading.value = true
  oauthError.value = ''
  authUrl.value = ''
  sessionId.value = ''
  generatedState.value = ''
  try {
    const result = await openaiPublicAPI.generateAuthUrl(token.value)
    authUrl.value = result.auth_url
    sessionId.value = result.session_id
    generatedState.value = parseStateFromAuthUrl(result.auth_url)
  } catch (error: any) {
    oauthError.value = error?.message || t('admin.accounts.publicAddPage.generateUrlFailed')
  } finally {
    oauthLoading.value = false
  }
}

const handleExchangeCode = async (code: string) => {
  const trimmedCode = code.trim() || oauthFlowRef.value?.authCode?.trim() || ''
  const state = oauthFlowRef.value?.oauthState?.trim() || generatedState.value
  if (!trimmedCode || !sessionId.value || !state) {
    oauthError.value = t('admin.accounts.publicAddPage.missingAuthInfo')
    return
  }
  if (!ensureLinkedGroupsAvailable()) return

  submitting.value = true
  oauthError.value = ''
  try {
    const account = await openaiPublicAPI.createFromOAuth(token.value, {
      session_id: sessionId.value,
      code: trimmedCode,
      state
    })
    updateSuccessState([account])
    appStore.showSuccess(t('admin.accounts.publicAddPage.successToast'))
  } catch (error: any) {
    oauthError.value = error?.message || t('admin.accounts.publicAddPage.createFailed')
  } finally {
    submitting.value = false
  }
}

const createFromRefreshTokens = async (refreshTokenInput: string, clientId?: string) => {
  const refreshTokens = splitNonEmptyLines(refreshTokenInput)
  if (refreshTokens.length === 0) {
    oauthError.value = t('admin.accounts.oauth.openai.pleaseEnterRefreshToken')
    return
  }
  if (!ensureLinkedGroupsAvailable()) return

  submitting.value = true
  oauthError.value = ''
  const created: Account[] = []
  const errors: string[] = []

  try {
    for (const [index, refreshToken] of refreshTokens.entries()) {
      try {
        const account = await openaiPublicAPI.createFromRefreshToken(token.value, {
          refresh_token: refreshToken,
          client_id: clientId
        })
        created.push(account)
      } catch (error: any) {
        errors.push(`#${index + 1}: ${error?.message || t('admin.accounts.publicAddPage.createFailed')}`)
      }
    }

    if (created.length > 0) {
      updateSuccessState(created)
      appStore.showSuccess(
        created.length > 1
          ? t('admin.accounts.oauth.batchSuccess', { count: created.length })
          : t('admin.accounts.publicAddPage.successToast')
      )
    }

    if (errors.length > 0) {
      oauthError.value = errors.join('\n')
      if (created.length > 0) {
        appStore.showWarning(
          t('admin.accounts.oauth.batchPartialSuccess', { success: created.length, failed: errors.length })
        )
      } else {
        appStore.showError(t('admin.accounts.oauth.batchFailed'))
      }
    }
  } finally {
    submitting.value = false
  }
}

const handleValidateRefreshToken = async (refreshToken: string) => {
  await createFromRefreshTokens(refreshToken)
}

const handleValidateMobileRefreshToken = async (refreshToken: string) => {
  await createFromRefreshTokens(refreshToken, openAITokenImportConstants.mobileClientId)
}

const previewModeText = (mode: ParsedOpenAITokenImport['mode']): string => {
  switch (mode) {
    case 'refresh_token':
      return t('admin.accounts.publicAddPage.modeRefreshToken')
    case 'mobile_refresh_token':
      return t('admin.accounts.publicAddPage.modeMobileRefreshToken')
    case 'credentials':
      return t('admin.accounts.publicAddPage.modeCredentials')
  }
}

const addImportedFiles = async (files: File[]) => {
  const nextItems: ImportedTokenPreviewItem[] = []
  importParsing.value = true
  beginTokenFileProgress('parsing', files.length)

  try {
    for (const [index, file] of files.entries()) {
      try {
        const raw = await file.text()
        const parsed = parseOpenAITokenImport(raw)
        nextItems.push({
          id: createPreviewId(),
          fileName: file.name,
          parsed,
          error: '',
          createdAccount: null
        })
      } catch (error: any) {
        nextItems.push({
          id: createPreviewId(),
          fileName: file.name,
          parsed: null,
          error: error?.message || t('admin.accounts.publicAddPage.importReadFailed'),
          createdAccount: null
        })
      } finally {
        advanceTokenFileProgress(index + 1)
      }
    }

    importItems.value = [...importItems.value, ...nextItems]
  } finally {
    importParsing.value = false
  }
}

const handleImportTokenFiles = async (files: File[]) => {
  if (files.length === 0) return
  await addImportedFiles(files)
}

const removeImportItem = (id: string) => {
  importItems.value = importItems.value.filter((item) => item.id !== id)
  if (importItems.value.length === 0 && !tokenFileBusy.value) {
    resetTokenFileProgress()
  }
}

const clearImportItems = () => {
  importItems.value = []
  if (!tokenFileBusy.value) {
    resetTokenFileProgress()
  }
}

const handleConfirmImportCreate = async () => {
  if (!ensureLinkedGroupsAvailable()) return
  const pendingItems = importReadyItems.value
  if (pendingItems.length === 0) {
    appStore.showError(t('admin.accounts.publicAddPage.importNoValidFiles'))
    return
  }

  submitting.value = true
  beginTokenFileProgress('creating', pendingItems.length)
  const created: Account[] = []
  let failedCount = 0

  try {
    for (const [index, item] of pendingItems.entries()) {
      const parsed = item.parsed
      if (!parsed) {
        advanceTokenFileProgress(index + 1)
        continue
      }
      item.error = ''
      try {
        if (parsed.mode === 'credentials') {
          item.createdAccount = await openaiPublicAPI.createFromCredentials(token.value, {
            credentials: parsed.credentials,
            extra: parsed.extra
          })
        } else {
          item.createdAccount = await openaiPublicAPI.createFromRefreshToken(token.value, {
            refresh_token: parsed.refreshToken,
            client_id: parsed.clientId
          })
        }
        created.push(item.createdAccount)
      } catch (error: any) {
        failedCount += 1
        item.error = error?.message || t('admin.accounts.publicAddPage.importCreateFailed')
      } finally {
        advanceTokenFileProgress(index + 1)
      }
    }

    if (created.length > 0) {
      updateSuccessState(created)
      appStore.showSuccess(
        created.length > 1
          ? t('admin.accounts.oauth.batchSuccess', { count: created.length })
          : t('admin.accounts.publicAddPage.successToast')
      )
    }

    if (failedCount > 0) {
      if (created.length > 0) {
        appStore.showWarning(
          t('admin.accounts.oauth.batchPartialSuccess', { success: created.length, failed: failedCount })
        )
      } else {
        appStore.showError(t('admin.accounts.oauth.batchFailed'))
      }
    }
  } finally {
    submitting.value = false
  }
}

const resetAfterSuccess = () => {
  successState.count = 0
  successState.name = ''
  authUrl.value = ''
  sessionId.value = ''
  generatedState.value = ''
  oauthError.value = ''
  importItems.value = []
  resetTokenFileProgress()
  oauthFlowRef.value?.reset()
}

watch(
  () => route.params.token,
  (value) => {
    token.value = typeof value === 'string' ? value : ''
    resetAfterSuccess()
    loadAllowedGroups()
  },
  { immediate: true }
)

onMounted(() => {
  if (!allowedGroups.value.length && !loadingGroups.value) {
    loadAllowedGroups()
  }
})
</script>
