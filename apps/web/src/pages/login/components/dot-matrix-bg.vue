<template>
  <canvas
    ref="canvasEl"
    class="size-full text-foreground"
    aria-hidden="true"
  />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import {
  STAGE_DUR,
  STAGE_FR,
  STAGE_OP,
  THEMES,
  type KeyframeTrack,
  type ShapeDef,
} from './dot-matrix-keyframes'

/** 点阵背景:归一化轨迹 × 当前视口,canvas 矢量 + 渐变,再采样成点阵。 */

const GAP = 14
const DOT_RADIUS = 1
const LEVELS = 4
const MOTION_RATE = 0.55
/** 归一化坐标映射到视口时的内边距(含形状半径余量) */
const VIEW_INSET = 0.08
/** 离屏画布长边上限,避免 4K 全屏每帧 getImageData 过重 */
const MAX_FIELD_DIM = 1920

const AMBIENT_LIGHT = 0.08
const GAIN_LIGHT = 0.32
const AMBIENT_DARK = 0.12
const GAIN_DARK = 0.72

const ACCENT_VARS = [
  '--accent-blue', '--accent-purple', '--accent-teal',
  '--accent-green', '--accent-orange', '--accent-pink', '--accent-red',
]

const canvasEl = ref<HTMLCanvasElement | null>(null)

let raf = 0
let ctx: CanvasRenderingContext2D | null = null
let field: HTMLCanvasElement | null = null
let fieldCtx: CanvasRenderingContext2D | null = null
let cols = 0
let rows = 0
let fieldW = 0
let fieldH = 0
let fieldScale = 1
let viewW = 0
let viewH = 0
let dpr = 1
let dotColor = 'currentColor'
let isDark = false
let paletteRGB: string[] = []
let lastNow = 0
let reduceMotion = false
let animTime = 0
let pageVisible = true

function minDim(w: number, h: number) {
  return Math.min(w, h)
}

/** 归一化 [0,1] → 视图像素,始终铺满当前窗口(含超宽/竖屏) */
function normToPx(nx: number, ny: number, w: number, h: number): [number, number] {
  const span = 1 - 2 * VIEW_INSET
  return [
    (VIEW_INSET + nx * span) * w,
    (VIEW_INSET + ny * span) * h,
  ]
}

function sampleTrack(track: KeyframeTrack, frame: number, comp = 0): number {
  if (track.static !== undefined) {
    const v = track.static
    return typeof v === 'number' ? v : (v[comp] ?? 0)
  }
  const frames = track.frames!
  if (!frames.length) return 0
  let i = 0
  while (i < frames.length - 1 && frames[i + 1].t <= frame) i++
  const a = frames[i]
  const b = frames[Math.min(i + 1, frames.length - 1)]
  if (a.t === b.t) {
    const v = a.v
    return typeof v === 'number' ? v : (v[comp] ?? 0)
  }
  const t = (frame - a.t) / (b.t - a.t)
  const va = a.v
  const vb = b.v
  const na = typeof va === 'number' ? va : (va[comp] ?? 0)
  const nb = typeof vb === 'number' ? vb : (vb[comp] ?? 0)
  return na + (nb - na) * t
}

function toRGB(color: string): string {
  if (!fieldCtx) return '255,255,255'
  fieldCtx.fillStyle = color
  fieldCtx.fillRect(0, 0, 1, 1)
  const d = fieldCtx.getImageData(0, 0, 1, 1).data
  return `${d[0]},${d[1]},${d[2]}`
}

function readColor() {
  if (!canvasEl.value) return
  const cs = getComputedStyle(canvasEl.value)
  dotColor = cs.color || 'currentColor'
  isDark = document.documentElement.classList.contains('dark')
  paletteRGB = ACCENT_VARS
    .map(v => cs.getPropertyValue(v).trim())
    .filter(Boolean)
    .map(toRGB)
}

