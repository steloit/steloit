import { BrokleAPIClient } from '@/lib/api/core/client'

const client = new BrokleAPIClient('/api')

// ============================================================================
// Types
// ============================================================================

export interface CommentUser {
  id: string
  name: string
  email: string
  avatar_url?: string | null
}

export interface ReactionSummary {
  emoji: string
  count: number
  users: string[]
  has_user: boolean
}

export interface Comment {
  id: string
  entity_type: 'trace' | 'span'
  entity_id: string
  project_id: string
  content: string
  created_by: string
  updated_by?: string | null
  created_at: string
  updated_at: string
  author?: CommentUser | null
  editor?: CommentUser | null
  is_edited: boolean
  parent_id?: string | null
  reactions: ReactionSummary[]
  replies?: Comment[]
  reply_count: number
}

export interface ListCommentsResponse {
  comments: Comment[]
  total: number
}

export interface CommentCountResponse {
  count: number
}

export interface CreateCommentRequest {
  content: string
}

export interface UpdateCommentRequest {
  content: string
}

export interface ToggleReactionRequest {
  emoji: string
}

// ============================================================================
// Comments API
// ============================================================================

/**
 * List all comments for a trace
 *
 * Backend endpoint: GET /api/v1/traces/:trace_id/comments
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @returns List of comments with user information
 */
export const listComments = async (
  projectId: string,
  traceId: string
): Promise<ListCommentsResponse> => {
  return client.get<ListCommentsResponse>(
    `/v1/traces/${traceId}/comments`,
    { project_id: projectId }
  )
}

/**
 * Get comment count for a trace
 *
 * Backend endpoint: GET /api/v1/traces/:trace_id/comments/count
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @returns Comment count
 */
export const getCommentCount = async (
  projectId: string,
  traceId: string
): Promise<CommentCountResponse> => {
  return client.get<CommentCountResponse>(
    `/v1/traces/${traceId}/comments/count`,
    { project_id: projectId }
  )
}

/**
 * Create a new comment on a trace
 *
 * Backend endpoint: POST /api/v1/traces/:trace_id/comments
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @param data - Comment content
 * @returns Created comment
 */
export const createComment = async (
  projectId: string,
  traceId: string,
  data: CreateCommentRequest
): Promise<Comment> => {
  return client.post<Comment>(
    `/v1/traces/${traceId}/comments`,
    data,
    { params: { project_id: projectId } }
  )
}

/**
 * Update an existing comment
 *
 * Backend endpoint: PUT /api/v1/traces/:trace_id/comments/:comment_id
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @param commentId - Comment ID
 * @param data - Updated content
 * @returns Updated comment
 */
export const updateComment = async (
  projectId: string,
  traceId: string,
  commentId: string,
  data: UpdateCommentRequest
): Promise<Comment> => {
  return client.put<Comment>(
    `/v1/traces/${traceId}/comments/${commentId}`,
    data,
    { params: { project_id: projectId } }
  )
}

/**
 * Delete a comment
 *
 * Backend endpoint: DELETE /api/v1/traces/:trace_id/comments/:comment_id
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @param commentId - Comment ID
 */
export const deleteComment = async (
  projectId: string,
  traceId: string,
  commentId: string
): Promise<void> => {
  await client.delete(
    `/v1/traces/${traceId}/comments/${commentId}`,
    { params: { project_id: projectId } }
  )
}

/**
 * Toggle a reaction on a comment
 *
 * Backend endpoint: POST /api/v1/traces/:trace_id/comments/:comment_id/reactions
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @param commentId - Comment ID
 * @param data - Reaction emoji
 * @returns Updated reaction summaries for the comment
 */
export const toggleReaction = async (
  projectId: string,
  traceId: string,
  commentId: string,
  data: ToggleReactionRequest
): Promise<ReactionSummary[]> => {
  return client.post<ReactionSummary[]>(
    `/v1/traces/${traceId}/comments/${commentId}/reactions`,
    data,
    { params: { project_id: projectId } }
  )
}

/**
 * Create a reply to an existing comment
 *
 * Backend endpoint: POST /api/v1/traces/:trace_id/comments/:comment_id/replies
 *
 * @param projectId - Project UUID
 * @param traceId - Trace ID
 * @param parentId - Parent comment ID
 * @param data - Reply content
 * @returns Created reply comment
 */
export const createReply = async (
  projectId: string,
  traceId: string,
  parentId: string,
  data: CreateCommentRequest
): Promise<Comment> => {
  return client.post<Comment>(
    `/v1/traces/${traceId}/comments/${parentId}/replies`,
    data,
    { params: { project_id: projectId } }
  )
}
