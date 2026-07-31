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
import { createFileRoute, redirect } from '@tanstack/react-router'

import { AuthenticatedLayout } from '@/components/layout'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const USER_CONSOLE_PREFIXES = ['/devices', '/keys', '/profile', '/wallet']

export function isUserConsolePath(pathname: string): boolean {
  if (pathname === '/dashboard' || pathname === '/dashboard/overview') {
    return true
  }

  return USER_CONSOLE_PREFIXES.some(
    (prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`)
  )
}

export const Route = createFileRoute('/_authenticated')({
  beforeLoad: ({ location }) => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || !auth.accessToken) {
      throw redirect({
        to: '/sign-in',
        search: { redirect: location.href },
      })
    }

    if (auth.user.role < ROLE.ADMIN && !isUserConsolePath(location.pathname)) {
      throw redirect({
        to: '/dashboard/$section',
        params: { section: 'overview' },
        replace: true,
      })
    }
  },
  component: AuthenticatedLayout,
})
