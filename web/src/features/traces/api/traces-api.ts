import { BrokleAPIClient } from '@/lib/api/core/client'
import type { PaginatedResponse } from '@/lib/api/core/types'
import type { Trace, Span, Score } from '../data/schema'
import {
  transformTrace,
  transformTraceResponse,
  transformSpan,
  transformScore,
} from '../utils/transform'

const client = new BrokleAPIClient('/api')

// ============================================================================
// API Parameter Types
// ============================================================================

export interface GetTracesParams {
  projectId: string
  page?: number
  pageSize?: number
  status?: string[] // ['ok', 'error', 'unset'] (inclusion)
  statusNot?: string[] // ['ok', 'error', 'unset'] (exclusion)
  search?: string
  searchType?: string // 'id' | 'content' | 'all'
  sessionId?: string
  userId?: string
  startTime?: Date
  endTime?: Date
  sortBy?: string
  sortOrder?: 'asc' | 'desc'
  modelName?: string
  providerName?: string
  serviceName?: string
  minCost?: number
  maxCost?: number
  minTokens?: number
  maxTokens?: number
  minDuration?: number // nanoseconds
  maxDuration?: number // nanoseconds
  hasError?: boolean
}

export interface FilterRange {
  min: number
  max: number
}

export interface TraceFilterOptions {
  models: string[]
  providers: string[]
  services: string[]
  environments: string[]
  users: string[]
  sessions: string[]
  costRange: FilterRange | null
  tokenRange: FilterRange | null
  durationRange: FilterRange | null
}

export interface GetSpansParams {
  projectId: string
  traceId?: string
  type?: string
  model?: string
  level?: string
  page?: number
  pageSize?: number
  sortBy?: string
  sortOrder?: 'asc' | 'desc'
}

export interface GetScoresParams {
  projectId: string
  traceId?: string
  spanId?: string
  sessionId?: string
  name?: string
  source?: string
  dataType?: string
  page?: number
  pageSize?: number
}

export interface UpdateTraceData {
  name?: string
  tags?: string[]
  bookmarked?: boolean
  public?: boolean
}

export interface UpdateSpanData {
  span_name?: string
}

export interface UpdateScoreData {
  name?: string
  value?: number
  string_value?: string
  comment?: string
}

// ============================================================================
// Traces API
// ============================================================================

/**
 * Get all traces for a project with filtering and pagination
 *
 * Backend endpoint: GET /api/v1/traces
 *
 * @param params - Filter and pagination parameters
 * @returns Traces array with pagination metadata
 */
export const getProjectTraces = async (params: GetTracesParams): Promise<PaginatedResponse<Trace>> => {
  const {
    projectId,
    page = 1,
    pageSize = 20,
    status,
    statusNot,
    search,
    searchType,
    sessionId,
    userId,
    startTime,
    endTime,
    sortBy,
    sortOrder,
    modelName,
    providerName,
    serviceName,
    minCost,
    maxCost,
    minTokens,
    maxTokens,
    minDuration,
    maxDuration,
    hasError,
  } = params

  const queryParams: Record<string, any> = {
    project_id: projectId,
    page,
    limit: pageSize,
  }

  if (search) queryParams.search = search
  if (searchType) queryParams.search_type = searchType
  if (sessionId) queryParams.session_id = sessionId
  if (userId) queryParams.user_id = userId
  if (startTime) queryParams.start_time = Math.floor(startTime.getTime() / 1000)
  if (endTime) queryParams.end_time = Math.floor(endTime.getTime() / 1000)
  if (sortBy) queryParams.sort_by = sortBy
  if (sortOrder) queryParams.sort_dir = sortOrder

  if (status && status.length > 0) {
    queryParams.status = status.join(',')
  }
  if (statusNot && statusNot.length > 0) {
    queryParams.status_not = statusNot.join(',')
  }

  if (modelName) queryParams.model_name = modelName
  if (providerName) queryParams.provider_name = providerName
  if (serviceName) queryParams.service_name = serviceName
  if (minCost !== undefined) queryParams.min_cost = minCost
  if (maxCost !== undefined) queryParams.max_cost = maxCost
  if (minTokens !== undefined) queryParams.min_tokens = minTokens
  if (maxTokens !== undefined) queryParams.max_tokens = maxTokens
  if (minDuration !== undefined) queryParams.min_duration = minDuration
  if (maxDuration !== undefined) queryParams.max_duration = maxDuration
  if (hasError !== undefined) queryParams.has_error = hasError

  const response = await client.getPaginated<any>('/v1/traces', queryParams)

  return {
    data: response.data.map(transformTrace),
    pagination: response.pagination,
  }
}

