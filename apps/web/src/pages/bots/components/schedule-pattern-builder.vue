<template>
  <div class="space-y-4">
    <div class="space-y-2">
      <Label>{{ $t('bots.schedule.form.mode') }}</Label>
      <NativeSelect
        v-model="modeModel"
        class="h-9 text-xs"
      >
        <option value="minutes">
          {{ $t('bots.schedule.mode.minutes') }}
        </option>
        <option value="hourly">
          {{ $t('bots.schedule.mode.hourly') }}
        </option>
        <option value="daily">
          {{ $t('bots.schedule.mode.daily') }}
        </option>
        <option value="weekly">
          {{ $t('bots.schedule.mode.weekly') }}
        </option>
        <option value="monthly">
          {{ $t('bots.schedule.mode.monthly') }}
        </option>
        <option value="yearly">
          {{ $t('bots.schedule.mode.yearly') }}
        </option>
        <option value="advanced">
          {{ $t('bots.schedule.mode.advanced') }}
        </option>
      </NativeSelect>
      <p class="text-xs text-muted-foreground">
        {{ modeHint }}
      </p>
    </div>

    <!-- minutes -->
    <div
      v-if="state.mode === 'minutes'"
      class="space-y-2"
    >
      <Label>{{ $t('bots.schedule.form.everyMinutes') }}</Label>
      <Input
        :model-value="state.intervalMinutes"
        type="number"
        :min="1"
        :max="59"
        @update:model-value="(v) => update({ intervalMinutes: clampInt(v, 1, 59, 1) })"
      />
    </div>

    <!-- hourly -->
    <div
      v-else-if="state.mode === 'hourly'"
      class="space-y-2"
    >
      <Label>{{ $t('bots.schedule.form.atMinute') }}</Label>
      <Input
        :model-value="state.minute"
        type="number"
        :min="0"
        :max="59"
        @update:model-value="(v) => update({ minute: clampInt(v, 0, 59, 0) })"
      />
    </div>

    <!-- daily -->
    <div
      v-else-if="state.mode === 'daily'"
      class="space-y-4"
    >
      <div class="space-y-3">
        <div class="space-y-1.5">
          <Label>{{ $t('bots.schedule.form.hours') }}</Label>
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.schedule.form.hoursHint') }}
          </p>
        </div>

        <!-- AM Row -->
        <div class="space-y-2">
          <div class="flex items-center gap-1.5 text-muted-foreground">
            <Sun class="size-3.5" />
            <h3 class="text-[10px] font-semibold uppercase tracking-wider">
              AM
            </h3>
          </div>
          <div class="grid grid-cols-6 sm:grid-cols-12 gap-1.5">
            <button
              v-for="h in 12"
              :key="h - 1"
              type="button"
              class="h-8 rounded-md border text-[11px] font-mono transition-colors"
              :class="state.hours.includes(h - 1)
                ? 'bg-primary text-primary-foreground border-primary'
                : 'bg-background hover:bg-accent'"
              @click="toggleHour(h - 1)"
            >
              {{ pad2(h - 1) }}
            </button>
          </div>
        </div>

        <!-- PM Row -->
        <div class="space-y-2">
          <div class="flex items-center gap-1.5 text-muted-foreground">
            <Moon class="size-3.5" />
            <h3 class="text-[10px] font-semibold uppercase tracking-wider">
              PM
            </h3>
          </div>
          <div class="grid grid-cols-6 sm:grid-cols-12 gap-1.5">
            <button
              v-for="h in 12"
              :key="h + 11"
              type="button"
              class="h-8 rounded-md border text-[11px] font-mono transition-colors"
              :class="state.hours.includes(h + 11)
                ? 'bg-primary text-primary-foreground border-primary'
                : 'bg-background hover:bg-accent'"
              @click="toggleHour(h + 11)"
            >
              {{ pad2(h + 11) }}
            </button>
          </div>
        </div>
      </div>
      <div class="space-y-2">
        <Label>{{ $t('bots.schedule.form.minute') }}</Label>
        <Input
          :model-value="state.minute"
          type="number"
          :min="0"
          :max="59"
          @update:model-value="(v) => update({ minute: clampInt(v, 0, 59, 0) })"
        />
      </div>
    </div>

    <!-- weekly -->
    <div
      v-else-if="state.mode === 'weekly'"
      class="space-y-3"
    >
      <div class="space-y-2">
        <Label>{{ $t('bots.schedule.form.weekdays') }}</Label>
        <div class="grid grid-cols-7 gap-1.5">
          <button
            v-for="(key, idx) in WEEKDAY_KEYS"
            :key="key"
            type="button"
            class="h-9 rounded-md border text-xs transition-colors"
            :class="state.weekdays.includes(idx)
              ? 'bg-primary text-primary-foreground border-primary'
              : 'bg-background hover:bg-accent'"
            @click="toggleWeekday(idx)"
          >
            {{ $t(`bots.schedule.weekday.${key}`) }}
          </button>
        </div>
      </div>
      <div class="grid grid-cols-2 gap-2">
        <div class="space-y-2">
          <Label>{{ $t('bots.schedule.form.hour') }}</Label>
          <Input
            :model-value="singleHour"
            type="number"
            :min="0"
            :max="23"
            @update:model-value="(v) => setSingleHour(clampInt(v, 0, 23, 0))"
          />
        </div>
        <div class="space-y-2">
          <Label>{{ $t('bots.schedule.form.minute') }}</Label>
          <Input
            :model-value="state.minute"
            type="number"
            :min="0"
            :max="59"
            @update:model-value="(v) => update({ minute: clampInt(v, 0, 59, 0) })"
          />
        </div>
      </div>
    </div>

    <!-- monthly -->
    <div
      v-else-if="state.mode === 'monthly'"
      class="space-y-3"
    >
      <div class="space-y-2">
        <Label>{{ $t('bots.schedule.form.monthDays') }}</Label>
        <div class="grid grid-cols-7 gap-1.5">
          <button
            v-for="d in 31"
            :key="d"
            type="button"
            class="h-8 rounded-md border text-xs font-mono transition-colors"
            :class="state.monthDays.includes(d)
              ? 'bg-primary text-primary-foreground border-primary'
              : 'bg-background hover:bg-accent'"
            @click="toggleMonthDay(d)"
          >
            {{ d }}
          </button>
        </div>
      </div>
      <div class="grid grid-cols-2 gap-2">
        <div class="space-y-2">
          <Label>{{ $t('bots.schedule.form.hour') }}</Label>
          <Input
            :model-value="singleHour"
            type="number"
            :min="0"
            :max="23"
            @update:model-value="(v) => setSingleHour(clampInt(v, 0, 23, 0))"
          />
        </div>
        <div class="space-y-2">
          <Label>{{ $t('bots.schedule.form.minute') }}</Label>
          <Input
            :model-value="state.minute"
            type="number"
            :min="0"
            :max="59"
            @update:model-value="(v) => update({ minute: clampInt(v, 0, 59, 0) })"
          />
        </div>
      </div>
    </div>

    <!-- yearly -->
    <div
      v-else-if="state.mode === 'yearly'"
      class="space-y-3"
    >
      <div class="grid grid-cols-2 gap-2">
        <div class="space-y-2">
          <Label>{{ $t('bots.schedule.form.month') }}</Label>
          <NativeSelect
            v-model="yearlyMonthModel"
            class="h-9 text-xs"
          >
            <option
              v-for="(key, idx) in MONTH_KEYS"
              :key="key"
              :value="String(idx + 1)"
            >
              {{ $t(`bots.schedule.month.${key}`) }}
            </option>
          </NativeSelect>
        </div>
        <div class="space-y-2">
          <Label>{{ $t('bots.schedule.form.monthDay') }}</Label>
          <Input
            :model-value="state.monthDay"
            type="number"
            :min="1"
            :max="31"
            @update:model-value="(v) => update({ monthDay: clampInt(v, 1, 31, 1) })"
          />
        </div>
      </div>
      <div class="grid grid-cols-2 gap-2">
        <div class="space-y-2">
          <Label>{{ $t('bots.schedule.form.hour') }}</Label>
          <Input
            :model-value="singleHour"
            type="number"
            :min="0"
            :max="23"
            @update:model-value="(v) => setSingleHour(clampInt(v, 0, 23, 0))"
          />
        </div>
        <div class="space-y-2">
          <Label>{{ $t('bots.schedule.form.minute') }}</Label>
          <Input
            :model-value="state.minute"
            type="number"
            :min="0"
            :max="59"
            @update:model-value="(v) => update({ minute: clampInt(v, 0, 59, 0) })"
          />
        </div>
      </div>
    </div>

    <!-- advanced -->
    <div
      v-else-if="state.mode === 'advanced'"
      class="space-y-2"
    >
      <Label>{{ $t('bots.schedule.form.advancedPattern') }}</Label>
      <p class="text-xs text-muted-foreground">
        {{ $t('bots.schedule.form.advancedHint') }}
      </p>
      <Input
        :model-value="state.advancedPattern"
        class="font-mono"
        :placeholder="'0 9 * * *'"
        @update:model-value="(v) => update({ advancedPattern: String(v) })"
      />
    </div>

    <!-- preview -->
    <div class="rounded-md border bg-muted/30 px-3 py-2 space-y-1.5">
      <div class="flex items-start justify-between gap-2">
        <div class="min-w-0 flex-1 space-y-1">
          <div class="text-xs text-muted-foreground">
            {{ $t('bots.schedule.form.patternPreview') }}
          </div>
          <code
            v-if="previewPattern"
            class="text-xs font-mono break-all"
          >{{ previewPattern }}</code>
          <code
            v-else
            class="text-xs text-muted-foreground"
          >—</code>
        </div>
      </div>
      <div class="text-xs">
        <span
          v-if="humanText"
          class="text-foreground"
        >{{ humanText }}</span>
        <span
          v-else
          class="text-destructive"
        >{{ $t('bots.schedule.form.invalidPattern') }}</span>
      </div>
      <div
        v-if="upcomingRuns.length"
        class="text-xs text-muted-foreground space-y-0.5 pt-1 border-t border-border/60"
      >
        <div>
          {{ $t('bots.schedule.form.nextRuns', { tz: effectiveTimezone }) }}
        </div>
        <div
          v-for="(d, i) in upcomingRuns"
          :key="i"
          class="font-mono"
        >
          · {{ formatPreviewDate(d) }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Sun, Moon } from 'lucide-vue-next'
