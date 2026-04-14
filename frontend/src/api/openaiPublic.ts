import { apiClient } from './client'
import type { Account, Group } from '@/types'

export interface OpenAIPublicAuthUrlResponse {
  auth_url: string
  session_id: string
}

export interface CreateOpenAIAccountFromPublicLinkRequest {
  session_id: string
  code: string
  state: string
  redirect_uri?: string
}

export interface CreateOpenAIAccountFromPublicRefreshTokenRequest {
  refresh_token: string
  client_id?: string
}

export interface CreateOpenAIAccountFromPublicCredentialsRequest {
  credentials: Record<string, unknown>
  extra?: Record<string, unknown>
}

const buildPublicLinkPath = (token: string, suffix: string): string => {
  const encodedToken = encodeURIComponent(token.trim())
  return `/openai/public-links/${encodedToken}${suffix}`
}

export async function getOpenAIPublicAllowedGroups(token: string): Promise<Group[]> {
  const { data } = await apiClient.get<Group[]>(buildPublicLinkPath(token, '/groups'))
  return data
}

export async function generateOpenAIPublicAuthUrl(token: string): Promise<OpenAIPublicAuthUrlResponse> {
  const { data } = await apiClient.post<OpenAIPublicAuthUrlResponse>(
    buildPublicLinkPath(token, '/generate-auth-url')
  )
  return data
}

export async function createOpenAIAccountFromPublicLink(
  token: string,
  payload: CreateOpenAIAccountFromPublicLinkRequest
): Promise<Account> {
  const { data } = await apiClient.post<Account>(buildPublicLinkPath(token, '/create-from-oauth'), payload)
  return data
}

export async function createOpenAIAccountFromPublicRefreshToken(
  token: string,
  payload: CreateOpenAIAccountFromPublicRefreshTokenRequest
): Promise<Account> {
  const { data } = await apiClient.post<Account>(buildPublicLinkPath(token, '/create-from-refresh-token'), payload)
  return data
}

export async function createOpenAIAccountFromPublicCredentials(
  token: string,
  payload: CreateOpenAIAccountFromPublicCredentialsRequest
): Promise<Account> {
  const { data } = await apiClient.post<Account>(buildPublicLinkPath(token, '/create-from-credentials'), payload)
  return data
}

export const openaiPublicAPI = {
  getAllowedGroups: getOpenAIPublicAllowedGroups,
  generateAuthUrl: generateOpenAIPublicAuthUrl,
  createFromOAuth: createOpenAIAccountFromPublicLink,
  createFromRefreshToken: createOpenAIAccountFromPublicRefreshToken,
  createFromCredentials: createOpenAIAccountFromPublicCredentials
}

export default openaiPublicAPI
