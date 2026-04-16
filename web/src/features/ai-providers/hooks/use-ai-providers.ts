'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useMemo } from 'react'
import {
  listProviderCredentials,
  createProviderCredential,
  updateProviderCredential,
  deleteProviderCredential,
  testProviderConnection,
  getAvailableModels,
} from '../api/ai-providers-api'
import type {
  AIProviderCredential,
  AvailableModel,
  CreateProviderRequest,
  UpdateProviderRequest,
  ModelsByProvider,
  TestConnectionRequest,
  AIProvider,
} from '../types'

/**
 * Query keys for AI provider credentials
 * Structured factory pattern for cache management
 *
 * Note: AI provider credentials are organization-scoped
 */
export const aiProviderQueryKeys = {
  all: ['ai-providers'] as const,
  lists: () => [...aiProviderQueryKeys.all, 'list'] as const,
  list: (orgId: string) => [...aiProviderQueryKeys.lists(), orgId] as const,
  detail: (orgId: string, credentialId: string) =>
    [...aiProviderQueryKeys.all, 'detail', orgId, credentialId] as const,
  models: () => [...aiProviderQueryKeys.all, 'models'] as const,
  modelsByOrg: (orgId: string) => [...aiProviderQueryKeys.models(), orgId] as const,
}

/**
 * Query hook to list all AI provider credentials for an organization
 *
 * @param orgId - Organization UUID
 * @param options - React Query options
 *
 * @example
 * ```tsx
 * const { data, isLoading, error } = useAIProvidersQuery('org_123')
 * ```
 */
export function useAIProvidersQuery(
  orgId: string | undefined,
  options: {
    enabled?: boolean
  } = {}
) {
  return useQuery({
    queryKey: aiProviderQueryKeys.list(orgId || ''),
    queryFn: () => {
      if (!orgId) {
        throw new Error('Organization ID is required')
      }
      return listProviderCredentials(orgId)
    },
    enabled: !!orgId && (options.enabled ?? true),
    staleTime: 30000, // 30 seconds
    gcTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Mutation hook to create a new AI provider credential
 *
 * Automatically invalidates the credentials list cache on success.
 *
 * @param orgId - Organization UUID
 *
 * @example
 * ```tsx
 * const mutation = useCreateProviderMutation('org_123')
 *
 * const handleSave = async () => {
 *   await mutation.mutateAsync({
 *     name: 'OpenAI Production',
 *     adapter: 'openai',
 *     api_key: 'sk-...',
 *   })
 * }
 * ```
 */
export function useCreateProviderMutation(orgId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (data: CreateProviderRequest) => {
      return createProviderCredential(orgId, data)
    },
    onSuccess: (credential: AIProviderCredential) => {
      // Invalidate credentials list
      queryClient.invalidateQueries({
        queryKey: aiProviderQueryKeys.lists(),
      })

      // Invalidate models cache (models depend on configured providers)
      queryClient.invalidateQueries({
        queryKey: aiProviderQueryKeys.models(),
      })

      toast.success('Provider Created', {
        description: `${credential.name} has been created successfully.`,
      })
    },
    onError: (error: unknown) => {
      const apiError = error as { message?: string }
      toast.error('Failed to Create Provider', {
        description: apiError?.message || 'Could not create provider credentials. Please try again.',
      })
    },
  })
}

/**
 * Mutation hook to update an existing AI provider credential
 *
 * Automatically invalidates the credentials list cache on success.
 *
 * @param orgId - Organization UUID
 *
 * @example
 * ```tsx
 * const mutation = useUpdateProviderMutation('org_123')
 *
 * const handleSave = async () => {
 *   await mutation.mutateAsync({
 *     credentialId: 'cred_456',
 *     data: {
 *       name: 'OpenAI Development',
 *       api_key: 'sk-new-key...',
 *     }
 *   })
 * }
 * ```
 */
export function useUpdateProviderMutation(orgId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ credentialId, data }: { credentialId: string; data: UpdateProviderRequest }) => {
      return updateProviderCredential(orgId, credentialId, data)
    },
    onSuccess: (credential: AIProviderCredential) => {
      // Invalidate credentials list
      queryClient.invalidateQueries({
        queryKey: aiProviderQueryKeys.lists(),
      })

      // Invalidate models cache (models depend on configured providers)
      queryClient.invalidateQueries({
        queryKey: aiProviderQueryKeys.models(),
      })

      toast.success('Provider Updated', {
        description: `${credential.name} has been updated successfully.`,
      })
    },
    onError: (error: unknown) => {
      const apiError = error as { message?: string }
      toast.error('Failed to Update Provider', {
        description: apiError?.message || 'Could not update provider credentials. Please try again.',
      })
    },
  })
}

/**
 * Mutation hook to delete an AI provider credential
 *
 * Supports optimistic updates. Deletes by credential ID.
 *
 * @param orgId - Organization UUID
 *
 * @example
 * ```tsx
 * const mutation = useDeleteProviderMutation('org_123')
 *
 * await mutation.mutateAsync({
 *   credentialId: 'cred_456',
 *   displayName: 'OpenAI Production'
 * })
 * ```
 */