/**
 * Get a single trace by ID
 *
 * Backend endpoint: GET /api/v1/traces/:id
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID (32 hex characters)
 * @returns Single trace object
 */
export const getTraceById = async (
  projectId: string,
  traceId: string
): Promise<Trace> => {
  const response = await client.get(`/v1/traces/${traceId}`, {
    project_id: projectId,
  })
  return transformTraceResponse(response)
}

/**
 * Get all spans for a trace
 *
 * Backend endpoint: GET /api/v1/traces/:id/spans
 * Returns spans array directly (not wrapped in trace object)
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @returns Array of spans for the trace
 */
export const getSpansForTrace = async (
  projectId: string,
  traceId: string
): Promise<Span[]> => {
  const response = await client.get<any>(`/v1/traces/${traceId}/spans`, {
    project_id: projectId,
  })
  // Backend returns spans array directly in response (via response.Success(c, spans))
  // The BrokleAPIClient unwraps the response, so we get the data directly
  const spansData = Array.isArray(response) ? response : []
  return spansData.map(transformSpan)
}

/**
 * @deprecated Use getSpansForTrace instead - clearer naming
 */
export const getTraceWithSpans = getSpansForTrace

/**
 * Get trace with quality scores
 *
 * Backend endpoint: GET /api/v1/traces/:id/scores
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @returns Trace with quality scores
 * @deprecated Use getScoresForTrace instead - this function incorrectly transforms response
 */
export const getTraceWithScores = async (
  projectId: string,
  traceId: string
): Promise<Trace> => {
  const response = await client.get(`/v1/traces/${traceId}/scores`, {
    project_id: projectId,
  })
  return transformTraceResponse(response)
}

/**
 * Get quality scores for a trace
 *
 * Backend endpoint: GET /api/v1/traces/:id/scores
 * Returns all scores for the trace (both trace-level and span-level)
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @returns Array of scores for the trace
 */
export const getScoresForTrace = async (
  projectId: string,
  traceId: string
): Promise<Score[]> => {
  const response = await client.get<Score[]>(`/v1/traces/${traceId}/scores`, {
    project_id: projectId,
  })
  // Backend returns Score[] directly via response.Success(c, scores)
  return Array.isArray(response) ? response : []
}

/**
 * Update trace metadata
 *
 * Backend endpoint: PUT /api/v1/traces/:id
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @param data - Updated trace data
 * @returns Updated trace
 */
export const updateTrace = async (
  projectId: string,
  traceId: string,
  data: UpdateTraceData
): Promise<Trace> => {
  const response = await client.put(`/v1/traces/${traceId}`, {
    project_id: projectId,
    ...data,
  })
  return transformTraceResponse(response)
}

/**
 * Delete a trace
 *
 * Backend endpoint: DELETE /api/v1/traces/:id
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID (32 hex characters)
 */
export const deleteTrace = async (
  projectId: string,
  traceId: string
): Promise<void> => {
  await client.delete(`/v1/traces/${traceId}`, {
    params: { project_id: projectId },
  })
}

/**
 * Update trace tags
 *
 * Backend endpoint: PUT /api/v1/traces/:id/tags
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID (32 hex characters)
 * @param tags - Array of tag strings
 * @returns Updated tags array (normalized: lowercase, unique, sorted)
 */
