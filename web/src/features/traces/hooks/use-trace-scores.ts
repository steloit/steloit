'use client'

import { useQuery } from '@tanstack/react-query'
import { getScoresForTrace } from '../api/traces-api'
import { traceQueryKeys } from './trace-query-keys'

/**
 * Hook to fetch scores for a trace
 *
 * Uses React Query for:
 * - Automatic caching
 * - Loading state management
 * - Error handling
 * - Background refetching
 *
 * @param projectId - Project UUID (optional, query disabled if not provided)
 * @param traceId - Trace ID (optional, query disabled if not provided)
 * @returns Query result with scores array
 */
export function useTraceScoresQuery(
  projectId: string | undefined,
  traceId: string | undefined
) {
  return useQuery({
    queryKey: traceQueryKeys.scores(projectId ?? '', traceId ?? ''),
    queryFn: () => getScoresForTrace(projectId!, traceId!),
    enabled: !!projectId && !!traceId,
    staleTime: 30_000, // 30 seconds - scores don't change frequently
    gcTime: 5 * 60 * 1000, // 5 minutes garbage collection
  })
}
