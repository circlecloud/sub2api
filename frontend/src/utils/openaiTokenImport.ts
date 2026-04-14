const OPENAI_CODEX_CLIENT_ID = 'app_EMoamEEZ73f0CkXaXp7hrann'
const OPENAI_MOBILE_CLIENT_ID = 'app_LlGpXReQgckcGGUo2JrYvtJK'

type ImportMode = 'refresh_token' | 'mobile_refresh_token' | 'credentials'

export interface ParsedOpenAITokenImport {
  mode: ImportMode
  email: string
  displayType: string
  clientId: string
  refreshToken: string
  credentials: Record<string, unknown>
  extra?: Record<string, unknown>
}

const stringKeys = {
  accessToken: ['access_token', 'accessToken', 'at'],
  refreshToken: ['refresh_token', 'refreshToken', 'rt'],
  idToken: ['id_token', 'idToken'],
  email: ['email'],
  name: ['name'],
  accountId: ['chatgpt_account_id', 'account_id', 'accountId'],
  userId: ['chatgpt_user_id', 'user_id', 'userId'],
  organizationId: ['organization_id', 'organizationId', 'poid'],
  planType: ['plan_type', 'planType', 'chatgpt_plan_type'],
  privacyMode: ['privacy_mode', 'privacyMode'],
  clientId: ['client_id', 'clientId'],
  expiresAt: ['expires_at', 'expiresAt', 'expired', 'expires', 'expiry'],
  type: ['type']
} as const

const getNormalizedKey = (key: string): string => key.replace(/[^a-zA-Z0-9]/g, '').toLowerCase()

const findFirstString = (value: unknown, candidates: readonly string[]): string => {
  const candidateSet = new Set(candidates.map(getNormalizedKey))
  const queue: unknown[] = [value]

  while (queue.length > 0) {
    const current = queue.shift()
    if (!current || typeof current !== 'object') continue

    if (Array.isArray(current)) {
      queue.push(...current)
      continue
    }

    for (const [key, fieldValue] of Object.entries(current as Record<string, unknown>)) {
      if (typeof fieldValue === 'string' && candidateSet.has(getNormalizedKey(key)) && fieldValue.trim()) {
        return fieldValue.trim()
      }
      if (fieldValue && typeof fieldValue === 'object') {
        queue.push(fieldValue)
      }
    }
  }

  return ''
}

const inferClientId = (typeValue: string, explicitClientId: string): string => {
  if (explicitClientId) return explicitClientId
  const normalizedType = typeValue.trim().toLowerCase()
  if (
    normalizedType.includes('mobile') ||
    normalizedType.includes('ios') ||
    normalizedType.includes('android') ||
    normalizedType.includes('sora')
  ) {
    return OPENAI_MOBILE_CLIENT_ID
  }
  return OPENAI_CODEX_CLIENT_ID
}

const inferDisplayType = (typeValue: string, mode: ImportMode): string => {
  const normalizedType = typeValue.trim().toLowerCase()
  if (normalizedType === 'codex') return 'Codex'
  if (normalizedType) return normalizedType
  if (mode === 'mobile_refresh_token') return 'mobile'
  if (mode === 'refresh_token') return 'codex'
  return 'token-file'
}

const parseMaybeJSON = (raw: string): unknown => {
  const trimmed = raw.trim()
  if (!trimmed) return null

  try {
    return JSON.parse(trimmed)
  } catch {
    const firstBrace = trimmed.indexOf('{')
    const lastBrace = trimmed.lastIndexOf('}')
    if (firstBrace >= 0 && lastBrace > firstBrace) {
      return JSON.parse(trimmed.slice(firstBrace, lastBrace + 1))
    }
    return null
  }
}

const parseKeyValueText = (raw: string): Record<string, string> => {
  const result: Record<string, string> = {}
  raw
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .forEach((line) => {
      const match = line.match(/^([a-zA-Z0-9_\-.]+)\s*[:=]\s*(.+)$/)
      if (!match) return
      const [, key, value] = match
      result[key] = value.trim().replace(/^['"`]+|['"`]+$/g, '')
    })
  return result
}

export function parseOpenAITokenImport(raw: string): ParsedOpenAITokenImport {
  const trimmed = raw.trim()
  if (!trimmed) {
    throw new Error('文件内容为空')
  }

  let source: unknown = parseMaybeJSON(trimmed)
  if (!source) {
    if (!trimmed.includes('\n') && !trimmed.includes('{') && !trimmed.includes('}')) {
      const refreshToken = trimmed
      return {
        mode: 'refresh_token',
        email: '',
        displayType: 'codex',
        clientId: OPENAI_CODEX_CLIENT_ID,
        refreshToken,
        credentials: {
          refresh_token: refreshToken,
          client_id: OPENAI_CODEX_CLIENT_ID
        }
      }
    }
    source = parseKeyValueText(trimmed)
  }

  const accessToken = findFirstString(source, stringKeys.accessToken)
  const refreshToken = findFirstString(source, stringKeys.refreshToken)
  const idToken = findFirstString(source, stringKeys.idToken)
  const email = findFirstString(source, stringKeys.email)
  const name = findFirstString(source, stringKeys.name)
  const accountId = findFirstString(source, stringKeys.accountId)
  const userId = findFirstString(source, stringKeys.userId)
  const organizationId = findFirstString(source, stringKeys.organizationId)
  const planType = findFirstString(source, stringKeys.planType)
  const privacyMode = findFirstString(source, stringKeys.privacyMode)
  const explicitClientId = findFirstString(source, stringKeys.clientId)
  const expiresAt = findFirstString(source, stringKeys.expiresAt)
  const typeValue = findFirstString(source, stringKeys.type)

  if (!accessToken && !refreshToken) {
    throw new Error('未在文件中找到 access_token 或 refresh_token')
  }

  const clientId = inferClientId(typeValue, explicitClientId)
  const mode: ImportMode = refreshToken
    ? clientId === OPENAI_MOBILE_CLIENT_ID
      ? 'mobile_refresh_token'
      : 'refresh_token'
    : 'credentials'

  const credentials: Record<string, unknown> = {
    client_id: clientId
  }

  if (accessToken) credentials.access_token = accessToken
  if (refreshToken) credentials.refresh_token = refreshToken
  if (idToken) credentials.id_token = idToken
  if (email) credentials.email = email
  if (accountId) credentials.chatgpt_account_id = accountId
  if (userId) credentials.chatgpt_user_id = userId
  if (organizationId) credentials.organization_id = organizationId
  if (planType) credentials.plan_type = planType
  if (expiresAt) credentials.expires_at = expiresAt

  const extra: Record<string, unknown> = {}
  if (name) extra.name = name
  if (privacyMode) extra.privacy_mode = privacyMode

  return {
    mode,
    email,
    displayType: inferDisplayType(typeValue, mode),
    clientId,
    refreshToken,
    credentials,
    extra: Object.keys(extra).length > 0 ? extra : undefined
  }
}

export const openAITokenImportConstants = {
  codexClientId: OPENAI_CODEX_CLIENT_ID,
  mobileClientId: OPENAI_MOBILE_CLIENT_ID
}
