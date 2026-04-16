import axios, { AxiosInstance, AxiosResponse } from 'axios'
import { urlContextManager } from '@/lib/context/url-context-manager'
import type {
  BrokleClientConfig,
  RequestOptions,
  APIResponse,
  QueryParams,
  PaginatedResponse,
  BackendPagination,
  Pagination,
  ExtendedAxiosRequestConfig,
} from './types'
import { BrokleAPIError as APIError } from './types'

export class BrokleAPIClient {
  protected axiosInstance: AxiosInstance
  // Note: Refresh logic removed - auth store owns token refresh (single source of truth)

  constructor(
    basePath: string = '',
    protected config: Partial<BrokleClientConfig> = {}
  ) {
    const defaultConfig: BrokleClientConfig = {
      baseURL: process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8000',
      timeout: 30000,
      retries: 3,
      retryDelay: 1000,
      enableRequestId: true,
      enableLogging: process.env.NODE_ENV === 'development',
      logLevel: 'info',
      enablePerformanceLogging: process.env.NODE_ENV === 'development',
      maxConcurrentRequests: 10,
      headers: {
        'Content-Type': 'application/json',
      },
    }

    const finalConfig = { ...defaultConfig, ...config }

    this.axiosInstance = axios.create({
      baseURL: finalConfig.baseURL,
      timeout: finalConfig.timeout,
      headers: finalConfig.headers,
      withCredentials: true,  // CRITICAL: Required for httpOnly cookies
    })

    // Set base path for service-specific clients
    if (basePath) {
      this.axiosInstance.defaults.baseURL += basePath
    }

    this.setupInterceptors()
  }

  // Public HTTP methods with retry support
  async get<T>(
    endpoint: string, 
    params?: QueryParams, 
    options: RequestOptions = {}
  ): Promise<T> {
    return this.executeWithRetry(async () => {
      const response = await this.axiosInstance.get<APIResponse<T>>(endpoint, {
        params,
        ...options,
      })
      return this.extractData(response)
    }, options.retries)
  }

  async post<T, D = unknown>(
    endpoint: string,
    data?: D,
    options: RequestOptions = {}
  ): Promise<T> {
    return this.executeWithRetry(async () => {
      const response = await this.axiosInstance.post<APIResponse<T>>(endpoint, data, options)
      return this.extractData(response)
    }, options.retries)
  }

  async put<T, D = unknown>(
    endpoint: string,
    data?: D,
    options: RequestOptions = {}
  ): Promise<T> {
    return this.executeWithRetry(async () => {
      const response = await this.axiosInstance.put<APIResponse<T>>(endpoint, data, options)
      return this.extractData(response)
    }, options.retries)
  }

  async patch<T, D = unknown>(
    endpoint: string,
    data?: D,
    options: RequestOptions = {}
  ): Promise<T> {
    return this.executeWithRetry(async () => {
      const response = await this.axiosInstance.patch<APIResponse<T>>(endpoint, data, options)
      return this.extractData(response)
    }, options.retries)
  }

  async delete<T>(
    endpoint: string, 
    options: RequestOptions = {}
  ): Promise<T> {
    return this.executeWithRetry(async () => {
      const response = await this.axiosInstance.delete<APIResponse<T>>(endpoint, options)
      return this.extractData(response)
    }, options.retries)
  }

  // Paginated HTTP methods (preserve pagination metadata from meta.pagination)

  async getPaginated<T>(
    endpoint: string, 
    params?: QueryParams, 
    options: RequestOptions = {}
  ): Promise<PaginatedResponse<T>> {
    return this.executeWithRetry(async () => {
      const response = await this.axiosInstance.get<APIResponse<T[]>>(endpoint, {
        params,
        ...options,
      })
      return this.extractPaginatedData(response)
    }, options.retries)
  }

  async postPaginated<T, D = unknown>(
    endpoint: string,
    data?: D,
    options: RequestOptions = {}
  ): Promise<PaginatedResponse<T>> {
    return this.executeWithRetry(async () => {
      const response = await this.axiosInstance.post<APIResponse<T[]>>(endpoint, data, options)
      return this.extractPaginatedData(response)
    }, options.retries)
  }

