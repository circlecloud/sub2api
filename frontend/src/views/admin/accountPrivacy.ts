import type { Account } from '@/types'

const OPENAI_PRIVACY_MODE = 'training_off'
const ANTIGRAVITY_PRIVACY_MODE = 'privacy_set'

export function readAccountPrivacyMode(account: Pick<Account, 'extra'> | null | undefined): string {
  const raw = account?.extra?.privacy_mode
  return typeof raw === 'string' ? raw.trim() : ''
}

export function isAccountPrivacyApplied(account: Pick<Account, 'platform' | 'extra'> | null | undefined): boolean {
  if (!account) return false

  const mode = readAccountPrivacyMode(account)
  switch (account.platform) {
    case 'openai':
      return mode === OPENAI_PRIVACY_MODE
    case 'antigravity':
      return mode === ANTIGRAVITY_PRIVACY_MODE
    default:
      return false
  }
}
