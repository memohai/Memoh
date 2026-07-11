import { describe, expect, it } from 'vitest'
import { decidePostConnectNavigation } from './connection-navigation'

describe('post-connect navigation', () => {
  it('clears authentication and opens login after switching servers', () => {
    expect(decidePostConnectNavigation({
      changed: true,
      hasToken: true,
      returnTo: '/settings/about',
    })).toEqual({
      clearAuth: true,
      animateLogin: true,
      destination: { name: 'Login' },
    })
  })

  it('keeps authentication and returns to the requested page on the same server', () => {
    expect(decidePostConnectNavigation({
      changed: false,
      hasToken: true,
      returnTo: '/settings/about',
    })).toEqual({
      clearAuth: false,
      animateLogin: false,
      destination: '/settings/about',
    })
  })

  it('opens login without clearing state when the same server has no token', () => {
    expect(decidePostConnectNavigation({
      changed: false,
      hasToken: false,
      returnTo: '/settings/about',
    })).toEqual({
      clearAuth: false,
      animateLogin: true,
      destination: { name: 'Login' },
    })
  })

  it('falls back to home for an invalid return path', () => {
    expect(decidePostConnectNavigation({
      changed: false,
      hasToken: true,
      returnTo: '//example.com',
    }).destination).toBe('/')
  })
})