export const updateTraceTags = async (
  projectId: string,
  traceId: string,
  tags: string[]
): Promise<{ message: string; tags: string[] }> => {
  return client.put<{ message: string; tags: string[] }>(
    `/v1/traces/${traceId}/tags`,
    { tags },
    { params: { project_id: projectId } }
  )
}

/**
 * Update trace bookmark status
 *
 * Backend endpoint: PUT /api/v1/traces/:id/bookmark
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID (32 hex characters)
 * @param bookmarked - Bookmark status
 * @returns Updated bookmark status
 */
export const updateTraceBookmark = async (
  projectId: string,
  traceId: string,
  bookmarked: boolean
): Promise<{ message: string; bookmarked: boolean }> => {
  return client.put<{ message: string; bookmarked: boolean }>(
    `/v1/traces/${traceId}/bookmark`,
    { bookmarked },
    { params: { project_id: projectId } }
  )
}

/**
 * Delete multiple traces (NOT IMPLEMENTED IN BACKEND)
 *
 * @deprecated Backend endpoint not yet implemented
 * @throws Error indicating feature is not available
 */
export const deleteMultipleTraces = async (
  projectId: string,
  traceIds: string[]
): Promise<void> => {
  throw new Error('Bulk delete functionality is not yet implemented on the backend')
  // Future implementation:
  // await client.post(`/v1/traces/bulk-delete`, {
  //   project_id: projectId,
  //   trace_ids: traceIds,
  // })
}

/**
 * Export traces to CSV (NOT IMPLEMENTED IN BACKEND)
 *
 * @deprecated Backend endpoint not yet implemented
 * @throws Error indicating feature is not available
 */
export const exportTraces = async (
  projectId: string,
  traceIds?: string[]
): Promise<Blob> => {
  throw new Error('Export functionality is not yet implemented on the backend')
  // Future implementation:
  // const response = await client.get('/v1/traces/export', {
  //   project_id: projectId,
  //   trace_ids: traceIds?.join(','),
  //   format: 'csv',
  // })
  // return response as Blob
}

/**
 * Get available filter options for traces
 *
 * Backend endpoint: GET /api/v1/traces/filter-options
 *
 * Returns distinct values for filter dropdowns and min/max ranges for sliders.
 * Used to populate the advanced filter UI dynamically based on actual data.
 *
 * @param projectId - Project UUID
 * @returns Filter options with available values and ranges
 */
export const getTraceFilterOptions = async (
  projectId: string
): Promise<TraceFilterOptions> => {
  const response = await client.get<{
    models: string[]
    providers: string[]
    services: string[]
    environments: string[]
    users: string[]
    sessions: string[]
    cost_range: { min: number; max: number } | null
    token_range: { min: number; max: number } | null
    duration_range: { min: number; max: number } | null
  }>('/v1/traces/filter-options', {
    project_id: projectId,
  })

  return {
    models: response.models || [],
    providers: response.providers || [],
    services: response.services || [],
    environments: response.environments || [],
    users: response.users || [],
    sessions: response.sessions || [],
    costRange: response.cost_range,
    tokenRange: response.token_range,
    durationRange: response.duration_range,
  }
}

// ============================================================================
// Spans API
// ============================================================================

/**
 * Get spans with filtering
 *
 * Backend endpoint: GET /api/v1/spans
 *
 * @param params - Filter and pagination parameters
 * @returns Spans array with pagination
 */
export const getSpans = async (params: GetSpansParams): Promise<PaginatedResponse<Span>> => {
  const {
    projectId,
    traceId,
    type,
    model,
    level,
    page = 1,
    pageSize = 20,
    sortBy,
    sortOrder,
  } = params

  const queryParams: Record<string, any> = {
    project_id: projectId,
    page,
    limit: pageSize,
  }

  if (traceId) queryParams.trace_id = traceId
  if (type) queryParams.type = type
  if (model) queryParams.model = model
  if (level) queryParams.level = level
  if (sortBy) queryParams.sort_by = sortBy
  if (sortOrder) queryParams.sort_dir = sortOrder

  const response = await client.getPaginated<any>('/v1/spans', queryParams)

  return {
    data: response.data.map(transformSpan),
    pagination: response.pagination,
  }
}

