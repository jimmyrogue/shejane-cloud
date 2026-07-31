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
import { useLocation } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { LanguageSwitcher } from '@/components/language-switcher'
import { AnimatedOutlet } from '@/components/page-transition'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { SkipToMain } from '@/components/skip-to-main'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { LayoutProvider } from '@/context/layout-provider'
import { SearchProvider } from '@/context/search-provider'
import { getCookie } from '@/lib/cookies'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { AppHeader } from './app-header'
import { AppSidebar } from './app-sidebar'
import { SystemBrand } from './system-brand'
import { TopNav } from './top-nav'

type AuthenticatedLayoutProps = {
  children?: React.ReactNode
}

function UserConsoleHeader() {
  const { t } = useTranslation()
  const pathname = useLocation({ select: (location) => location.pathname })
  const links = useMemo(
    () => [
      {
        title: t('Dashboard'),
        href: '/dashboard/overview',
        isActive: pathname.startsWith('/dashboard'),
      },
      {
        title: t('SheJane devices'),
        href: '/devices',
        isActive: pathname.startsWith('/devices'),
      },
      {
        title: t('API Keys'),
        href: '/keys',
        isActive: pathname.startsWith('/keys'),
      },
      {
        title: t('Wallet'),
        href: '/wallet',
        isActive: pathname.startsWith('/wallet'),
      },
    ],
    [pathname, t]
  )

  return (
    <header className='border-border/80 bg-background/95 sticky top-0 z-40 h-14 shrink-0 border-b backdrop-blur'>
      <div className='mx-auto flex h-full w-full max-w-7xl items-center gap-3 px-3 sm:px-5'>
        <SystemBrand variant='inline' />
        <TopNav links={links} className='ms-3' />
        <div className='ms-auto flex items-center gap-1'>
          <LanguageSwitcher />
          <ProfileDropdown />
        </div>
      </div>
    </header>
  )
}

function UserConsoleLayout(props: AuthenticatedLayoutProps) {
  return (
    <div className='shejane-user-surface bg-background text-foreground flex h-svh min-h-0 flex-col overflow-hidden'>
      <SkipToMain />
      <UserConsoleHeader />
      <div
        id='content'
        className='@container/content flex min-h-0 flex-1 overflow-hidden'
      >
        <div className='mx-auto flex min-h-0 w-full max-w-7xl flex-1'>
          {props.children ?? <AnimatedOutlet />}
        </div>
      </div>
    </div>
  )
}

export function AuthenticatedLayout(props: AuthenticatedLayoutProps) {
  const role = useAuthStore((state) => state.auth.user?.role)
  const defaultOpen = getCookie('sidebar_state') !== 'false'

  if ((role ?? 0) < ROLE.ADMIN) {
    return <UserConsoleLayout>{props.children}</UserConsoleLayout>
  }

  return (
    <LayoutProvider>
      <SearchProvider>
        <SidebarProvider defaultOpen={defaultOpen} className='flex-col'>
          <SkipToMain />
          <AppHeader />
          <div className='flex min-h-0 w-full flex-1'>
            <AppSidebar />
            <SidebarInset
              className={cn(
                '@container/content',
                'h-[calc(100svh-var(--app-header-height,0px))]',
                'min-h-0 overflow-hidden',
                'peer-data-[variant=inset]:h-[calc(100svh-var(--app-header-height,0px)-(var(--spacing)*4))]'
              )}
            >
              {props.children ?? <AnimatedOutlet />}
            </SidebarInset>
          </div>
        </SidebarProvider>
      </SearchProvider>
    </LayoutProvider>
  )
}
