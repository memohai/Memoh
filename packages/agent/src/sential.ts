import { createHash } from 'node:crypto'

export interface SentialOptions {
  ngramSize?: number;
  windowSize?: number;
  overlapThreshold?: number;
}

export interface TextLoopGuardOptions extends SentialOptions {
  consecutiveHitsToAbort?: number;
  minNewGramsPerChunk?: number;
}

export interface SentialInspectResult {
  hit: boolean;
  overlap: number;
  matchedGrams: number;
  newGrams: number;
}

export interface TextLoopGuardInspectResult extends SentialInspectResult {
  streak: number;
  abort: boolean;
}

export interface ToolLoopInspectInput {
  toolName: string;
  input: unknown;
}

export interface ToolLoopInspectResult {
  hash: string;
  repeatCount: number;
  breachCount: number;
  warn: boolean;
  abort: boolean;
}

export interface ToolLoopGuardOptions {
  repeatThreshold?: number;
  warningsBeforeAbort?: number;
  volatileKeys?: string[];
}

export interface Sential {
  inspect(text: string): SentialInspectResult;
  reset(): void;
}

export interface TextLoopGuard {
  inspect(text: string): TextLoopGuardInspectResult;
  reset(): void;
}

export interface TextLoopProbeBuffer {
  push(text: string): void;
  flush(): void;
}

export interface ToolLoopGuard {
  inspect(input: ToolLoopInspectInput): ToolLoopInspectResult;
  reset(): void;
}

const DEFAULT_NGRAM_SIZE = 10
const DEFAULT_WINDOW_SIZE = 1000
const DEFAULT_OVERLAP_THRESHOLD = 0.75
const DEFAULT_CONSECUTIVE_HITS_TO_ABORT = 10
const DEFAULT_MIN_NEW_GRAMS_PER_CHUNK = 1
const DEFAULT_TOOL_LOOP_REPEAT_THRESHOLD = 5
const DEFAULT_TOOL_LOOP_WARNINGS_BEFORE_ABORT = 1
const DEFAULT_VOLATILE_KEYS = [
  'toolCallId',
  'tool_call_id',
  'requestId',
  'request_id',
  'traceId',
  'trace_id',
  'spanId',
  'span_id',
  'sessionId',
  'session_id',
  'timestamp',
  'createdAt',
  'created_at',
  'updatedAt',
  'updated_at',
  'expiresAt',
  'expires_at',
  'nonce',
]
const VOLATILE_KEY_SUFFIXES = [
  'requestid',
  'traceid',
  'sessionid',
  'toolcallid',
  'timestamp',
  'createdat',
  'updatedat',
  'expiresat',
]

type NormalizedValue =
  | null
  | string
  | number
  | boolean
  | NormalizedValue[]
  | { [key: string]: NormalizedValue };

function validatePositiveInt(name: string, value: number): number {
  if (!Number.isFinite(value) || value <= 0 || !Number.isInteger(value)) {
    throw new Error(`${name} must be a positive integer`)
  }
  return value
}

function validateThreshold(value: number): number {
  if (!Number.isFinite(value) || value < 0 || value > 1) {
    throw new Error('overlapThreshold must be between 0 and 1')
  }
  return value
}

function normalizeChars(text: string): string[] {
  if (!text) return []
  return Array.from(text.normalize('NFC'))
}

function buildNgram(chars: string[], start: number, size: number): string {
  return chars.slice(start, start + size).join('')
}

function normalizeKeyName(key: string): string {
  return key
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]/g, '')
}

function isVolatileKey(key: string, volatileKeySet: Set<string>): boolean {
  const normalized = normalizeKeyName(key)
  if (!normalized) return false
  if (volatileKeySet.has(normalized)) return true
  return VOLATILE_KEY_SUFFIXES.some((suffix) => normalized.endsWith(suffix))
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  if (value === null || typeof value !== 'object') return false
  const prototype = Object.getPrototypeOf(value as object)
  return prototype === Object.prototype || prototype === null
}