  // Setup axios interceptors
  private setupInterceptors(): void {
    // Request interceptor - Add CSRF tokens and context headers
    this.axiosInstance.interceptors.request.use(
      async (config) => {
        // Initialize headers
        config.headers = config.headers || {}

        // Add CSRF token for mutations (NOT for GET requests)
        // httpOnly cookies are sent automatically by the browser
        if (['post', 'put', 'patch', 'delete'].includes(config.method?.toLowerCase() || '')) {
          const csrfToken = this.getCookie('csrf_token')
          if (csrfToken) {
            config.headers['X-CSRF-Token'] = csrfToken
          }
        }

        // Add context headers based on explicit request options (URL-based, opt-in only)
        const requestOptions = config as RequestOptions
        try {
          // Only generate headers if explicitly requested and we have a pathname
          if (typeof window !== 'undefined' && 
              (requestOptions.includeOrgContext || requestOptions.includeProjectContext || requestOptions.includeEnvironmentContext)) {
            
            const contextHeaders = await urlContextManager.getHeadersFromURL(window.location.pathname, {
              includeOrgContext: requestOptions.includeOrgContext,
              includeProjectContext: requestOptions.includeProjectContext,
              includeEnvironmentContext: requestOptions.includeEnvironmentContext,
              customOrgId: requestOptions.customOrgId,
              customProjectId: requestOptions.customProjectId,
              customEnvironmentId: requestOptions.customEnvironmentId,
            })

            // Add context headers to request (will be empty if none requested)
            Object.assign(config.headers, contextHeaders)

            // Log context headers in development (only if headers were added)
            if (this.config.enableLogging && Object.keys(contextHeaders).length > 0) {
              console.debug('[API Context]', {
                url: config.url,
                method: config.method?.toUpperCase(),
                pathname: window.location.pathname,
                contextHeaders,
              })
            }
          }
        } catch (error) {
          // Don't fail the request if context headers fail
          console.warn('[API] Failed to add context headers:', error)
        }

        // Add request ID for tracing if enabled
        if (this.config.enableRequestId && !config.headers['X-Request-Id']) {
          config.headers['X-Request-Id'] = crypto.randomUUID()
        }

        // Add custom headers from config
        if (this.config.customHeaders) {
          Object.assign(config.headers, this.config.customHeaders)
        }

        // Add performance timing start
        if (this.config.enablePerformanceLogging) {
          (config as ExtendedAxiosRequestConfig)._requestStartTime = Date.now()
        }

        return config
      },
      (error) => {
        return Promise.reject(new APIError(error))
      }
    )

    // Response interceptor - Handle data extraction and token refresh
    this.axiosInstance.interceptors.response.use(
      (response: AxiosResponse) => {
        // Calculate performance timing if enabled
        const startTime = (response.config as ExtendedAxiosRequestConfig)._requestStartTime
        const duration = startTime ? Date.now() - startTime : undefined

        // Enhanced logging based on configuration
        if (this.config.enableLogging) {
          const logData = {
            method: response.config.method?.toUpperCase(),
            url: response.config.url,
            status: response.status,
            requestId: response.headers['x-request-id'] || response.config.headers?.['X-Request-Id'],
            ...(duration && { duration: `${duration}ms` }),
            ...(this.config.logLevel === 'debug' && { 
              responseData: response.data,
              responseHeaders: response.headers 
            })
          }

          const logLevel = this.config.logLevel || 'info'
          const message = `[API] ${logData.method} ${logData.url}`

          switch (logLevel) {
            case 'debug':
              console.debug(message, logData)
              break
            case 'info':
              console.info(message, { 
                status: logData.status, 
                requestId: logData.requestId,
                ...(duration && { duration: logData.duration })
              })
              break
            case 'warn':
              if (response.status >= 400) console.warn(message, logData)
              break
            case 'error':
              if (response.status >= 500) console.error(message, logData)
              break
          }

          // Performance logging if enabled
          if (this.config.enablePerformanceLogging && duration) {
            console.debug(`[PERF] ${logData.method} ${logData.url}: ${duration}ms`)
          }
        }

        return response
      },
      async (error) => {
        const originalRequest = error.config as RequestOptions

        // Handle 401 errors with token refresh
        // Skip if: already retried, explicitly bypassed, or is the refresh endpoint itself
        if (error.response?.status === 401 &&
            !originalRequest._retry &&
            !originalRequest.skipRefreshInterceptor) {
          originalRequest._retry = true

          try {
            // Delegate to auth store's refresh (single source of truth)
            const { useAuthStore } = await import('@/features/authentication')
            await useAuthStore.getState().refreshToken()

            // Cookies updated by store, retry the request
            return this.axiosInstance.request(originalRequest)
          } catch (refreshError) {
            // Refresh failed - store already dispatched event and cleared state
            return Promise.reject(new APIError(error))
          }
        }

        // Enhanced error logging based on configuration
        if (this.config.enableLogging) {
          const startTime = (error.config as ExtendedAxiosRequestConfig)?._requestStartTime
          const duration = startTime ? Date.now() - startTime : undefined

          const errorData = {
            method: error.config?.method?.toUpperCase(),
            url: error.config?.url,
            status: error.response?.status,
            statusText: error.response?.statusText,
            requestId: error.response?.headers['x-request-id'] || error.config?.headers?.['X-Request-Id'],
            errorCode: error.code,
            ...(duration && { duration: `${duration}ms` }),
            ...(this.config.logLevel === 'debug' && {
              requestData: error.config?.data,
              responseData: error.response?.data,
              stack: error.stack
            })
          }

          const logLevel = this.config.logLevel || 'info'
          const message = `[API ERROR] ${errorData.method} ${errorData.url}`

          // Log based on error severity and configuration
          if (errorData.status && errorData.status >= 500) {
            console.error(message, errorData)
          } else if (errorData.status && errorData.status >= 400) {
            if (logLevel === 'debug' || logLevel === 'info') {
              console.warn(message, errorData)
            }
          } else {
            // Network errors, timeouts, etc.
            console.error(message, errorData)
          }

          // Performance logging for failed requests
          if (this.config.enablePerformanceLogging && duration) {
            console.debug(`[PERF ERROR] ${errorData.method} ${errorData.url}: ${duration}ms (failed)`)
          }
        }

        return Promise.reject(new APIError(error))
      }
    )
  }

