declare module '@novnc/novnc' {
  export default class RFB {
    viewOnly: boolean
    scaleViewport: boolean
    resizeSession: boolean
    background: string
    constructor(target: HTMLElement, url: string, options?: Record<string, unknown>)
    disconnect(): void
    addEventListener(type: string, listener: (event: Event) => void): void
    removeEventListener(type: string, listener: (event: Event) => void): void
  }
}