import { Input, Label, NativeSelect } from '@memohai/ui'
import {
  describeCron,
  nextRuns,
  toCron,
  MONTH_KEYS,
  WEEKDAY_KEYS,
  type CronLocale,
  type ScheduleFormState,
  type ScheduleMode,
} from '@/utils/cron-pattern'

const props = defineProps<{
  state: ScheduleFormState
  timezone?: string
}>()

const emit = defineEmits<{
  'update:state': [value: ScheduleFormState]
}>()

const { locale, t } = useI18n()

const cronLocale = computed<CronLocale>(() => (locale.value.startsWith('zh') ? 'zh' : 'en'))

const effectiveTimezone = computed(() => {
  const tz = props.timezone?.trim()
  if (tz) return tz
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone
  } catch {
    return 'UTC'
  }
})

function update(patch: Partial<ScheduleFormState>) {
  emit('update:state', { ...props.state, ...patch })
}

// NativeSelect's v-model type allows any AcceptableValue, so we wrap writes
// with string coercion and validated casts before updating form state.
const modeModel = computed({
  get: (): string => props.state.mode,
  set: (val: unknown) => {
    const next = String(val)
    if (
      next === 'minutes' || next === 'hourly' || next === 'daily'
      || next === 'weekly' || next === 'monthly' || next === 'yearly'
      || next === 'advanced'
    ) {
      handleModeChange(next)
    }
  },
})

