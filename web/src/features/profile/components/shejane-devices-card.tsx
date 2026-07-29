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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { MonitorSmartphone } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import {
  getSheJaneDevices,
  revokeSheJaneDevice,
} from '@/features/shejane-authorization/api'
import type { SheJaneDevice } from '@/features/shejane-authorization/types'
import { formatTimestamp } from '@/lib/format'

const sheJaneDevicesQueryKey = ['profile', 'shejane-devices'] as const

export function SheJaneDevicesCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [revokeTarget, setRevokeTarget] = useState<SheJaneDevice | null>(null)

  const devicesQuery = useQuery({
    queryKey: sheJaneDevicesQueryKey,
    queryFn: async () => {
      const response = await getSheJaneDevices()
      if (!response.success) throw new Error('load')
      return response.data ?? []
    },
    retry: false,
  })

  const revokeMutation = useMutation({
    mutationFn: async (id: number) => {
      const response = await revokeSheJaneDevice(id)
      if (!response.success || !response.data?.revoked) {
        throw new Error(response.code || 'revoke')
      }
    },
    onSuccess: async () => {
      setRevokeTarget(null)
      toast.success(t('Device access revoked'))
      await queryClient.invalidateQueries({ queryKey: sheJaneDevicesQueryKey })
    },
    onError: () => toast.error(t('Failed to revoke device access')),
  })

  const devices = devicesQuery.data ?? []
  let content: ReactNode
  if (devicesQuery.isLoading) {
    content = (
      <div
        className='space-y-3'
        role='status'
        aria-label={t('Loading devices')}
      >
        <Skeleton className='h-20 w-full' />
        <Skeleton className='h-20 w-full' />
      </div>
    )
  } else if (devicesQuery.isError) {
    content = (
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <MonitorSmartphone aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('Unable to load SheJane devices')}</EmptyTitle>
          <EmptyDescription>
            {t('Refresh the list and try again.')}
          </EmptyDescription>
        </EmptyHeader>
        <Button
          type='button'
          variant='outline'
          onClick={() => devicesQuery.refetch()}
        >
          {t('Retry')}
        </Button>
      </Empty>
    )
  } else if (devices.length === 0) {
    content = (
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <MonitorSmartphone aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('No SheJane devices')}</EmptyTitle>
          <EmptyDescription>
            {t('Devices appear here after you authorize SheJane Desktop.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else {
    content = (
      <div className='flex flex-col'>
        {devices.map((device, index) => {
          const revoked = device.revoked_at > 0
          return (
            <div key={device.id}>
              {index > 0 && <Separator />}
              <div className='flex flex-col gap-3 py-4 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between'>
                <div className='min-w-0 space-y-1'>
                  <div className='flex flex-wrap items-center gap-2'>
                    <p className='truncate font-medium'>{device.name}</p>
                    <Badge variant={revoked ? 'secondary' : 'outline'}>
                      {revoked ? t('Revoked') : t('Active')}
                    </Badge>
                  </div>
                  <p className='text-muted-foreground text-sm'>
                    {device.platform} ·{' '}
                    {t('Version {{version}}', { version: device.app_version })}
                  </p>
                  <p className='text-muted-foreground text-xs'>
                    {t('Authorized {{time}}', {
                      time: formatTimestamp(device.created_at),
                    })}
                  </p>
                </div>
                {!revoked && (
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => setRevokeTarget(device)}
                  >
                    {t('Revoke')}
                  </Button>
                )}
              </div>
            </div>
          )
        })}
      </div>
    )
  }

  return (
    <>
      <Card data-card-hover='false'>
        <CardHeader>
          <CardTitle>{t('SheJane devices')}</CardTitle>
          <CardDescription>
            {t('Review and revoke devices connected to SheJane Cloud.')}
          </CardDescription>
        </CardHeader>
        <CardContent>{content}</CardContent>
      </Card>

      <ConfirmDialog
        open={Boolean(revokeTarget)}
        onOpenChange={(open) => !open && setRevokeTarget(null)}
        title={t('Revoke SheJane device?')}
        desc={t(
          'This device will immediately lose access and must be authorized again.'
        )}
        confirmText={t('Revoke')}
        destructive
        isLoading={revokeMutation.isPending}
        handleConfirm={() => {
          if (revokeTarget) revokeMutation.mutate(revokeTarget.id)
        }}
      />
    </>
  )
}
