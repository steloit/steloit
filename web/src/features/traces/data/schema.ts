import { z } from 'zod'

// ============================================================================
// Manual Type Definitions (Required for Recursive Schemas)
// ============================================================================
//
// Note: TypeScript cannot statically infer types for circular/recursive schemas.
// We define types manually FIRST, then provide them as type hints to Zod schemas.
// This is the official Zod pattern for recursive data structures.
//
// Reference: https://zod.dev/?id=recursive-types
// ============================================================================

/**
 * Score Type - Quality/evaluation score for a trace or span
 */
export type Score = {
  // Identifiers
  id: string
  project_id: string
  trace_id: string
  span_id: string

  // Score Data
  name: string
  value?: number
  string_value?: string
  type: string // NUMERIC, CATEGORICAL, BOOLEAN

  // Metadata
  source: string // API, ANNOTATION, EVAL
  comment?: string

  // Evaluator Info
  evaluator_name?: string
  evaluator_version?: string
  evaluator_config?: Record<string, string>
  author_user_id?: string

  // Timestamps
  timestamp: Date
  version?: string
}

/**
 * Span Type - Individual operation within a trace (recursive)
 */
export type Span = {
  // OTEL Identifiers
  span_id: string
  trace_id: string
  parent_span_id?: string
  project_id: string

  // Metadata
  span_name: string
  span_kind: number // UInt8: 0-5

  // Timing
  start_time: Date
  end_time?: Date
  duration?: number // Nanoseconds (OTLP spec)

  // OTEL Status
  status_code: number // UInt8: 0=UNSET, 1=OK, 2=ERROR
  status_message?: string
  has_error: boolean // Semantic flag (true when status_code=2)

  // Attributes (new schema names)
  attributes?: Record<string, any>
  metadata?: Record<string, any>

  // I/O Data
  input?: string
  output?: string

  // OTEL Events/Links
  events_timestamp?: Date[]
  events_name?: string[]
  events_attributes?: string[]
  links_trace_id?: string[]
  links_span_id?: string[]
  links_attributes?: string[]

  // Materialized Columns - gen_ai.*
  gen_ai_operation_name?: string
  gen_ai_provider_name?: string
  gen_ai_request_model?: string
  gen_ai_request_max_tokens?: number
  gen_ai_request_temperature?: number
  gen_ai_request_top_p?: number
  gen_ai_usage_input_tokens?: number
  gen_ai_usage_output_tokens?: number

  // Materialized Columns (from attributes JSON)
  model_name?: string          // From attributes.gen_ai.request.model
  provider_name?: string       // From attributes.gen_ai.provider.name
  span_type?: string          // From attributes.brokle.span.type
  version?: string            // From span_attributes['brokle.span.version']
  level?: string              // From attributes.brokle.span.level

  // Usage & Cost Maps
  usage_details?: Record<string, number>       // Map: {"input": 100, "output": 50, "total": 150}
  cost_details?: Record<string, string>        // Map: {"input": "0.001", "output": "0.002", "total": "0.003"}
  pricing_snapshot?: Record<string, string>    // Map: {"input_price_per_million": "5.00", ...}
  total_cost?: number                          // Decimal(18,12): Pre-computed total

  // Timestamps
  created_at?: Date

  // Relationships (RECURSIVE!)
  scores?: Score[]
  child_spans?: Span[] // Self-reference
}

/**
 * Trace Type - Top-level telemetry trace
 */
export type Trace = {
  // OTEL Identifiers
  trace_id: string // 32 hex characters
  project_id: string // UUID

  // Metadata
  name: string
  user_id?: string
  session_id?: string // Virtual session

  // Timing
  start_time: Date
  end_time?: Date
  duration?: number // Nanoseconds (OTLP spec)

  // OTEL Status
  status_code: number // UInt8: 0=UNSET, 1=OK, 2=ERROR
  status_message?: string
  has_error: boolean // Semantic flag (true when status_code=2)

  // Attributes
  resource_attributes?: Record<string, any>

  // I/O Data (ZSTD compressed in backend)
  input?: string
  output?: string

  // Tags
  tags?: string[]

  // Extracted Attributes
  environment?: string
  service_name?: string
  service_version?: string
  release?: string

  // Model/Provider info (from root span)
  model_name?: string
  provider_name?: string

  // Flags
  bookmarked: boolean
  public: boolean

  // Versioning
  version?: string

  // Computed Fields (from transformers)
  cost?: number
  tokens?: number
  spanCount: number

  // Timestamps
  created_at?: Date
  updated_at?: Date

  // Relationships
  spans?: Span[]
  scores?: Score[]
}

// ============================================================================
// Zod Schemas with Type Hints
// ============================================================================

/**
 * Score Schema - No recursion, simple z.object()
 *
 * Backend: internal/core/domain/observability/entity.go::Score
 */
export const scoreSchema: z.ZodType<Score> = z.object({
  // Identifiers
  id: z.string(),
  project_id: z.string(),
  trace_id: z.string(),
  span_id: z.string(),

  // Score Data
  name: z.string(),
  value: z.number().optional(),
  string_value: z.string().optional(),
  type: z.string(), // NUMERIC, CATEGORICAL, BOOLEAN

  // Metadata
  source: z.string(), // API, ANNOTATION, EVAL
  comment: z.string().optional(),

  // Evaluator Info
  evaluator_name: z.string().optional(),
  evaluator_version: z.string().optional(),
  evaluator_config: z.record(z.string(), z.string()).optional(),
  author_user_id: z.string().optional(),

  // Timestamps
  timestamp: z.date(),
  version: z.string().optional(),
})

