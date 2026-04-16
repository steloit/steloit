import { create } from 'zustand'
import type { ChatMessage, ModelConfig, RunHistoryEntry, ToolCall } from '../types'
import { createMessage } from '../types'

export interface PlaygroundWindow {
  id: string
  name: string // Editable window name (e.g., "Window 1", "GPT-4 Test")

  // Messages are always the primary data (chat only, no text mode)
  messages: ChatMessage[]

  // Prompt linking (Opik-style - track prompt AND version)
  loadedFromPromptId: string | null        // Which prompt was loaded (if any)
  loadedFromPromptName: string | null      // Display name for "Linked: X"
  loadedFromPromptVersionId: string | null // Version ID (UUID) for precise tracking
  loadedFromPromptVersionNumber: number | null // Version number (e.g., 5) for display
  loadedTemplate: string | null            // Original template JSON for change detection

  // Span linking (for "Open in Playground" from traces)
  loadedFromSpanId: string | null          // Span ID that was loaded
  loadedFromSpanName: string | null        // Span name for display
  loadedFromTraceId: string | null         // Parent trace ID for reference

  // Shared
  variables: Record<string, string>
  config: ModelConfig | null
  createTrace: boolean

  // Last execution (ephemeral - not persisted to DB)
  lastOutput: string | null
  lastMetrics: {
    model?: string
    prompt_tokens?: number
    completion_tokens?: number
    total_tokens?: number
    cost?: number
    ttft_ms?: number
    total_duration_ms?: number
  } | null
  lastToolCalls: ToolCall[] | null // Tool calls from last execution (if any)

  // Run history (in-memory, max 10 entries per window)
  // Opik pattern: entries marked stale when prompt changes
  runHistory: RunHistoryEntry[]

  // Execution state (UI only)
  isExecuting: boolean

  // Content snapshot for dirty detection (JSON string of saveable content)
  // isDirty is computed from: currentSnapshot !== lastSavedSnapshot
  lastSavedSnapshot: string | null
}

interface PlaygroundState {
  // Current session ID (from URL)
  currentSessionId: string | null

  // Windows (up to 3)
  windows: PlaygroundWindow[]

  // Shared variables across windows
  sharedVariables: Record<string, string>
  useSharedVariables: boolean

  // Global execution state
  isExecutingAll: boolean

  // Actions - Session
  setCurrentSessionId: (sessionId: string | null) => void

  // Actions - Windows
  addWindow: () => void
  removeWindow: (index: number) => void
  updateWindow: (index: number, updates: Partial<PlaygroundWindow>) => void
  duplicateWindow: (index: number) => void
  renameWindow: (index: number, name: string) => void
  setLastSavedSnapshot: (index: number, snapshot: string | null) => void
  setAllSavedSnapshots: () => void

  // Actions - Run History (Opik pattern: in-memory with stale marking)
  addRunToHistory: (index: number, entry: Omit<RunHistoryEntry, 'id' | 'timestamp' | 'isStale'>) => void
  restoreFromHistory: (windowIndex: number, historyId: string) => void
  clearWindowHistory: (index: number) => void
  markHistoryAsStale: (index: number) => void

  // Actions - Variables
  setSharedVariables: (variables: Record<string, string>) => void
  toggleSharedVariables: () => void

  // Actions - Execution
  setWindowExecuting: (index: number, isExecuting: boolean) => void
  setWindowOutput: (
    index: number,
    output: string,
    metrics: PlaygroundWindow['lastMetrics'],
    inputSnapshot?: { messages: ChatMessage[]; variables: Record<string, string>; config: ModelConfig | null },
    toolCalls?: ToolCall[] | null
  ) => void
  setExecutingAll: (isExecuting: boolean) => void

  // Reset
  clearAll: () => void

  // Session Loading - atomic multi-window initialization
  loadWindowsFromSession: (windowsData: Array<{
    messages: ChatMessage[]
    variables?: Record<string, string>
    config?: ModelConfig | null
    loadedFromPromptId?: string | null
    loadedFromPromptName?: string | null
    loadedFromPromptVersionId?: string | null
    loadedFromPromptVersionNumber?: number | null
    loadedTemplate?: string | null
  }>) => void

  // Prompt Linking - unlink a prompt from a window
  unlinkPrompt: (windowIndex: number) => void

  // Load prompt directly into store (for "Try in Playground" feature)
  // This is the new in-memory approach - no session creation until save
  loadFromPrompt: (promptData: {
    messages: ChatMessage[]
    config?: ModelConfig | null
    loadedFromPromptId: string
    loadedFromPromptName: string
    loadedFromPromptVersionId?: string
    loadedFromPromptVersionNumber?: number
    loadedTemplate?: string
  }) => void