  // Extract data from API response wrapper
  private extractData<T>(response: AxiosResponse<APIResponse<T>>): T {
    // Handle 204 No Content responses (DELETE endpoints, etc.)
    if (response.status === 204) {
      return undefined as T
    }

    const responseData = response.data

    // Defensive validation with helpful error messages
    if (!responseData) {
      console.error('[API] Response data is undefined:', response)
      throw new Error('API response data is undefined')
    }

    if (responseData.success === undefined) {
      console.error('[API] Response missing success field:', responseData)
      throw new Error('API response missing success field')
    }

    if (!responseData.success) {
      console.error('[API] Response indicates failure:', responseData)
      throw new Error('API response indicates failure but was not caught by error interceptor')
    }

    if (responseData.data === undefined) {
      console.error('[API] Response missing data field:', responseData)
      throw new Error('API response missing data field')
    }

    return responseData.data
  }

  // Convert backend pagination format to frontend format
  private convertPagination(backendPagination: BackendPagination): Pagination {
    return {
      page: backendPagination.page,
      limit: backendPagination.limit,
      total: backendPagination.total,
      totalPages: backendPagination.total_pages,
      hasNext: backendPagination.has_next,
      hasPrev: backendPagination.has_prev
    }
  }

  // Extract paginated data from API response (preserves pagination metadata)
  private extractPaginatedData<T>(response: AxiosResponse<APIResponse<T[]>>): PaginatedResponse<T> {
    const { data, success, meta } = response.data

    if (!success) {
      throw new Error('API response indicates failure but was not caught by error interceptor')
    }

    // Check if pagination metadata exists
    const backendPagination = meta?.pagination as BackendPagination
    if (!backendPagination) {
      throw new Error('No pagination metadata found in response. Use regular get() method for non-paginated responses.')
    }

    return {
      data: data ?? [], // Normalize null to empty array (defensive against backend bugs)
      pagination: this.convertPagination(backendPagination)
    }
  }

  // File upload with progress tracking
  async uploadFile<T = unknown>(
    endpoint: string,
    file: File | Blob,
    options: {
      fieldName?: string
      additionalFields?: Record<string, string | number | boolean>
      onProgress?: (progress: number) => void
      retries?: number
    } = {}
  ): Promise<T> {
    const {
      fieldName = 'file',
      additionalFields = {},
      onProgress,
      retries
    } = options

    return this.executeWithRetry(async () => {
      const formData = new FormData()
      formData.append(fieldName, file)
      
      // Add additional fields to form data
      Object.entries(additionalFields).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          formData.append(key, String(value))
        }
      })

      const response = await this.axiosInstance.post<APIResponse<T>>(endpoint, formData, {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
        onUploadProgress: (progressEvent) => {
          if (onProgress && progressEvent.total) {
            const progress = Math.round((progressEvent.loaded * 100) / progressEvent.total)
            onProgress(progress)
          }
        },
      })