/**
 * Get a single span by ID
 *
 * Backend endpoint: GET /api/v1/spans/:id
 */
export const getSpanById = async (
  projectId: string,
  spanId: string
): Promise<Span> => {
  const response = await client.get<any>(`/v1/spans/${spanId}`, {
    project_id: projectId,
  })
  return transformSpan(response)
}

/**
 * Update span metadata
 *
 * Backend endpoint: PUT /api/v1/spans/:id
 */
export const updateSpan = async (
  projectId: string,
  spanId: string,
  data: UpdateSpanData
): Promise<Span> => {
  const response = await client.put<any>(`/v1/spans/${spanId}`, {
    project_id: projectId,
    ...data,
  })
  return transformSpan(response)
}

// ============================================================================
// Quality Scores API
// ============================================================================

/**
 * Get quality scores with filtering
 *
 * Backend endpoint: GET /api/v1/scores
 */
export const getScores = async (params: GetScoresParams): Promise<PaginatedResponse<Score>> => {
  const {
    projectId,
    traceId,
    spanId,
    sessionId,
    name,
    source,
    dataType,
    page = 1,
    pageSize = 20,
  } = params

  const queryParams: Record<string, any> = {
    project_id: projectId,
    page,
    limit: pageSize,
  }

  if (traceId) queryParams.trace_id = traceId
  if (spanId) queryParams.span_id = spanId
  if (sessionId) queryParams.session_id = sessionId
  if (name) queryParams.name = name
  if (source) queryParams.source = source
  if (dataType) queryParams.type = dataType

  const response = await client.getPaginated<Score>('/v1/scores', queryParams)

  return {
    data: response.data.map(transformScore),
    pagination: response.pagination,
  }
}

/**
 * Get a single score by ID
 *
 * Backend endpoint: GET /api/v1/scores/:id
 */
export const getScoreById = async (
  projectId: string,
  scoreId: string
): Promise<Score> => {
  const response = await client.get<any>(`/v1/scores/${scoreId}`, {
    project_id: projectId,
  })
  return transformScore(response)
}

/**
 * Update a quality score
 *
 * Backend endpoint: PUT /api/v1/scores/:id
 */
export const updateScore = async (
  projectId: string,
  scoreId: string,
  data: UpdateScoreData
): Promise<Score> => {
  const response = await client.put<any>(`/v1/scores/${scoreId}`, {
    project_id: projectId,
    ...data,
  })
  return transformScore(response)
}

// ============================================================================
// Filter Presets API
// ============================================================================

export interface FilterCondition {
  id: string
  column: string
  operator: FilterOperator
  value: string | number | string[] | null
}

export type FilterOperator =
  | '='
  | '!='
  | '>'
  | '<'
  | '>='
  | '<='
  | 'CONTAINS'
  | 'NOT CONTAINS'
  | 'IN'
  | 'NOT IN'
  | 'EXISTS'
  | 'NOT EXISTS'
  | 'STARTS WITH'
  | 'ENDS WITH'
  | 'REGEX'
  | 'IS EMPTY'
  | 'IS NOT EMPTY'
  | '~' // full-text search

export interface FilterPreset {
  id: string
  project_id: string
  name: string
  description?: string
  table_name: 'traces' | 'spans'
  filters: FilterCondition[]
  column_order?: string[]
  column_visibility?: Record<string, boolean>
  search_query?: string
  search_types?: ('id' | 'content' | 'all')[]
  is_public: boolean
  created_by?: string
  created_at: string
  updated_at: string
}

export interface CreateFilterPresetRequest {
  name: string
  description?: string
  table_name: 'traces' | 'spans'
  filters: FilterCondition[]
  column_order?: string[]
  column_visibility?: Record<string, boolean>
  search_query?: string
  search_types?: string[]
  is_public?: boolean
}

