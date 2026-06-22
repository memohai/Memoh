import type { Ref } from 'vue'
import type { RouteLocationRaw, Router } from 'vue-router'

const NAVIGATION_PREFIXES = ['/bot', '/chat', '/settings'] as const

export function isRendererNavigationTarget(target: string): boolean {
  const path = target.split(/[?#]/, 1)[0] ?? ''
  return NAVIGATION_PREFIXES.some((prefix) => path === prefix || path.startsWith(`${prefix}/`))
}

export interface RendererNavigationRouter {
  currentRoute: Ref<{ fullPath: string }>
  push(target: RouteLocationRaw): ReturnType<Router['push']>
}

export function handleRendererNavigate(router: RendererNavigationRouter, target: string): boolean {
  if (!isRendererNavigationTarget(target)) return false
  if (router.currentRoute.value.fullPath === target) return false

  router.push(target).catch((error) => {
    console.warn('failed to navigate renderer route', error)
  })
  return true
}
