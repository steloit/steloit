import { BrokleAPIClient } from '@/lib/api/core/client'

const client = new BrokleAPIClient('/api')

// ============================================================================
// Types
// ============================================================================

export type ScoreDataType = 'NUMERIC' | 'CATEGORICAL' | 'BOOLEAN'
export type ScoreSource = 'api' | 'eval' | 'annotation'

export interface Annotation {
  id: string
  project_id: string
  trace_id?: string | null
  span_id?: string | null
  name: string
  value?: number | null
  string_value?: string | null
  type: ScoreDataType
  source: ScoreSource
  reason?: string | null
  created_by?: string | null
  timestamp: string
}

export interface CreateAnnotationRequest {
  name: string
  value?: number | null
  string_value?: string | null
  type: ScoreDataType
  reason?: string | null
}

export interface UpdateAnnotationRequest {
  value?: number | null
  string_value?: string | null
  reason?: string | null
}

// ============================================================================
// Annotations API
// ============================================================================

/**
 * Get all scores for a trace (annotations + automated)
 *
 * Backend endpoint: GET /api/v1/traces/:trace_id/scores
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @returns List of scores/annotations
 */
export const getTraceScores = async (
  projectId: string,
  traceId: string
): Promise<Annotation[]> => {
  const response = await client.get<Annotation[]>(
    `/v1/traces/${traceId}/scores`,
    { project_id: projectId }
  )
  return Array.isArray(response) ? response : []
}

/**
 * Create a human annotation score for a trace
 *
 * Backend endpoint: POST /api/v1/traces/:trace_id/scores
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @param data - Annotation data
 * @returns Created annotation
 */
export const createAnnotation = async (
  projectId: string,
  traceId: string,
  data: CreateAnnotationRequest
): Promise<Annotation> => {
  return client.post<Annotation>(
    `/v1/traces/${traceId}/scores`,
    data,
    { params: { project_id: projectId } }
  )
}

/**
 * Update an annotation score
 *
 * Backend endpoint: PUT /api/v1/traces/:trace_id/scores/:score_id
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @param scoreId - Score ID
 * @param data - Update data
 * @returns Updated annotation
 */
export const updateAnnotation = async (
  projectId: string,
  traceId: string,
  scoreId: string,
  data: UpdateAnnotationRequest
): Promise<Annotation> => {
  return client.put<Annotation>(
    `/v1/traces/${traceId}/scores/${scoreId}`,
    data,
    { params: { project_id: projectId } }
  )
}

/**
 * Delete an annotation score
 *
 * Backend endpoint: DELETE /api/v1/traces/:trace_id/scores/:score_id
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @param scoreId - Score ID
 */
export const deleteAnnotation = async (
  projectId: string,
  traceId: string,
  scoreId: string
): Promise<void> => {
  await client.delete(
    `/v1/traces/${traceId}/scores/${scoreId}`,
    { params: { project_id: projectId } }
  )
}