/**
 * Span Schema - Self-recursive, requires z.lazy()
 *
 * Backend: internal/core/domain/observability/entity.go::Span
 * ClickHouse: migrations/clickhouse/20251112000002_create_otel_spans.up.sql
 */
export const spanSchema: z.ZodType<Span> = z.lazy(() =>
  z.object({
    // OTEL Identifiers
    span_id: z.string(),
    trace_id: z.string(),
    parent_span_id: z.string().optional(),
    project_id: z.string(),

    // Metadata
    span_name: z.string(),
    span_kind: z.number(), // UInt8: 0-5 (UNSPECIFIED, INTERNAL, SERVER, CLIENT, PRODUCER, CONSUMER)

    // Timing
    start_time: z.date(),
    end_time: z.date().optional(),
    duration: z.number().optional(), // Nanoseconds (OTLP spec)

    // OTEL Status
    status_code: z.number(), // UInt8: 0=UNSET, 1=OK, 2=ERROR
    status_message: z.string().optional(),
    has_error: z.boolean(), // Semantic flag (true when status_code=2)

    // Attributes (new schema names - already parsed JSON objects, not strings)
    attributes: z.record(z.string(), z.any()).optional(),
    metadata: z.record(z.string(), z.any()).optional(),

    // I/O Data
    input: z.string().optional(),
    output: z.string().optional(),

    // OTEL Events/Links (arrays)
    events_timestamp: z.array(z.date()).optional(),
    events_name: z.array(z.string()).optional(),
    events_attributes: z.array(z.string()).optional(),
    links_trace_id: z.array(z.string()).optional(),
    links_span_id: z.array(z.string()).optional(),
    links_attributes: z.array(z.string()).optional(),

    // Materialized Columns (16 total - read-only from backend)
    // gen_ai.* attributes
    gen_ai_operation_name: z.string().optional(),
    gen_ai_provider_name: z.string().optional(),
    gen_ai_request_model: z.string().optional(),
    gen_ai_request_max_tokens: z.number().optional(),
    gen_ai_request_temperature: z.number().optional(),
    gen_ai_request_top_p: z.number().optional(),
    gen_ai_usage_input_tokens: z.number().optional(),
    gen_ai_usage_output_tokens: z.number().optional(),

    // Materialized columns (from attributes JSON)
    model_name: z.string().optional(),
    provider_name: z.string().optional(),
    span_type: z.string().optional(),
    version: z.string().optional(),
    level: z.string().optional(),

    // Usage & Cost Maps
    usage_details: z.record(z.number()).optional(),
    cost_details: z.record(z.string()).optional(),
    pricing_snapshot: z.record(z.string()).optional(),
    total_cost: z.number().optional(),

    // Timestamps
    created_at: z.date().optional(),

    // Relationships (optional)
    scores: z.array(scoreSchema).optional(),
    child_spans: z.array(spanSchema).optional(), // Self-reference with type hint!
  })
)

/**
 * Trace Schema - References Span/Score, requires z.lazy()
 *
 * Backend: internal/core/domain/observability/entity.go::Trace
 * ClickHouse: migrations/clickhouse/20251112000001_create_otel_traces.up.sql
 */
export const traceSchema: z.ZodType<Trace> = z.lazy(() =>
  z.object({
    // OTEL Identifiers
    trace_id: z.string(), // 32 hex characters
    project_id: z.string(), // UUID

    // Metadata
    name: z.string(),
    user_id: z.string().optional(),
    session_id: z.string().optional(), // Virtual session

    // Timing
    start_time: z.date(),
    end_time: z.date().optional(),
    duration: z.number().optional(), // Nanoseconds (OTLP spec)

    // OTEL Status
    status_code: z.number(), // UInt8: 0=UNSET, 1=OK, 2=ERROR
    status_message: z.string().optional(),
    has_error: z.boolean(), // Semantic flag (true when status_code=2)

    // Attributes (already parsed JSON object, not string)
    resource_attributes: z.record(z.string(), z.any()).optional(),

    // I/O Data (ZSTD compressed in backend)
    input: z.string().optional(),
    output: z.string().optional(),

    // Tags
    tags: z.array(z.string()).optional(),

    // Extracted Attributes
    environment: z.string().optional(),
    service_name: z.string().optional(),
    service_version: z.string().optional(),
    release: z.string().optional(),

    // Model/Provider info (from root span)
    model_name: z.string().optional(),
    provider_name: z.string().optional(),

    // Flags
    bookmarked: z.boolean().default(false),
    public: z.boolean().default(false),

    // Versioning
    version: z.string().optional(),

    // Computed Fields (from transformers)
    cost: z.number().optional(),
    tokens: z.number().optional(),
    spanCount: z.number().default(0),

    // Timestamps
    created_at: z.date().optional(),
    updated_at: z.date().optional(),

    // Relationships (optional)
    spans: z.array(spanSchema).optional(),
    scores: z.array(scoreSchema).optional(),
  })
)
