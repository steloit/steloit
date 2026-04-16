/**
 * API Keys API Client
 * CRUD operations for API key management
 */

import { BrokleAPIClient } from '@/lib/api/core/client'
import type { PaginatedResponse } from '@/lib/api/core/types'
import type {
  APIKey,
  CreateAPIKeyRequest,
  APIKeyFilters,
  BackendAPIKey,
} from '../types/api-keys'

// Initialize API client with /api base path (Dashboard routes)
const client = new BrokleAPIClient('/api')

/**
 * Map backend API key format (snake_case) to frontend format (camelCase)
 */
function mapAPIKeyToFrontend(backendKey: BackendAPIKey): APIKey {
  return {
    id: backendKey.id,
    name: backendKey.name,
    key: backendKey.key, // Only present on creation
    key_preview: backendKey.key_preview,
    project_id: backendKey.project_id,
    status: backendKey.status,
    last_used: backendKey.last_used,
    created_at: backendKey.created_at,
    expires_at: backendKey.expires_at,
    created_by: backendKey.created_by,
  }
}

/**
 * List API keys for a project with optional filters and pagination
 *
 * @param projectId - Project UUID
 * @param filters - Optional filters (status, pagination, sorting)
 * @returns Paginated list of API keys
 *
 * @example
 * ```ts
 * const result = await listAPIKeys('proj_123', {
 *   status: 'active',
 *   page: 1,
 *   limit: 50,
 *   sort_by: 'created_at',
 *   sort_dir: 'desc'
 * })
 * ```
 */
export async function listAPIKeys(
  projectId: string,
  filters?: APIKeyFilters
): Promise<PaginatedResponse<APIKey>> {
  const endpoint = `/v1/projects/${projectId}/api-keys`

  // Build query params
  const params: Record<string, string | number | undefined> = {
    status: filters?.status,
    page: filters?.page,
    limit: filters?.limit,
    sort_by: filters?.sort_by,
    sort_dir: filters?.sort_dir,
  }

  // Remove undefined values
  Object.keys(params).forEach(key => {
    if (params[key] === undefined) {
      delete params[key]
    }
  })

  const response = await client.getPaginated<APIKey>(endpoint, params)

  // Map backend format to frontend format for each key
  return {
    data: response.data.map(mapAPIKeyToFrontend),
    pagination: response.pagination,
  }
}

/**
 * Create a new API key for a project
 *
 * IMPORTANT: The full `key` field is ONLY returned once during creation.
 * Store it securely as it cannot be retrieved again.
 *
 * @param projectId - Project UUID
 * @param data - API key creation data (name, expiry_option)
 * @returns Created API key with full key (one-time view)
 *
 * @example
 * ```ts
 * const newKey = await createAPIKey('proj_123', {
 *   name: 'Production API Key',
 *   expiry_option: '90days'
 * })
 * console.log(newKey.key) // bk_AbCdEfGh... (full key, only shown once)
 * ```
 */
export async function createAPIKey(
  projectId: string,
  data: CreateAPIKeyRequest
): Promise<APIKey> {
  const endpoint = `/v1/projects/${projectId}/api-keys`

  const response = await client.post<BackendAPIKey, CreateAPIKeyRequest>(
    endpoint,
    data
  )

  return mapAPIKeyToFrontend(response)
}

/**
 * Delete an API key permanently
 *
 * This action cannot be undone. The API key will be immediately revoked
 * and all requests using this key will fail.
 *
 * @param projectId - Project UUID
 * @param keyId - API key ID (UUID)
 * @returns void (204 No Content)
 *
 * @example
 * ```ts
 * await deleteAPIKey('proj_123', 'key_456')
 * ```
 */
export async function deleteAPIKey(
  projectId: string,
  keyId: string
): Promise<void> {
  const endpoint = `/v1/projects/${projectId}/api-keys/${keyId}`

  await client.delete<void>(endpoint)
}

/**
 * Utility function to create a key preview from a full key
 * Format: bk_AbCd...XyZa (first 4 + last 4 chars of secret)
 *
 * @param fullKey - Full API key (bk_{40_char})
 * @returns Key preview string
 *
 * @example
 * ```ts
 * const preview = createKeyPreview('bk_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789AbCd')
 * console.log(preview) // "bk_AbCd...AbCd"
 * ```
 */
export function createKeyPreview(fullKey: string): string {
  if (fullKey.length <= 11) {
    return fullKey + '...'
  }
  // Show: bk_ + first 4 chars of secret + ... + last 4 chars
  return fullKey.substring(0, 7) + '...' + fullKey.substring(fullKey.length - 4)
}

/**
 * Validate API key format (client-side validation)
 * Format: bk_{40_char_alphanumeric}
 *
 * @param key - API key to validate
 * @returns true if valid format, false otherwise
 *
 * @example
 * ```ts
 * validateAPIKeyFormat('bk_AbCdEfGh...') // true
 * validateAPIKeyFormat('invalid') // false
 * ```
 */
export function validateAPIKeyFormat(key: string): boolean {
  // Format: bk_{40_char}
  const parts = key.split('_')
  if (parts.length !== 2) return false
  if (parts[0] !== 'bk') return false
  if (parts[1].length !== 40) return false

  // Check alphanumeric only
  const alphanumericRegex = /^[a-zA-Z0-9]+$/
  return alphanumericRegex.test(parts[1])
}