function resize() {
  const el = canvasEl.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  viewW = Math.max(1, rect.width)
  viewH = Math.max(1, rect.height)
  dpr = Math.min(window.devicePixelRatio || 1, 2)
  el.width = Math.max(1, Math.round(viewW * dpr))
  el.height = Math.max(1, Math.round(viewH * dpr))

  const longEdge = Math.max(viewW, viewH)
  fieldScale = longEdge > MAX_FIELD_DIM ? MAX_FIELD_DIM / longEdge : 1
  fieldW = Math.max(1, Math.round(viewW * fieldScale))
  fieldH = Math.max(1, Math.round(viewH * fieldScale))

  cols = Math.max(1, Math.ceil(viewW / GAP) + 1)
  rows = Math.max(1, Math.ceil(viewH / GAP) + 1)

  if (!field) field = document.createElement('canvas')
  field.width = fieldW
  field.height = fieldH
  fieldCtx = field.getContext('2d', { willReadFrequently: true })
  readColor()
}

function shapeFillGradient(s: ShapeDef, rgb: string, alpha: number, unit: number) {
  if (!fieldCtx) return '#fff'
  const r = s.radius * s.scale * unit
  const ang = s.gradAngle
  const dx = Math.cos(ang) * r
  const dy = Math.sin(ang) * r
  const hi = Math.min(1, alpha)
  const lo = Math.min(1, alpha * 0.4)
  const grad = fieldCtx.createLinearGradient(-dx, -dy, dx, dy)
  grad.addColorStop(0, `rgba(${rgb},${hi})`)
  grad.addColorStop(1, `rgba(${rgb},${lo})`)
  return grad
}

function drawStar(r: number, points = 8) {
  if (!fieldCtx) return
  const inner = r * 0.42
  fieldCtx.beginPath()
  for (let i = 0; i < points * 2; i++) {
    const rad = (i * Math.PI) / points - Math.PI / 2
    const dist = i % 2 === 0 ? r : inner
    const x = Math.cos(rad) * dist
    const y = Math.sin(rad) * dist
    if (i === 0) fieldCtx.moveTo(x, y)
    else fieldCtx.lineTo(x, y)
  }
  fieldCtx.closePath()
}

function drawTri(r: number) {
  if (!fieldCtx) return
  fieldCtx.beginPath()
  fieldCtx.moveTo(0, -r)
  fieldCtx.lineTo(r * 0.92, r * 0.78)
  fieldCtx.lineTo(-r * 0.92, r * 0.78)
  fieldCtx.closePath()
}

function drawShape(
  s: ShapeDef,
  frame: number,
  vw: number,
  vh: number,
  rgb: string,
) {
  if (!fieldCtx) return
  const unit = minDim(vw, vh)
  const nx = sampleTrack(s.pos, frame, 0)
  const ny = sampleTrack(s.pos, frame, 1)
  const rotDeg = sampleTrack(s.rot, frame, 0) + s.localRot
  const [cx, cy] = normToPx(nx, ny, vw, vh)
  const r = s.radius * s.scale * unit
  const alpha = isDark ? Math.max(s.opacity, 0.88) : 1

  fieldCtx.save()
  fieldCtx.translate(cx, cy)
  fieldCtx.rotate(rotDeg * (Math.PI / 180))
  fieldCtx.fillStyle = shapeFillGradient(s, rgb, alpha, unit)
  fieldCtx.strokeStyle = shapeFillGradient(s, rgb, alpha, unit)

  switch (s.kind) {
    case 'circle':
    case 'circle2': {
      fieldCtx.beginPath()
      fieldCtx.arc(0, 0, r, 0, Math.PI * 2)
      fieldCtx.fill()
      break
    }
    case 'ring': {
      const sw = (s.strokeW ?? s.radius * 0.28) * unit
      fieldCtx.lineWidth = sw
      fieldCtx.beginPath()
      fieldCtx.arc(0, 0, r, 0, Math.PI * 2)
      fieldCtx.stroke()
      break
    }
    case 'rect': {
      const asp = s.aspect ?? 1.5
      const w = r * asp
      const h = r
      fieldCtx.fillRect(-w / 2, -h / 2, w, h)
      break
    }
    case 'star':
      drawStar(r)
      fieldCtx.fill()
      break
    case 'tri':
      drawTri(r)
      fieldCtx.fill()
      break
  }
  fieldCtx.restore()
}