  // Load span directly into store (for "Open in Playground" from traces)
  loadFromSpan: (spanData: {
    messages: ChatMessage[]
    config?: ModelConfig | null
    loadedFromSpanId: string
    loadedFromSpanName: string
    loadedFromTraceId: string
  }) => void

  // Span Linking - unlink a span from a window
  unlinkSpan: (windowIndex: number) => void
}

const createEmptyWindow = (index?: number): PlaygroundWindow => ({
  id: crypto.randomUUID(),
  name: index !== undefined ? `Window ${index + 1}` : 'New Window',
  messages: [
    createMessage('system', ''),
    createMessage('user', ''),
  ],
  loadedFromPromptId: null,
  loadedFromPromptName: null,
  loadedFromPromptVersionId: null,
  loadedFromPromptVersionNumber: null,
  loadedTemplate: null,
  loadedFromSpanId: null,
  loadedFromSpanName: null,
  loadedFromTraceId: null,
  variables: {},
  config: null,
  createTrace: false, // Default OFF for playground (ephemeral)
  lastOutput: null,
  lastMetrics: null,
  lastToolCalls: null, // Tool calls from last execution
  runHistory: [], // In-memory run history (max 10)
  isExecuting: false,
  lastSavedSnapshot: null, // null = never saved, isDirty computed from comparison
})

/**
 * Creates a JSON snapshot of window's saveable content for dirty comparison.
 * This is the industry standard approach (Notion, Google Docs):
 * isDirty = currentSnapshot !== lastSavedSnapshot
 */
export const createContentSnapshot = (window: PlaygroundWindow): string => {
  // Strip IDs from messages for comparison (IDs are for drag-drop, not content)
  const messagesForSnapshot = window.messages.map(({ role, content }) => ({ role, content }))
  return JSON.stringify({
    messages: messagesForSnapshot,
    variables: window.variables,
    config: window.config,
  })
}

/**
 * Computes whether a window has unsaved changes.
 * Returns false if never saved (no point saving empty state).
 */
export const isWindowDirty = (window: PlaygroundWindow): boolean => {
  if (!window.lastSavedSnapshot) return false // Never saved = not dirty
  return createContentSnapshot(window) !== window.lastSavedSnapshot
}