export function useDeleteProviderMutation(orgId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      credentialId,
    }: {
      credentialId: string
      displayName: string // For display in toast
    }) => {
      return deleteProviderCredential(orgId, credentialId)
    },
    onMutate: async ({ credentialId }) => {
      // Cancel outgoing refetches
      await queryClient.cancelQueries({
        queryKey: aiProviderQueryKeys.lists(),
      })

      // Snapshot previous value
      const previousCredentials = queryClient.getQueriesData({
        queryKey: aiProviderQueryKeys.lists(),
      })

      // Optimistically remove from cache by credential ID
      queryClient.setQueriesData<AIProviderCredential[]>(
        { queryKey: aiProviderQueryKeys.lists() },
        (old) => {
          if (!old) return old
          return old.filter((cred) => cred.id !== credentialId)
        }
      )

      return { previousCredentials }
    },
    onSuccess: (_data, variables) => {
      // Invalidate to get fresh data
      queryClient.invalidateQueries({
        queryKey: aiProviderQueryKeys.lists(),
      })

      // Invalidate models cache (deleted provider's models should disappear)
      queryClient.invalidateQueries({
        queryKey: aiProviderQueryKeys.models(),
      })

      toast.success('Provider Deleted', {
        description: `${variables.displayName} has been removed.`,
      })
    },
    onError: (error: unknown, _variables, context) => {
      // Rollback optimistic update
      if (context?.previousCredentials) {
        context.previousCredentials.forEach(([queryKey, data]) => {
          queryClient.setQueryData(queryKey, data)
        })
      }

      const apiError = error as { message?: string }
      toast.error('Failed to Delete Provider', {
        description: apiError?.message || 'Could not delete provider. Please try again.',
      })
    },
  })
}

/**
 * Mutation hook to test a provider connection
 *
 * Use this before saving credentials to validate the API key.
 *
 * @param orgId - Organization UUID
 *
 * @example
 * ```tsx
 * const mutation = useTestConnectionMutation('org_123')
 *
 * const handleTest = async () => {
 *   const result = await mutation.mutateAsync({
 *     provider: 'openai',
 *     api_key: 'sk-...',
 *   })
 *   if (result.success) {
 *     console.log('Connection successful!')
 *   }
 * }
 * ```
 */
export function useTestConnectionMutation(orgId: string) {
  return useMutation({
    mutationFn: async (data: TestConnectionRequest) => {
      return testProviderConnection(orgId, data)
    },
    onSuccess: (result) => {
      if (result.success) {
        toast.success('Connection Successful', {
          description: 'The API key is valid and can connect to the provider.',
        })
      } else {
        toast.error('Connection Failed', {
          description: result.error || 'Could not connect to the provider.',
        })
      }
    },
    onError: (error: unknown) => {
      const apiError = error as { message?: string }
      toast.error('Connection Test Failed', {
        description: apiError?.message || 'Could not test connection. Please try again.',
      })
    },
  })
}

/**
 * Query hook to get available models for an organization based on configured providers
 *
 * Returns models from:
 * - Standard providers (openai, anthropic, etc.): default models + custom_models
 * - Custom provider: only custom_models
 *
 * @param orgId - Organization UUID
 * @param options - React Query options
 *
 * @example
 * ```tsx
 * const { data: models, isLoading } = useAvailableModelsQuery('org_123')
 * ```
 */
export function useAvailableModelsQuery(
  orgId: string | undefined,
  options: {
    enabled?: boolean
  } = {}
) {
  return useQuery({
    queryKey: aiProviderQueryKeys.modelsByOrg(orgId || ''),
    queryFn: () => {
      if (!orgId) {
        throw new Error('Organization ID is required')
      }
      return getAvailableModels(orgId)
    },
    enabled: !!orgId && (options.enabled ?? true),
    staleTime: 5 * 60 * 1000, // 5 minutes - models don't change often
    gcTime: 10 * 60 * 1000, // 10 minutes
  })
}

/**
 * Hook to get available models grouped by provider
 *
 * @param orgId - Organization UUID
 *
 * @example
 * ```tsx
 * const { modelsByProvider, configuredProviders, isLoading } = useModelsByProvider('org_123')
 * // modelsByProvider: { openai: [...], anthropic: [...] }
 * // configuredProviders: ['openai', 'anthropic']
 * ```
 */
export function useModelsByProvider(orgId: string | undefined) {
  const query = useAvailableModelsQuery(orgId)

  const modelsByProvider = useMemo((): ModelsByProvider => {
    if (!query.data) return {}

    return query.data.reduce((acc, model) => {
      const provider = model.provider as AIProvider
      if (!acc[provider]) {
        acc[provider] = []
      }
      acc[provider]!.push(model)
      return acc
    }, {} as ModelsByProvider)
  }, [query.data])

  // Get list of providers that have models
  const configuredProviders = useMemo(() => {
    return Object.keys(modelsByProvider) as AIProvider[]
  }, [modelsByProvider])

  return {
    ...query,
    modelsByProvider,
    configuredProviders,
  }
}