function drawField(frame: number) {
  if (!fieldCtx || !field) return
  fieldCtx.clearRect(0, 0, fieldW, fieldH)
  const theme = isDark ? THEMES.dark : THEMES.light
  theme.shapes.forEach((s, i) => {
    const rgb = isDark
      ? (paletteRGB[i % paletteRGB.length] ?? '255,255,255')
      : toRGB(dotColor)
    drawShape(s, frame, fieldW, fieldH, rgb)
  })
}

function scheduleFrame() {
  cancelAnimationFrame(raf)
  if (!pageVisible || reduceMotion) return
  raf = requestAnimationFrame(render)
}

function render(now: number) {
  if (!ctx || !fieldCtx || !field || !canvasEl.value) return
  if (!lastNow) lastNow = now
  const dt = Math.min(0.05, (now - lastNow) / 1000)
  lastNow = now
  if (!reduceMotion) animTime = (animTime + dt * MOTION_RATE) % STAGE_DUR
  const frame = (animTime * STAGE_FR) % STAGE_OP

  drawField(frame)
  const data = fieldCtx.getImageData(0, 0, fieldW, fieldH).data
  const el = canvasEl.value
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, el.width, el.height)

  const theme = isDark ? THEMES.dark : THEMES.light
  const ambient = isDark ? AMBIENT_DARK : Math.max(AMBIENT_LIGHT, theme.bgOpacity * 0.65)
  const gain = isDark ? GAIN_DARK : GAIN_LIGHT
  const TWO_PI = Math.PI * 2

  ctx.fillStyle = dotColor
  ctx.globalAlpha = ambient
  for (let y = 0; y < rows; y++) {
    const py = y * GAP
    for (let x = 0; x < cols; x++) {
      ctx.beginPath()
      ctx.arc(x * GAP, py, DOT_RADIUS, 0, TWO_PI)
      ctx.fill()
    }
  }

  if (!isDark) ctx.fillStyle = dotColor
  for (let y = 0; y < rows; y++) {
    const py = y * GAP
    const fy = Math.min(fieldH - 1, Math.round(py * fieldScale))
    for (let x = 0; x < cols; x++) {
      const px = x * GAP
      const fx = Math.min(fieldW - 1, Math.round(px * fieldScale))
      const idx = (fy * fieldW + fx) * 4
      const raw = data[idx + 3] / 255
      if (raw <= 0.04) continue
      const cov = Math.round(raw * LEVELS) / LEVELS
      if (cov <= 0) continue
      if (isDark) ctx.fillStyle = `rgb(${data[idx]},${data[idx + 1]},${data[idx + 2]})`
      ctx.globalAlpha = cov * gain
      ctx.beginPath()
      ctx.arc(px, py, DOT_RADIUS, 0, TWO_PI)
      ctx.fill()
    }
  }
  ctx.globalAlpha = 1
  scheduleFrame()
}

let resizeObserver: ResizeObserver | null = null
let themeObserver: MutationObserver | null = null
let mediaQuery: MediaQueryList | null = null

function onReduceMotionChange() {
  reduceMotion = !!mediaQuery?.matches
  lastNow = 0
  if (reduceMotion) {
    cancelAnimationFrame(raf)
    raf = requestAnimationFrame(render)
    return
  }
  scheduleFrame()
}

function onVisibilityChange() {
  pageVisible = !document.hidden
  lastNow = 0
  if (!pageVisible) {
    cancelAnimationFrame(raf)
    return
  }
  if (!reduceMotion) scheduleFrame()
}

onMounted(() => {
  const el = canvasEl.value
  if (!el) return
  ctx = el.getContext('2d')
  mediaQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  reduceMotion = mediaQuery.matches
  mediaQuery.addEventListener('change', onReduceMotionChange)
  document.addEventListener('visibilitychange', onVisibilityChange)
  resize()
  resizeObserver = new ResizeObserver(() => resize())
  resizeObserver.observe(el)
  themeObserver = new MutationObserver(() => readColor())
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  raf = requestAnimationFrame(render)
})

onBeforeUnmount(() => {
  cancelAnimationFrame(raf)
  resizeObserver?.disconnect()
  themeObserver?.disconnect()
  mediaQuery?.removeEventListener('change', onReduceMotionChange)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>