function normalizeToolLoopValue(
  value: unknown,
  volatileKeySet: Set<string>,
  seen: WeakSet<object>,
): NormalizedValue | undefined {
  if (value === null) return null

  if (typeof value === 'string') return value.normalize('NFC')
  if (typeof value === 'boolean') return value
  if (typeof value === 'number') {
    return Number.isFinite(value) ? value : (String(value) as NormalizedValue)
  }
  if (typeof value === 'bigint') return value.toString()
  if (
    typeof value === 'undefined' ||
    typeof value === 'function' ||
    typeof value === 'symbol'
  ) {
    return undefined
  }

  if (value instanceof Date) {
    return value.toISOString()
  }

  if (Array.isArray(value)) {
    return value.map(
      (item) => normalizeToolLoopValue(item, volatileKeySet, seen) ?? null,
    )
  }

  if (!isPlainObject(value)) {
    const maybeRecord = value as { toJSON?: () => unknown }
    if (typeof maybeRecord.toJSON === 'function') {
      return normalizeToolLoopValue(maybeRecord.toJSON(), volatileKeySet, seen)
    }
    return String(value)
  }

  if (seen.has(value)) {
    return '[Circular]'
  }
  seen.add(value)

  const normalizedObject: { [key: string]: NormalizedValue } = {}
  const keys = Object.keys(value).sort()
  for (const key of keys) {
    if (isVolatileKey(key, volatileKeySet)) {
      continue
    }
    const normalized = normalizeToolLoopValue(value[key], volatileKeySet, seen)
    if (normalized !== undefined) {
      normalizedObject[key] = normalized
    }
  }

  seen.delete(value)
  return normalizedObject
}

function computeToolLoopHash(
  input: ToolLoopInspectInput,
  volatileKeySet: Set<string>,
): string {
  const payload = {
    toolName: input.toolName.trim(),
    input:
      normalizeToolLoopValue(input.input, volatileKeySet, new WeakSet()) ??
      null,
  }
  const serialized = JSON.stringify(payload)
  return createHash('sha256').update(serialized).digest('hex')
}

export function createSential(options: SentialOptions = {}): Sential {
  const ngramSize = validatePositiveInt(
    'ngramSize',
    options.ngramSize ?? DEFAULT_NGRAM_SIZE,
  )
  const windowSize = validatePositiveInt(
    'windowSize',
    options.windowSize ?? DEFAULT_WINDOW_SIZE,
  )
  const overlapThreshold = validateThreshold(
    options.overlapThreshold ?? DEFAULT_OVERLAP_THRESHOLD,
  )
  if (windowSize < ngramSize) {
    throw new Error('windowSize must be greater than or equal to ngramSize')
  }

  const windowChars: string[] = []
  const windowNgramQueue: string[] = []
  const historySet = new Set<string>()
  const historyCounts = new Map<string, number>()

  const addHistoryGram = (gram: string) => {
    const nextCount = (historyCounts.get(gram) ?? 0) + 1
    historyCounts.set(gram, nextCount)
    if (nextCount === 1) {
      historySet.add(gram)
    }
  }

  const removeHistoryGram = (gram: string) => {
    const prevCount = historyCounts.get(gram)
    if (!prevCount) return
    if (prevCount <= 1) {
      historyCounts.delete(gram)
      historySet.delete(gram)
      return
    }
    historyCounts.set(gram, prevCount - 1)
  }

  const pushWindowChar = (char: string) => {
    windowChars.push(char)

    if (windowChars.length >= ngramSize) {
      const gram = buildNgram(
        windowChars,
        windowChars.length - ngramSize,
        ngramSize,
      )
      windowNgramQueue.push(gram)
      addHistoryGram(gram)
    }

    if (windowChars.length <= windowSize) {
      return
    }

    windowChars.shift()
    const removedGram = windowNgramQueue.shift()
    if (removedGram) {
      removeHistoryGram(removedGram)
    }
  }

  return {
    inspect(text: string): SentialInspectResult {
      const incomingChars = normalizeChars(text)
      if (incomingChars.length === 0) {
        return {
          hit: false,
          overlap: 0,
          matchedGrams: 0,
          newGrams: 0,
        }
      }

      const contextSize = Math.max(ngramSize - 1, 0)
      const contextChars =
        contextSize > 0 ? windowChars.slice(-contextSize) : []
      const candidateChars = [...contextChars, ...incomingChars]

      let matchedGrams = 0
      let newGrams = 0
      const contextLength = contextChars.length

      if (candidateChars.length >= ngramSize) {
        for (let i = 0; i <= candidateChars.length - ngramSize; i += 1) {
          const gramEndIndex = i + ngramSize - 1
          if (gramEndIndex < contextLength) {
            continue
          }
          const gram = buildNgram(candidateChars, i, ngramSize)
          newGrams += 1
          if (historySet.has(gram)) {
            matchedGrams += 1
          }
        }
      }

      const overlap = newGrams === 0 ? 0 : matchedGrams / newGrams
      const hit = overlap > overlapThreshold

      for (const char of incomingChars) {
        pushWindowChar(char)
      }

      return {
        hit,
        overlap,
        matchedGrams,
        newGrams,
      }
    },
    reset(): void {
      windowChars.length = 0
      windowNgramQueue.length = 0
      historySet.clear()
      historyCounts.clear()
    },
  }
}

