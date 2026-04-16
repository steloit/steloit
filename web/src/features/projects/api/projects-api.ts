// Dashboard API - Direct functions for dashboard data

import { BrokleAPIClient } from '@/lib/api/core/client'
import type { QueryParams } from '@/lib/api/core/types'

// Project types
export interface UpdateProjectRequest {
  name?: string
  description?: string
  // Status removed - use archive/unarchive functions instead
}

export interface Project {
  id: string
  name: string
  slug: string
  description: string
  status: 'active' | 'archived'
  organization_id: string
  created_at: string
  updated_at: string
}

// Dashboard-specific types
export interface QuickStat {
  label: string
  value: string | number
  change?: number
  changeType?: 'increase' | 'decrease' | 'neutral'
  icon?: string
}

export interface ChartData {
  labels: string[]
  datasets: Array<{
    label: string
    data: number[]
    backgroundColor?: string
    borderColor?: string
  }>
}

export interface DashboardOverview {
  quickStats: QuickStat[]
  requestTrend: ChartData
  costTrend: ChartData
  recentActivity: Array<{
    id: string
    type: 'request' | 'error' | 'cost_alert' | 'model_change'
    message: string
    timestamp: string
    severity?: 'info' | 'warning' | 'error'
  }>
  alerts: Array<{
    id: string
    type: 'budget' | 'error_rate' | 'latency' | 'quota'
    message: string
    severity: 'info' | 'warning' | 'error'
    timestamp: string
    acknowledged: boolean
  }>
}

export interface DashboardConfig {
  widgets: Array<{
    id: string
    type: string
    position: { x: number; y: number }
    size: { width: number; height: number }
    config: Record<string, any>
  }>
  layout: string
}

// Flexible base client - versions specified per endpoint
const client = new BrokleAPIClient('/api')

// Direct dashboard functions
export const getOverview = async (timeRange: string = '24h'): Promise<DashboardOverview> => {
    return client.get<DashboardOverview>('/v2/analytics/overview', { timeRange })
  }

export const getQuickStats = async (timeRange: string = '24h'): Promise<QuickStat[]> => {
    return client.get<QuickStat[]>('/v2/analytics/overview', { timeRange })
  }

export const getRecentActivity = async (limit: number = 10): Promise<DashboardOverview['recentActivity']> => {
    return client.get('/logs/requests', { limit })
  }

export const getAlerts = async (acknowledged?: boolean): Promise<DashboardOverview['alerts']> => {
    const params = acknowledged !== undefined ? { acknowledged } : {}
    return client.get('/alerts', params)
  }

export const acknowledgeAlert = async (alertId: string): Promise<void> => {
    return client.patch<void>(`/alerts/${alertId}/acknowledge`, {})
  }

export const dismissAlert = async (alertId: string): Promise<void> => {
    return client.delete<void>(`/alerts/${alertId}`)
  }

export const getWidgetData = async (widgetType: string, config?: QueryParams): Promise<any> => {
    return client.get(`/dashboard/widgets/${widgetType}`, config)
  }

export const saveDashboardConfig = async (config: DashboardConfig): Promise<void> => {
    return client.post<void>('/dashboard/config', config)
  }

export const getDashboardConfig = async (): Promise<{
    widgets: any[]
    layout: string
    lastUpdated: string
  }> => {
    return client.get('/dashboard/config')
  }

/**
 * Update project settings
 *
 * @param projectId - Project ID (UUID)
 * @param data - Fields to update (name, description only - use archive/unarchive for status)
 * @returns Updated project
 *
 * @example
 * ```ts
 * await updateProject('proj_123', {
 *   name: 'Updated Name',
 *   description: 'New description'
 * })
 * ```
 */
export async function updateProject(
  projectId: string,
  data: UpdateProjectRequest
): Promise<Project> {
  const endpoint = `/v1/projects/${projectId}`
  return client.put<Project, UpdateProjectRequest>(endpoint, data)
}

/**
 * Archive a project (sets status to archived, read-only, reversible)
 *
 * @param projectId - Project ID (UUID)
 *
 * @example
 * ```ts
 * await archiveProject('proj_123')
 * ```
 */
export async function archiveProject(projectId: string): Promise<void> {
  const endpoint = `/v1/projects/${projectId}/archive`
  await client.post<void>(endpoint)
}

/**
 * Unarchive a project (sets status back to active)
 *
 * @param projectId - Project ID (UUID)
 *
 * @example
 * ```ts
 * await unarchiveProject('proj_123')
 * ```
 */
export async function unarchiveProject(projectId: string): Promise<void> {
  const endpoint = `/v1/projects/${projectId}/unarchive`
  await client.post<void>(endpoint)
}