export interface UpdateFilterPresetRequest {
  name?: string
  description?: string
  filters?: FilterCondition[]
  column_order?: string[]
  column_visibility?: Record<string, boolean>
  search_query?: string
  search_types?: string[]
  is_public?: boolean
}

/**
 * Get all filter presets for a project
 *
 * Backend endpoint: GET /api/v1/projects/{projectId}/filter-presets
 *
 * @param projectId - Project UUID
 * @param tableName - Filter by table name ('traces' or 'spans')
 * @param includePublic - Include public presets (default: true)
 * @returns Array of filter presets
 */
export const getFilterPresets = async (
  projectId: string,
  tableName?: 'traces' | 'spans',
  includePublic: boolean = true
): Promise<FilterPreset[]> => {
  const queryParams: Record<string, any> = {
    project_id: projectId,
    include_public: includePublic,
  }
  if (tableName) {
    queryParams.table_name = tableName
  }

  return client.get<FilterPreset[]>(
    `/v1/projects/${projectId}/filter-presets`,
    queryParams
  )
}

/**
 * Get a single filter preset by ID
 *
 * Backend endpoint: GET /api/v1/projects/{projectId}/filter-presets/{id}
 *
 * @param projectId - Project UUID
 * @param presetId - Filter preset ID
 * @returns Filter preset object
 */
export const getFilterPresetById = async (
  projectId: string,
  presetId: string
): Promise<FilterPreset> => {
  return client.get<FilterPreset>(
    `/v1/projects/${projectId}/filter-presets/${presetId}`,
    { project_id: projectId }
  )
}

/**
 * Create a new filter preset
 *
 * Backend endpoint: POST /api/v1/projects/{projectId}/filter-presets
 *
 * @param projectId - Project UUID
 * @param data - Filter preset data
 * @returns Created filter preset
 */
export const createFilterPreset = async (
  projectId: string,
  data: CreateFilterPresetRequest
): Promise<FilterPreset> => {
  return client.post<FilterPreset>(
    `/v1/projects/${projectId}/filter-presets`,
    data
  )
}

/**
 * Update an existing filter preset
 *
 * Backend endpoint: PATCH /api/v1/projects/{projectId}/filter-presets/{id}
 *
 * @param projectId - Project UUID
 * @param presetId - Filter preset ID
 * @param data - Updated filter preset data
 * @returns Updated filter preset
 */
export const updateFilterPreset = async (
  projectId: string,
  presetId: string,
  data: UpdateFilterPresetRequest
): Promise<FilterPreset> => {
  return client.patch<FilterPreset>(
    `/v1/projects/${projectId}/filter-presets/${presetId}`,
    data
  )
}

/**
 * Delete a filter preset
 *
 * Backend endpoint: DELETE /api/v1/projects/{projectId}/filter-presets/{id}
 *
 * @param projectId - Project UUID
 * @param presetId - Filter preset ID
 */
export const deleteFilterPreset = async (
  projectId: string,
  presetId: string
): Promise<void> => {
  await client.delete(`/v1/projects/${projectId}/filter-presets/${presetId}`)
}

/**
 * Discover available attribute keys from spans
 *
 * Backend endpoint: GET /api/v1/traces/attributes
 *
 * @param projectId - Project UUID
 * @returns Array of attribute keys with metadata
 */
export interface AttributeKey {
  key: string
  source: 'span_attributes' | 'resource_attributes'
  value_type: 'string' | 'number' | 'boolean' | 'array'
  count: number
}

export interface AttributeDiscoveryResponse {
  attributes: AttributeKey[]
  total_count: number
}

export const discoverAttributes = async (
  projectId: string
): Promise<AttributeDiscoveryResponse> => {
  return client.get<AttributeDiscoveryResponse>('/v1/traces/attributes', {
    project_id: projectId,
  })
}