export function createTextLoopGuard(
  options: TextLoopGuardOptions = {},
): TextLoopGuard {
  const consecutiveHitsToAbort = validatePositiveInt(
    'consecutiveHitsToAbort',
    options.consecutiveHitsToAbort ?? DEFAULT_CONSECUTIVE_HITS_TO_ABORT,
  )
  const minNewGramsPerChunk = validatePositiveInt(
    'minNewGramsPerChunk',
    options.minNewGramsPerChunk ?? DEFAULT_MIN_NEW_GRAMS_PER_CHUNK,
  )
  const sential = createSential(options)
  let streak = 0

  return {
    inspect(text: string): TextLoopGuardInspectResult {
      const result = sential.inspect(text)
      if (result.newGrams >= minNewGramsPerChunk) {
        if (result.hit) {
          streak += 1
        } else {
          streak = 0
        }
      }
      return {
        ...result,
        streak,
        abort: streak >= consecutiveHitsToAbort,
      }
    },
    reset(): void {
      sential.reset()
      streak = 0
    },
  }
}

export function createTextLoopProbeBuffer(
  chunkSize: number,
  inspect: (text: string) => void,
): TextLoopProbeBuffer {
  validatePositiveInt('chunkSize', chunkSize)
  let chars: string[] = []
  let offset = 0

  const compact = () => {
    if (offset > 0) {
      chars = chars.slice(offset)
      offset = 0
    }
  }

  const inspectChunk = (text: string) => {
    if (text.length > 0) {
      inspect(text)
    }
  }

  return {
    push(text: string): void {
      if (!text) return
      chars.push(...normalizeChars(text))

      while (chars.length - offset >= chunkSize) {
        const chunk = chars.slice(offset, offset + chunkSize).join('')
        offset += chunkSize
        inspectChunk(chunk)
      }

      // Prevent unbounded front-gaps after many chunks.
      if (offset >= chunkSize) {
        compact()
      }
    },
    flush(): void {
      if (chars.length - offset > 0) {
        const remainder = chars.slice(offset).join('')
        inspectChunk(remainder)
      }
      chars = []
      offset = 0
    },
  }
}

export function createToolLoopGuard(
  options: ToolLoopGuardOptions = {},
): ToolLoopGuard {
  const repeatThreshold = validatePositiveInt(
    'repeatThreshold',
    options.repeatThreshold ?? DEFAULT_TOOL_LOOP_REPEAT_THRESHOLD,
  )
  const warningsBeforeAbort = validatePositiveInt(
    'warningsBeforeAbort',
    options.warningsBeforeAbort ?? DEFAULT_TOOL_LOOP_WARNINGS_BEFORE_ABORT,
  )
  const volatileKeySet = new Set(DEFAULT_VOLATILE_KEYS.map(normalizeKeyName))
  for (const key of options.volatileKeys ?? []) {
    const normalizedKey = normalizeKeyName(key)
    if (normalizedKey) {
      volatileKeySet.add(normalizedKey)
    }
  }

  let lastHash = ''
  let repeatCount = 0
  let breachCount = 0
  let breachHash = ''

  return {
    inspect(input: ToolLoopInspectInput): ToolLoopInspectResult {
      const hash = computeToolLoopHash(input, volatileKeySet)

      if (hash === lastHash) {
        repeatCount += 1
      } else {
        lastHash = hash
        repeatCount = 1
      }

      // Breach phase is fingerprint-specific: switching tool signature restarts it.
      if (breachHash !== hash) {
        breachHash = hash
        breachCount = 0
      }

      let warn = false
      let abort = false
      if (repeatCount > repeatThreshold) {
        if (breachCount < warningsBeforeAbort) {
          breachCount += 1
          warn = true
          // Reset consecutive accumulation after first warning.
          lastHash = ''
          repeatCount = 0
        } else {
          breachCount += 1
          abort = true
        }
      }

      return {
        hash,
        repeatCount,
        breachCount,
        warn,
        abort,
      }
    },
    reset(): void {
      lastHash = ''
      repeatCount = 0
      breachCount = 0
      breachHash = ''
    },
  }
}
