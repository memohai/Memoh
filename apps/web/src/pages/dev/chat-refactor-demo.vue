<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useTemplateRef, watch } from 'vue'
import { useChatRefactorDemo } from './use-chat-refactor-demo'
import DemoTurn from './demo-turn.vue'

const demo = useChatRefactorDemo()
const { mode, messages, stats, orderedSessions, sidebarStats } = demo

const life = ref({ mounts: 0, unmounts: 0 })
function onLife(kind: 'mount' | 'unmount') {
  if (kind === 'mount') life.value.mounts += 1
  else life.value.unmounts += 1
}

const playing = ref(false)
const speed = ref(140)
let timer: ReturnType<typeof setInterval> | null = null
const scrollEl = useTemplateRef<HTMLElement>('scrollEl')

function scrollToBottom() {
  const el = scrollEl.value
  if (el) el.scrollTop = el.scrollHeight
}

function step() {
  demo.tick()
  void nextTick(scrollToBottom)
}

function play() {
  if (playing.value) return
  playing.value = true
  timer = setInterval(step, speed.value)
}

function pause() {
  playing.value = false
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

function restart() {
  pause()
  demo.reset()
  life.value = { mounts: 0, unmounts: 0 }
}

function setMode(next: 'new' | 'old') {
  if (mode.value === next) return
  mode.value = next
  restart()
}

watch(speed, () => {
  if (playing.value) {
    pause()
    play()
  }
})

onBeforeUnmount(pause)
</script>

<template>
  <div class="demo-root">
    <header class="demo-header">
      <div>
        <h1>Chat stream refactor — live demo</h1>
        <p class="sub">
          Same mock agent run, two code paths. <strong>OLD</strong> = pre-refactor (wholesale replace + churn);
          <strong>NEW</strong> = this branch (in-place reconcile + identity-preserving). Watch the
          background-task popup and the remount counter.
        </p>
      </div>
      <div class="mode-toggle">
        <button
          :class="{ active: mode === 'old' }"
          @click="setMode('old')"
        >
          OLD
        </button>
        <button
          :class="{ active: mode === 'new' }"
          @click="setMode('new')"
        >
          NEW
        </button>
      </div>
    </header>

    <div class="controls">
      <button
        class="primary"
        @click="playing ? pause() : play()"
      >
        {{ playing ? 'Pause' : 'Play' }}
      </button>
      <button @click="step">
        Step
      </button>
      <button @click="restart">
        Reset
      </button>
      <label>speed <input
        v-model.number="speed"
        type="range"
        min="40"
        max="400"
        step="20"
      >{{ speed }}ms</label>
    </div>

    <div class="hud">
      <span>mode: <b :class="mode">{{ mode.toUpperCase() }}</b></span>
      <span>cycles: {{ stats.cyclesCompleted }}</span>
      <span>refetches: {{ stats.refetches }}</span>
      <span :class="{ bad: life.unmounts > 0, good: life.unmounts === 0 }">turn remounts: {{ life.unmounts }}</span>
      <span :class="{ bad: stats.blockChurn > 0, good: stats.blockChurn === 0 }">block churn: {{ stats.blockChurn }}</span>
    </div>
    <p class="hint">
      Red flash on a turn = it remounted (its markdown / background-task popup was thrown away and rebuilt).
      In NEW the just-sent turn flashes once and never again; in OLD it re-flashes on every refetch.
    </p>

    <div class="grid">
      <section class="chat">
        <div
          ref="scrollEl"
          class="scroll"
        >
          <DemoTurn
            v-for="turn in messages"
            :key="turn.id"
            :turn="turn"
            @life="onLife"
          />
          <p
            v-if="!messages.length"
            class="empty"
          >
            Press Play to start the mock stream.
          </p>
        </div>
      </section>

      <aside class="sidebar">
        <div class="sidebar-head">
          <span>Sessions</span>
          <button @click="demo.fireBackgroundEvent()">
            Fire bg event on “nightly build”
          </button>
        </div>
        <p class="hint sm">
          OLD restamps with the client clock and unshifts (the list jumps); NEW stamps the server time and
          re-sorts the view. reorders (array thrash): <b :class="{ bad: sidebarStats.reorders > 0 }">{{ sidebarStats.reorders }}</b>
        </p>
        <ul>
          <li
            v-for="session in orderedSessions"
            :key="session.id"
          >
            <span class="title">{{ session.title }}</span>
            <span class="time">{{ session.updated_at }}</span>
          </li>
        </ul>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.demo-root { max-width: 1100px; margin: 0 auto; padding: 20px; font-size: 14px; }
.demo-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
h1 { font-size: 18px; font-weight: 700; }
.sub { opacity: 0.7; font-size: 12.5px; max-width: 640px; margin-top: 4px; }
.mode-toggle { display: flex; border: 1px solid rgba(127, 127, 127, 0.35); border-radius: 8px; overflow: hidden; }
.mode-toggle button { padding: 6px 16px; font-weight: 600; }
.mode-toggle button.active { background: #6366f1; color: #fff; }
.controls { display: flex; align-items: center; gap: 10px; margin: 14px 0 8px; flex-wrap: wrap; }
.controls button, .sidebar-head button { border: 1px solid rgba(127, 127, 127, 0.35); border-radius: 8px; padding: 5px 12px; }
.controls button.primary { background: #6366f1; color: #fff; border-color: #6366f1; }
.controls label { display: flex; align-items: center; gap: 6px; opacity: 0.75; font-size: 12px; }
.hud { display: flex; gap: 16px; flex-wrap: wrap; padding: 8px 12px; border-radius: 8px; background: rgba(127, 127, 127, 0.08); font-family: ui-monospace, monospace; font-size: 12.5px; }
.hud b.new, .good b, .good { color: #16a34a; }
.hud b.old { color: #ef4444; }
.bad { color: #ef4444; font-weight: 700; }
.good { color: #16a34a; }
.hint { font-size: 12px; opacity: 0.65; margin: 8px 0; }
.hint.sm { font-size: 11.5px; }
.grid { display: grid; grid-template-columns: 1fr 280px; gap: 16px; margin-top: 8px; }
.scroll { height: 460px; overflow-y: auto; border: 1px solid rgba(127, 127, 127, 0.2); border-radius: 12px; padding: 12px; }
.empty { opacity: 0.5; text-align: center; margin-top: 40px; }
.sidebar { border: 1px solid rgba(127, 127, 127, 0.2); border-radius: 12px; padding: 12px; }
.sidebar-head { display: flex; flex-direction: column; gap: 8px; font-weight: 600; }
.sidebar ul { margin-top: 10px; display: flex; flex-direction: column; gap: 6px; }
.sidebar li { display: flex; flex-direction: column; padding: 8px 10px; border-radius: 8px; background: rgba(127, 127, 127, 0.07); transition: background 0.25s; }
.sidebar .title { font-size: 12.5px; }
.sidebar .time { font-size: 10.5px; opacity: 0.55; font-family: ui-monospace, monospace; }
</style>