      return this.extractData(response)
    }, retries)
  }

  // Batch file upload with progress tracking
  async uploadFiles<T = unknown>(
    endpoint: string,
    files: Array<File | Blob>,
    options: {
      fieldName?: string
      additionalFields?: Record<string, string | number | boolean>
      onProgress?: (fileIndex: number, progress: number) => void
      onComplete?: (fileIndex: number) => void
      retries?: number
    } = {}
  ): Promise<T[]> {
    const {
      fieldName = 'files',
      additionalFields = {},
      onProgress,
      onComplete,
      retries
    } = options

    const results: T[] = []
    
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      
      try {
        const result = await this.uploadFile<T>(endpoint, file, {
          fieldName: `${fieldName}[${i}]`,
          additionalFields: { ...additionalFields, fileIndex: i },
          onProgress: (progress) => onProgress?.(i, progress),
          retries
        })
        
        results.push(result)
        onComplete?.(i)
      } catch (error) {
        console.error(`[API] File upload failed for file ${i}:`, error)
        throw error
      }
    }

    return results
  }

  // Retry logic with exponential backoff
  private async executeWithRetry<T>(
    operation: () => Promise<T>,
    customRetries?: number
  ): Promise<T> {
    const maxRetries = customRetries ?? this.config.retries ?? 3
    const baseDelay = this.config.retryDelay ?? 1000

    let lastError: Error | APIError | unknown
    for (let attempt = 0; attempt <= maxRetries; attempt++) {
      try {
        return await operation()
      } catch (error: unknown) {
        lastError = error
        
        // Don't retry on certain errors
        if (!this.shouldRetry(error, attempt) || attempt === maxRetries) {
          throw error
        }

        // Calculate exponential backoff delay
        const delay = baseDelay * Math.pow(2, attempt) + Math.random() * 1000
        
        if (this.config.enableLogging) {
          const errorMessage = error instanceof Error ? error.message : 'Unknown error'
          const errorStatus = (error as APIError).statusCode || (error as { response?: { status?: number } }).response?.status
          const errorEndpoint = (error as { config?: { url?: string } }).config?.url

          console.warn(`[API Retry] Attempt ${attempt + 1}/${maxRetries + 1} failed, retrying in ${Math.round(delay)}ms`, {
            error: errorMessage,
            status: errorStatus,
            endpoint: errorEndpoint
          })
        }

        // Wait before retrying
        await this.delay(delay)
      }
    }

    throw lastError
  }

  private shouldRetry(error: Error | APIError | unknown, attempt: number): boolean {
    // Handle BrokleAPIError (wrapped) vs raw axios error
    const status = (error as APIError).statusCode || (error as { response?: { status?: number } }).response?.status

    // Never retry auth failures
    if (status === 401) return false

    // Never retry client errors (400-499) except specific cases
    if (status !== undefined && status >= 400 && status < 500) {
      return status === 429 || status === 408
    }

    // Retry server errors and network issues
    const hasResponse = !!(error as { response?: unknown }).response
    return (status !== undefined && status >= 500) || (!status && !hasResponse)
  }

  private delay(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms))
  }

  // Utility methods

  getBaseURL(): string {
    return this.axiosInstance.defaults.baseURL || ''
  }

  // Cookie helper method
  private getCookie(name: string): string | null {
    if (typeof document === 'undefined') return null

    const value = `; ${document.cookie}`
    const parts = value.split(`; ${name}=`)
    if (parts.length === 2) {
      return parts.pop()?.split(';').shift() || null
    }
    return null
  }

  // Note: Token refresh logic removed - auth store owns refresh (single source of truth)
  // See web/src/stores/auth-store.ts for refresh implementation

  // Development helper
  debug(): void {
    if (process.env.NODE_ENV !== 'development') return

    console.group('🌐 BrokleAPIClient Debug')
    console.log('Base URL:', this.getBaseURL())
    console.log('Default Headers:', this.axiosInstance.defaults.headers)
    console.log('Timeout:', this.axiosInstance.defaults.timeout)
    console.log('With Credentials:', this.axiosInstance.defaults.withCredentials)
    console.log('Retry Config:', {
      retries: this.config.retries,
      retryDelay: this.config.retryDelay
    })
    console.log('Has CSRF Token:', !!this.getCookie('csrf_token'))
    console.groupEnd()
  }
}