const yearlyMonthModel = computed({
  get: (): string => String(props.state.month),
  set: (val: unknown) => {
    const n = Number(val)
    if (Number.isInteger(n) && n >= 1 && n <= 12) {
      update({ month: n })
    }
  },
})

function clampInt(value: unknown, min: number, max: number, fallback: number): number {
  const n = Number(value)
  if (!Number.isFinite(n)) return fallback
  return Math.max(min, Math.min(max, Math.round(n)))
}

function pad2(n: number): string {
  return n.toString().padStart(2, '0')
}

const singleHour = computed(() => props.state.hours[0] ?? 0)

function setSingleHour(h: number) {
  update({ hours: [h] })
}

function toggleHour(h: number) {
  const set = new Set(props.state.hours)
  if (set.has(h)) set.delete(h)
  else set.add(h)
  const next = Array.from(set).sort((a, b) => a - b)
  update({ hours: next.length ? next : [h] })
}

function toggleWeekday(d: number) {
  const set = new Set(props.state.weekdays)
  if (set.has(d)) set.delete(d)
  else set.add(d)
  const next = Array.from(set).sort((a, b) => a - b)
  update({ weekdays: next.length ? next : [d] })
}

function toggleMonthDay(d: number) {
  const set = new Set(props.state.monthDays)
  if (set.has(d)) set.delete(d)
  else set.add(d)
  const next = Array.from(set).sort((a, b) => a - b)
  update({ monthDays: next.length ? next : [d] })
}

function handleModeChange(next: ScheduleMode) {
  const patch: Partial<ScheduleFormState> = { mode: next }
  // Normalize state when switching to modes that require a single hour, so the
  // builder stays internally consistent.
  if (next === 'weekly' || next === 'monthly' || next === 'yearly' || next === 'hourly') {
    patch.hours = [props.state.hours[0] ?? 9]
  }
  if (next === 'advanced' && !props.state.advancedPattern.trim()) {
    // Seed the advanced input with the currently-derived pattern so the user
    // can start from a known-good expression instead of a blank field.
    try {
      patch.advancedPattern = toCron(props.state)
    } catch {
      patch.advancedPattern = ''
    }
  }
  emit('update:state', { ...props.state, ...patch })
}

const previewPattern = computed(() => {
  try {
    const p = toCron(props.state)
    return p.trim()
  } catch {
    return ''
  }
})

const humanText = computed(() => {
  if (!previewPattern.value) return undefined
  return describeCron(previewPattern.value, cronLocale.value)
})

const upcomingRuns = computed(() => {
  if (!previewPattern.value) return []
  return nextRuns(previewPattern.value, effectiveTimezone.value, 3)
})

const modeHint = computed(() => t(`bots.schedule.modeHint.${props.state.mode}`))

const previewFormatter = computed(() => new Intl.DateTimeFormat(
  locale.value.startsWith('zh') ? 'zh-CN' : 'en-US',
  {
    timeZone: effectiveTimezone.value,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  },
))

function formatPreviewDate(d: Date): string {
  return previewFormatter.value.format(d)
}
</script>
