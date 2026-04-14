import { describe, expect, it } from 'vitest'
import { parseOpenAITokenImport, openAITokenImportConstants } from '@/utils/openaiTokenImport'

describe('parseOpenAITokenImport', () => {
  it('parses Codex token JSON and prefers refresh token mode', () => {
    const result = parseOpenAITokenImport(
      JSON.stringify({
        access_token: 'at-codex',
        refresh_token: 'rt-codex',
        account_id: 'acc-1',
        email: 'user@example.com',
        expired: '2026-04-09T14:23:38+08:00',
        type: 'codex'
      })
    )

    expect(result.mode).toBe('refresh_token')
    expect(result.clientId).toBe(openAITokenImportConstants.codexClientId)
    expect(result.refreshToken).toBe('rt-codex')
    expect(result.credentials).toMatchObject({
      access_token: 'at-codex',
      refresh_token: 'rt-codex',
      email: 'user@example.com',
      chatgpt_account_id: 'acc-1',
      expires_at: '2026-04-09T14:23:38+08:00'
    })
  })

  it('infers mobile refresh token mode from type', () => {
    const result = parseOpenAITokenImport(
      JSON.stringify({
        refresh_token: 'rt-mobile',
        type: 'mobile'
      })
    )

    expect(result.mode).toBe('mobile_refresh_token')
    expect(result.clientId).toBe(openAITokenImportConstants.mobileClientId)
    expect(result.credentials).toMatchObject({
      refresh_token: 'rt-mobile',
      client_id: openAITokenImportConstants.mobileClientId
    })
  })

  it('falls back to direct credentials when only access token exists', () => {
    const result = parseOpenAITokenImport(
      JSON.stringify({
        access_token: 'at-only',
        email: 'direct@example.com'
      })
    )

    expect(result.mode).toBe('credentials')
    expect(result.credentials).toMatchObject({
      access_token: 'at-only',
      email: 'direct@example.com',
      client_id: openAITokenImportConstants.codexClientId
    })
  })
})