// Store is now purely in-memory - database is the source of truth
// This store manages UI-only state (execution, dirty flags, etc.)
export const usePlaygroundStore = create<PlaygroundState>()((set, get) => ({
  currentSessionId: null,
  windows: [createEmptyWindow(0)],
  sharedVariables: {},
  useSharedVariables: false,
  isExecutingAll: false,

  setCurrentSessionId: (sessionId) => set({ currentSessionId: sessionId }),

  addWindow: () => {
    const { windows } = get()
    if (windows.length >= 20) return // Practical limit to prevent memory issues
    const newWindow = createEmptyWindow(windows.length)
    // Initialize snapshot so isDirty can detect changes
    newWindow.lastSavedSnapshot = createContentSnapshot(newWindow)
    set({ windows: [...windows, newWindow] })
  },

  removeWindow: (index) => {
    const { windows } = get()
    if (windows.length <= 1) return
    set({ windows: windows.filter((_, i) => i !== index) })
  },

  updateWindow: (index, updates) => {
    const { windows } = get()
    const newWindows = [...windows]
    // Note: isDirty is now COMPUTED from lastSavedSnapshot comparison
    // No need to manually track dirty state - just apply updates
    newWindows[index] = {
      ...newWindows[index],
      ...updates,
    }
    set({ windows: newWindows })
  },

  duplicateWindow: (index) => {
    const { windows } = get()
    if (windows.length >= 20) return // Practical limit to prevent memory issues
    const win = windows[index]
    const newWindow: PlaygroundWindow = {
      ...win,
      id: crypto.randomUUID(),
      name: `${win.name} (copy)`,
      lastOutput: null,
      lastMetrics: null,
      runHistory: [], // Don't copy history
      isExecuting: false,
      lastSavedSnapshot: null, // Will be set below
    }
    // Initialize snapshot so isDirty can detect changes
    newWindow.lastSavedSnapshot = createContentSnapshot(newWindow)
    set({ windows: [...windows, newWindow] })
  },

  renameWindow: (index, name) => {
    const { windows } = get()
    const newWindows = [...windows]
    if (newWindows[index]) {
      newWindows[index] = { ...newWindows[index], name }
    }
    set({ windows: newWindows })
  },

  setLastSavedSnapshot: (index, snapshot) => {
    const { windows } = get()
    const newWindows = [...windows]
    if (newWindows[index]) {
      newWindows[index] = { ...newWindows[index], lastSavedSnapshot: snapshot }
    }
    set({ windows: newWindows })
  },

  setAllSavedSnapshots: () => {
    const { windows } = get()
    const newWindows = windows.map(w => ({
      ...w,
      lastSavedSnapshot: createContentSnapshot(w),
    }))
    set({ windows: newWindows })
  },

  setSharedVariables: (variables) => set({ sharedVariables: variables }),

  toggleSharedVariables: () =>
    set((state) => ({ useSharedVariables: !state.useSharedVariables })),

  setWindowExecuting: (index, isExecuting) => {
    const { windows } = get()
    const newWindows = [...windows]
    newWindows[index] = { ...newWindows[index], isExecuting }
    set({ windows: newWindows })
  },

  setWindowOutput: (index, output, metrics, inputSnapshot, toolCalls) => {
    const { windows } = get()
    const window = windows[index]
    if (!window) return

    // Use captured inputs if provided (execution-time snapshot), otherwise fall back to current state
    // This ensures history entries reflect inputs at execution start, not when streaming ends
    const historyMessages = inputSnapshot?.messages ?? window.messages
    const historyVariables = inputSnapshot?.variables ?? window.variables
    const historyConfig = inputSnapshot?.config ?? window.config

    // Create history entry from captured state
    const historyEntry: RunHistoryEntry = {
      id: crypto.randomUUID(),
      content: output,
      metrics: metrics ? {
        prompt_tokens: metrics.prompt_tokens,
        completion_tokens: metrics.completion_tokens,
        total_tokens: metrics.total_tokens,
        cost: metrics.cost,
        latency_ms: metrics.total_duration_ms,
        ttft_ms: metrics.ttft_ms,
        model: metrics.model,
      } : null,
      timestamp: new Date().toISOString(),
      isStale: false, // Fresh run
      messages: historyMessages.map(m => ({ ...m })),
      variables: { ...historyVariables },
      config: historyConfig ? { ...historyConfig } : null,
    }

    // Add to history (keep max 10, newest first)
    const newHistory = [historyEntry, ...window.runHistory].slice(0, 10)

    const newWindows = [...windows]
    newWindows[index] = {
      ...window,
      lastOutput: output,
      lastMetrics: metrics,
      lastToolCalls: toolCalls || null,
      runHistory: newHistory,
      isExecuting: false,
    }
    set({ windows: newWindows })
  },

  setExecutingAll: (isExecuting) => set({ isExecutingAll: isExecuting }),

  // Run History Actions (Opik pattern)
  addRunToHistory: (index, entry) => {
    const { windows } = get()
    const window = windows[index]
    if (!window) return

    const historyEntry: RunHistoryEntry = {
      ...entry,
      id: crypto.randomUUID(),
      timestamp: new Date().toISOString(),
      isStale: false,
    }

    const newHistory = [historyEntry, ...window.runHistory].slice(0, 10)
    const newWindows = [...windows]
    newWindows[index] = { ...window, runHistory: newHistory }
    set({ windows: newWindows })
  },

  restoreFromHistory: (windowIndex, historyId) => {
    const { windows } = get()
    const window = windows[windowIndex]
    if (!window) return

    const historyEntry = window.runHistory.find(h => h.id === historyId)
    if (!historyEntry) return

    const newWindows = [...windows]
    newWindows[windowIndex] = {
      ...window,
      messages: historyEntry.messages.map(m => ({ ...m, id: crypto.randomUUID() })),
      variables: { ...historyEntry.variables },
      config: historyEntry.config ? { ...historyEntry.config } : null,
      lastOutput: historyEntry.content,
      lastMetrics: historyEntry.metrics ? {
        model: historyEntry.metrics.model,
        prompt_tokens: historyEntry.metrics.prompt_tokens,
        completion_tokens: historyEntry.metrics.completion_tokens,
        total_tokens: historyEntry.metrics.total_tokens,
        cost: historyEntry.metrics.cost,
        ttft_ms: historyEntry.metrics.ttft_ms,
        total_duration_ms: historyEntry.metrics.latency_ms,
      } : null,
    }
    set({ windows: newWindows })
  },

  clearWindowHistory: (index) => {
    const { windows } = get()
    const newWindows = [...windows]
    if (newWindows[index]) {
      newWindows[index] = { ...newWindows[index], runHistory: [] }
    }
    set({ windows: newWindows })
  },

  markHistoryAsStale: (index) => {
    const { windows } = get()
    const window = windows[index]
    if (!window || window.runHistory.length === 0) return

    const newWindows = [...windows]
    newWindows[index] = {
      ...window,
      runHistory: window.runHistory.map(entry => ({ ...entry, isStale: true })),
    }
    set({ windows: newWindows })
  },

  clearAll: () =>
    set({
      currentSessionId: null,
      windows: [createEmptyWindow(0)],
      sharedVariables: {},
      useSharedVariables: false,
      isExecutingAll: false,
    }),

  // Atomic multi-window initialization from session data
  // This avoids race conditions with addWindow() + updateWindow()
  loadWindowsFromSession: (windowsData) => {
    const newWindows = windowsData.map((data, index) => {
      // Ensure messages have IDs (migration from old format)
      const messagesWithIds = data.messages.map(msg =>
        msg.id ? msg : { ...msg, id: crypto.randomUUID() }
      )
      const window: PlaygroundWindow = {
        ...createEmptyWindow(index),
        messages: messagesWithIds,
        loadedFromPromptId: data.loadedFromPromptId || null,
        loadedFromPromptName: data.loadedFromPromptName || null,
        loadedFromPromptVersionId: data.loadedFromPromptVersionId || null,
        loadedFromPromptVersionNumber: data.loadedFromPromptVersionNumber || null,
        loadedTemplate: data.loadedTemplate || null,
        variables: data.variables || {},
        config: data.config || null,
      }
      // Set snapshot for dirty detection - marks this content as "saved"
      window.lastSavedSnapshot = createContentSnapshot(window)
      return window
    })
    set({ windows: newWindows.length > 0 ? newWindows : [createEmptyWindow(0)] })
  },

  // Unlink a prompt from a window (keeps content, removes link)
  unlinkPrompt: (windowIndex) => {
    const { windows } = get()
    const newWindows = [...windows]
    if (newWindows[windowIndex]) {
      newWindows[windowIndex] = {
        ...newWindows[windowIndex],
        loadedFromPromptId: null,
        loadedFromPromptName: null,
        loadedFromPromptVersionId: null,
        loadedFromPromptVersionNumber: null,
        loadedTemplate: null,
      }
    }
    set({ windows: newWindows })
  },

  // Load prompt directly into store (for "Try in Playground" feature)
  // This replaces the sessionStorage-based cache transfer
  loadFromPrompt: (promptData) => {
    // Ensure messages have IDs
    const messagesWithIds = promptData.messages.map(msg =>
      msg.id ? msg : { ...msg, id: crypto.randomUUID() }
    )

    const window: PlaygroundWindow = {
      ...createEmptyWindow(0),
      name: promptData.loadedFromPromptName || 'Window 1', // Use prompt name if available
      messages: messagesWithIds,
      loadedFromPromptId: promptData.loadedFromPromptId,
      loadedFromPromptName: promptData.loadedFromPromptName,
      loadedFromPromptVersionId: promptData.loadedFromPromptVersionId || null,
      loadedFromPromptVersionNumber: promptData.loadedFromPromptVersionNumber || null,
      loadedTemplate: promptData.loadedTemplate || null,
      config: promptData.config || null,
    }

    // Set snapshot for dirty detection
    window.lastSavedSnapshot = createContentSnapshot(window)

    // Replace all windows with this single window containing the prompt
    set({
      currentSessionId: null, // Clear any existing session ID
      windows: [window],
    })
  },

  // Load span directly into store (for "Open in Playground" from traces)
  loadFromSpan: (spanData) => {
    // Ensure messages have IDs
    const messagesWithIds = spanData.messages.map(msg =>
      msg.id ? msg : { ...msg, id: crypto.randomUUID() }
    )

    const window: PlaygroundWindow = {
      ...createEmptyWindow(0),
      name: spanData.loadedFromSpanName || 'Window 1', // Use span name if available
      messages: messagesWithIds,
      loadedFromSpanId: spanData.loadedFromSpanId,
      loadedFromSpanName: spanData.loadedFromSpanName,
      loadedFromTraceId: spanData.loadedFromTraceId,
      config: spanData.config || null,
    }

    // Set snapshot for dirty detection
    window.lastSavedSnapshot = createContentSnapshot(window)

    // Replace all windows with this single window containing the span data
    set({
      currentSessionId: null, // Clear any existing session ID
      windows: [window],
    })
  },

  // Unlink a span from a window (keeps content, removes link)
  unlinkSpan: (windowIndex) => {
    const { windows } = get()
    const newWindows = [...windows]
    if (newWindows[windowIndex]) {
      newWindows[windowIndex] = {
        ...newWindows[windowIndex],
        loadedFromSpanId: null,
        loadedFromSpanName: null,
        loadedFromTraceId: null,
      }
    }
    set({ windows: newWindows })
  },
}))
