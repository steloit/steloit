/**
 * AI Providers API Client
 * CRUD operations and connection testing for AI provider credentials
 *
 * Note: AI provider credentials are organization-scoped, not project-scoped.
 * This allows sharing credentials across all projects in an organization.
 */

import { BrokleAPIClient } from '@/lib/api/core/client'
import type {
  AIProviderCredential,
  AvailableModel,
  CreateProviderRequest,
  UpdateProviderRequest,
  TestConnectionRequest,
  TestConnectionResponse,
} from '../types'

// Initialize API client with /api base path (Dashboard routes)
const client = new BrokleAPIClient('/api')

/**
 * List all AI provider credentials for an organization
 *
 * @param orgId - Organization UUID
 * @returns List of configured provider credentials
 *
 * @example
 * ```ts
 * const credentials = await listProviderCredentials('org_123')
 * ```
 */
export async function listProviderCredentials(
  orgId: string
): Promise<AIProviderCredential[]> {
  const endpoint = `/v1/organizations/${orgId}/credentials/ai`
  return client.get<AIProviderCredential[]>(endpoint)
}

/**
 * Get a specific AI provider credential by ID
 *
 * @param orgId - Organization UUID
 * @param credentialId - Credential UUID
 * @returns Provider credential details
 *
 * @example
 * ```ts
 * const credential = await getProviderCredential('org_123', 'cred_456')
 * ```
 */
export async function getProviderCredential(
  orgId: string,
  credentialId: string
): Promise<AIProviderCredential> {
  const endpoint = `/v1/organizations/${orgId}/credentials/ai/${credentialId}`
  return client.get<AIProviderCredential>(endpoint)
}

/**
 * Create a new AI provider credential
 *
 * IMPORTANT: The API key is encrypted at rest. Only the key_preview
 * (masked version) is returned in responses.
 *
 * @param orgId - Organization UUID
 * @param data - Provider credential data including name and adapter type
 * @returns Created credential
 *
 * @example
 * ```ts
 * const credential = await createProviderCredential('org_123', {
 *   name: 'OpenAI Production',
 *   adapter: 'openai',
 *   api_key: 'sk-...',
 * })
 * ```
 */
export async function createProviderCredential(
  orgId: string,
  data: CreateProviderRequest
): Promise<AIProviderCredential> {
  const endpoint = `/v1/organizations/${orgId}/credentials/ai`
  return client.post<AIProviderCredential, CreateProviderRequest>(endpoint, data)
}

/**
 * Update an existing AI provider credential
 *
 * @param orgId - Organization UUID
 * @param credentialId - Credential UUID to update
 * @param data - Fields to update
 * @returns Updated credential
 *
 * @example
 * ```ts
 * const credential = await updateProviderCredential('org_123', 'cred_456', {
 *   name: 'OpenAI Development',
 *   api_key: 'sk-new-key...',
 * })
 * ```
 */
export async function updateProviderCredential(
  orgId: string,
  credentialId: string,
  data: UpdateProviderRequest
): Promise<AIProviderCredential> {
  const endpoint = `/v1/organizations/${orgId}/credentials/ai/${credentialId}`
  return client.patch<AIProviderCredential, UpdateProviderRequest>(endpoint, data)
}

/**
 * Delete an AI provider credential by ID
 *
 * @param orgId - Organization UUID
 * @param credentialId - Credential UUID to delete
 *
 * @example
 * ```ts
 * await deleteProviderCredential('org_123', 'cred_456')
 * ```
 */
export async function deleteProviderCredential(
  orgId: string,
  credentialId: string
): Promise<void> {
  const endpoint = `/v1/organizations/${orgId}/credentials/ai/${credentialId}`
  await client.delete<void>(endpoint)
}

/**
 * Test a provider connection without saving credentials
 *
 * Use this to validate API keys before storing them.
 *
 * @param orgId - Organization UUID
 * @param data - Connection details to test
 * @returns Success status and optional error message
 *
 * @example
 * ```ts
 * const result = await testProviderConnection('org_123', {
 *   adapter: 'openai',
 *   api_key: 'sk-...',
 * })
 * if (result.success) {
 *   // Proceed to save
 * } else {
 *   console.error(result.error)
 * }
 * ```
 */
export async function testProviderConnection(
  orgId: string,
  data: TestConnectionRequest
): Promise<TestConnectionResponse> {
  const endpoint = `/v1/organizations/${orgId}/credentials/ai/test`
  return client.post<TestConnectionResponse, TestConnectionRequest>(endpoint, data)
}

/**
 * Create a masked preview of an API key
 * Format: first 4 chars + "***" + last 4 chars
 *
 * @param apiKey - Full API key
 * @returns Masked preview string
 *
 * @example
 * ```ts
 * createKeyPreview('sk-abcdefghijklmnop') // "sk-a***mnop"
 * ```
 */
export function createKeyPreview(apiKey: string): string {
  if (apiKey.length <= 8) {
    return apiKey.substring(0, 2) + '***'
  }
  return apiKey.substring(0, 4) + '***' + apiKey.substring(apiKey.length - 4)
}

/**
 * Get available models for an organization based on configured providers
 *
 * Returns models from:
 * - Standard providers (openai, anthropic, etc.): default models + custom_models
 * - Custom provider: only custom_models
 *
 * @param orgId - Organization UUID
 * @returns List of available models
 *
 * @example
 * ```ts
 * const models = await getAvailableModels('org_123')
 * // [
 * //   { id: 'gpt-4o', name: 'GPT-4o', provider: 'openai' },
 * //   { id: 'claude-3-5-sonnet', name: 'Claude 3.5 Sonnet', provider: 'anthropic' },
 * // ]
 * ```
 */
export async function getAvailableModels(
  orgId: string
): Promise<AvailableModel[]> {
  const endpoint = `/v1/organizations/${orgId}/credentials/ai/models`
  return client.get<AvailableModel[]>(endpoint)
}
