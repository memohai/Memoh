// 点阵背景运动数据:归一化坐标 [0,1]×[0,1],半径相对 min(vw,vh),20s 循环,6 形状。

export const STAGE_FR = 30
export const STAGE_OP = 600
export const STAGE_DUR = STAGE_OP / STAGE_FR

export type ShapeKind = 'circle' | 'ring' | 'rect' | 'star' | 'tri' | 'circle2'

export interface KeyframeTrack {
  static?: number | [number, number, number]
  frames?: Array<{ t: number; v: number[] | number }>
}

export interface ShapeDef {
  id: string
  kind: ShapeKind
  /** 半径 = ratio × min(viewW, viewH) */
  radius: number
  scale: number
  opacity: number
  pos: KeyframeTrack
  rot: KeyframeTrack
  localRot: number
  aspect?: number
  /** 环宽 = ratio × min(viewW, viewH) */
  strokeW?: number
  gradAngle: number
}

export interface ThemeDef {
  theme: 'light' | 'dark'
  bgOpacity: number
  shapes: ShapeDef[]
}

const kf = (frames: Array<[number, number, number]>): KeyframeTrack => ({
  frames: frames.map(([t, x, y]) => ({ t, v: [x, y, 0] })),
})

/** 整圈自转,首尾闭合 */
const spin = (turns = 1, t1 = STAGE_OP): KeyframeTrack => ({
  frames: [{ t: 0, v: [0] }, { t: t1, v: [360 * turns] }],
})

/** 中心轨迹保持在 ~0.15–0.85,避免贴边裁切 */
const SHAPES: ShapeDef[] = [
  {
    id: 'a',
    kind: 'circle',
    radius: 0.071,
    scale: 1.08,
    opacity: 1,
    gradAngle: 0.6,
    localRot: 0,
    rot: { static: 0 },
    pos: kf([
      [0, 0.78, 0.28],
      [150, 0.66, 0.20],
      [300, 0.38, 0.26],
      [450, 0.22, 0.44],
      [480, 0.32, 0.74],
      [600, 0.78, 0.28],
    ]),
  },
  {
    id: 'b',
    kind: 'ring',
    radius: 0.082,
    scale: 1,
    opacity: 1,
    strokeW: 0.024,
    gradAngle: 2.1,
    localRot: 0,
    rot: { static: 0 },
    pos: kf([
      [0, 0.18, 0.55],
      [120, 0.36, 0.32],
      [280, 0.58, 0.50],
      [420, 0.74, 0.72],
      [480, 0.50, 0.84],
      [600, 0.18, 0.55],
    ]),
  },
  {
    id: 'c',
    kind: 'rect',
    radius: 0.060,
    scale: 1.05,
    aspect: 1.62,
    opacity: 1,
    gradAngle: -0.8,
    localRot: -28,
    rot: spin(1),
    pos: kf([
      [0, 0.28, 0.84],
      [160, 0.42, 0.58],
      [320, 0.62, 0.38],
      [480, 0.76, 0.62],
      [540, 0.58, 0.80],
      [600, 0.28, 0.84],
    ]),
  },
  {
    id: 'd',
    kind: 'star',
    radius: 0.068,
    scale: 1,
    opacity: 1,
    gradAngle: 1.4,
    localRot: 18,
    rot: spin(-1),
    pos: kf([
      [0, 0.32, 0.24],
      [140, 0.20, 0.46],
      [300, 0.26, 0.74],
      [460, 0.40, 0.66],
      [540, 0.30, 0.28],
      [600, 0.32, 0.24],
    ]),
  },
  {
    id: 'e',
    kind: 'tri',
    radius: 0.075,
    scale: 1,
    opacity: 1,
    gradAngle: -1.2,
    localRot: -12,
    rot: spin(1),
    pos: kf([
      [0, 0.72, 0.38],
      [150, 0.78, 0.64],
      [320, 0.68, 0.80],
      [480, 0.56, 0.68],
      [540, 0.64, 0.36],
      [600, 0.72, 0.38],
    ]),
  },
  {
    id: 'f',
    kind: 'circle2',
    radius: 0.053,
    scale: 1.12,
    opacity: 1,
    gradAngle: 0.2,
    localRot: 0,
    rot: { static: 0 },
    pos: kf([
      [0, 0.50, 0.50],
      [120, 0.62, 0.40],
      [260, 0.58, 0.66],
      [400, 0.42, 0.62],
      [520, 0.36, 0.42],
      [540, 0.48, 0.52],
      [600, 0.50, 0.50],
    ]),
  },
]

export const THEMES: Record<'light' | 'dark', ThemeDef> = {
  light: { theme: 'light', bgOpacity: 0.1, shapes: SHAPES },
  dark: {
    theme: 'dark',
    bgOpacity: 0.12,
    shapes: SHAPES.map(s => ({ ...s, opacity: 0.92 })),
  },
}
