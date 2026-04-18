import type { OpenAIApikeyUpstreamProtocol } from '@/types'

export const OPENAI_APIKEY_UPSTREAM_PROTOCOL_RESPONSES: OpenAIApikeyUpstreamProtocol = 'responses'
export const OPENAI_APIKEY_UPSTREAM_PROTOCOL_CHAT_COMPLETIONS: OpenAIApikeyUpstreamProtocol = 'chat_completions'
export const DEFAULT_OPENAI_APIKEY_UPSTREAM_PROTOCOL = OPENAI_APIKEY_UPSTREAM_PROTOCOL_RESPONSES

export const isOpenAIApikeyChatCompletionsProtocol = (
  protocol: OpenAIApikeyUpstreamProtocol
): boolean => protocol === OPENAI_APIKEY_UPSTREAM_PROTOCOL_CHAT_COMPLETIONS

export const resolveOpenAIApikeyUpstreamProtocol = (
  extra?: Record<string, unknown> | null
): OpenAIApikeyUpstreamProtocol => {
  return extra?.openai_apikey_upstream_protocol === OPENAI_APIKEY_UPSTREAM_PROTOCOL_CHAT_COMPLETIONS
    ? OPENAI_APIKEY_UPSTREAM_PROTOCOL_CHAT_COMPLETIONS
    : OPENAI_APIKEY_UPSTREAM_PROTOCOL_RESPONSES
}
