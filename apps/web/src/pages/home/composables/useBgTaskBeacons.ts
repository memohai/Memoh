import type { ComputedRef, InjectionKey } from 'vue'
import { computed, inject, provide, reactive } from 'vue'
import type { BgTaskBeacon, BgTaskPill } from '@/store/chat-list.utils'
import { computeBgTaskPill } from '@/store/chat-list.utils'

interface BeaconRecord extends BgTaskBeacon {
  scrollIntoView: () => void
}

export interface BgTaskBeaconApi {
  upsert: (record: BeaconRecord) => void
  remove: (taskId: string) => void
}

const KEY: InjectionKey<BgTaskBeaconApi> = Symbol('bg-task-beacon')

// How long a completed-while-offscreen task keeps showing its "done" pill.
const DONE_LINGER_MS = 4000

export function provideBgTaskBeacons(): {
  pill: ComputedRef<BgTaskPill | null>
  scrollToOffscreen: () => void
} {
  const records = reactive(new Map<string, BeaconRecord>())
  const doneTimers = new Map<string, ReturnType<typeof setTimeout>>()

  const clearTimer = (taskId: string) => {
    const timer = doneTimers.get(taskId)
    if (timer !== undefined) {
      clearTimeout(timer)
      doneTimers.delete(taskId)
    }
  }

  const api: BgTaskBeaconApi = {
    upsert(record) {
      records.set(record.taskId, record)
      if (record.phase === 'done') {
        if (!doneTimers.has(record.taskId)) {
          doneTimers.set(record.taskId, setTimeout(() => {
            records.delete(record.taskId)
            doneTimers.delete(record.taskId)
          }, DONE_LINGER_MS))
        }
      } else {
        clearTimer(record.taskId)
      }
    },
    remove(taskId) {
      clearTimer(taskId)
      records.delete(taskId)
    },
  }

  provide(KEY, api)

  const pill = computed<BgTaskPill | null>(() => computeBgTaskPill([...records.values()]))

  const scrollToOffscreen = () => {
    const all = [...records.values()]
    const target = all.find(record => !record.visible && record.phase === 'active')
      ?? all.find(record => !record.visible)
    target?.scrollIntoView()
  }

  return { pill, scrollToOffscreen }
}

export function useBgTaskBeacon(): BgTaskBeaconApi | null {
  return inject(KEY, null)
}
