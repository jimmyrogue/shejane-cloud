/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { AuthUser } from '@/stores/auth-store'

const allowedRedirectProtocols = new Set(['http:', 'https:'])
const authContinuationKey = 'shejane.auth.continuation'
const authContinuationTTL = 10 * 60 * 1000

type AuthContinuation = {
  path: string
  expires_at: number
}

export function getSavedLanguage(user: AuthUser): string | undefined {
  if (typeof user.language === 'string') {
    return user.language
  }

  if (user.setting && typeof user.setting === 'object') {
    return typeof user.setting.language === 'string'
      ? user.setting.language
      : undefined
  }

  if (typeof user.setting !== 'string') {
    return undefined
  }

  try {
    const setting = JSON.parse(user.setting) as { language?: unknown }
    return typeof setting.language === 'string' ? setting.language : undefined
  } catch {
    return undefined
  }
}

export function sanitizeAuthRedirect(
  value: unknown,
  origin: string
): string | null {
  if (typeof value !== 'string') return null

  const target = value.trim()
  if (!target || target.includes('\\') || target.startsWith('//')) return null

  let trustedOrigin: URL
  try {
    trustedOrigin = new URL(origin)
  } catch {
    return null
  }
  if (!allowedRedirectProtocols.has(trustedOrigin.protocol)) return null

  let redirectURL: URL
  try {
    redirectURL = target.startsWith('/')
      ? new URL(target, trustedOrigin.origin)
      : new URL(target)
  } catch {
    return null
  }

  if (
    !allowedRedirectProtocols.has(redirectURL.protocol) ||
    redirectURL.origin !== trustedOrigin.origin
  ) {
    return null
  }

  return `${redirectURL.pathname}${redirectURL.search}${redirectURL.hash}`
}

export function saveAuthContinuation(
  value: unknown,
  origin: string,
  storage?: Storage,
  now = Date.now()
): boolean {
  const target = sanitizeAuthContinuation(value, origin)
  if (!target) return false

  const targetStorage = storage ?? getSessionStorage()
  if (!targetStorage) return false
  try {
    targetStorage.setItem(
      authContinuationKey,
      JSON.stringify({ path: target, expires_at: now + authContinuationTTL })
    )
    return true
  } catch {
    return false
  }
}

export function consumeAuthContinuation(
  origin: string,
  storage?: Storage,
  now = Date.now()
): string | null {
  const targetStorage = storage ?? getSessionStorage()
  if (!targetStorage) return null

  let raw: string | null
  try {
    raw = targetStorage.getItem(authContinuationKey)
    targetStorage.removeItem(authContinuationKey)
  } catch {
    return null
  }
  if (!raw) return null

  try {
    const saved = JSON.parse(raw) as Partial<AuthContinuation>
    if (
      typeof saved.path !== 'string' ||
      typeof saved.expires_at !== 'number' ||
      !Number.isFinite(saved.expires_at) ||
      saved.expires_at <= now
    ) {
      return null
    }
    return sanitizeAuthContinuation(saved.path, origin)
  } catch {
    return null
  }
}

export function resolveAuthRedirect(
  explicitTarget: unknown,
  origin: string,
  storage?: Storage,
  now = Date.now()
): string {
  return (
    sanitizeAuthRedirect(explicitTarget, origin) ??
    consumeAuthContinuation(origin, storage, now) ??
    '/dashboard'
  )
}

function getSessionStorage(): Storage | undefined {
  if (typeof window === 'undefined') return undefined
  try {
    return window.sessionStorage
  } catch {
    return undefined
  }
}

function sanitizeAuthContinuation(
  value: unknown,
  origin: string
): string | null {
  const target = sanitizeAuthRedirect(value, origin)
  if (!target) return null

  let targetURL: URL
  try {
    targetURL = new URL(target, origin)
  } catch {
    return null
  }
  const flowTokens = targetURL.searchParams.getAll('flow_token')
  if (
    targetURL.pathname !== '/shejane/authorize' ||
    targetURL.hash ||
    [...targetURL.searchParams.keys()].some((key) => key !== 'flow_token') ||
    flowTokens.length !== 1 ||
    !flowTokens[0]?.trim()
  ) {
    return null
  }
  return target
}
