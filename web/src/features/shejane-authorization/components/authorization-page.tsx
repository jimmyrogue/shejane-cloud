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
import { useMutation, useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { AlertCircle, Check, Loader2, Monitor } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  consumeAuthContinuation,
  saveAuthContinuation,
} from '@/features/auth/lib/auth-redirect'
import { useAuthStore } from '@/stores/auth-store'

import {
  decideSheJaneAuthorization,
  getSheJaneErrorCode,
  getSheJaneAuthorization,
  startSheJaneAuthorization,
} from '../api'
import {
  parseSheJaneAuthorizationSearch,
  trustedSheJaneLoopbackRedirect,
} from '../lib/authorization'

type AuthorizationPageProps = {
  search: unknown
}

export function AuthorizationPage(props: AuthorizationPageProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const user = useAuthStore((state) => state.auth.user)
  const parsedSearch = useMemo(
    () => parseSheJaneAuthorizationSearch(props.search),
    [props.search]
  )
  const [flowToken, setFlowToken] = useState(
    parsedSearch.kind === 'resume' ? parsedSearch.flowToken : ''
  )
  const startAttempted = useRef(false)

  const startMutation = useMutation({
    mutationFn: startSheJaneAuthorization,
    onSuccess: (response) => {
      const nextFlowToken = response.success
        ? response.data?.flow_token?.trim()
        : ''
      if (!nextFlowToken) return
      const path = `/shejane/authorize?flow_token=${encodeURIComponent(nextFlowToken)}`
      window.history.replaceState(null, '', path)
      setFlowToken(nextFlowToken)
    },
  })

  useEffect(() => {
    if (parsedSearch.kind !== 'start' || startAttempted.current) return
    startAttempted.current = true
    startMutation.mutate(parsedSearch.request)
  }, [parsedSearch, startMutation])

  useEffect(() => {
    if (!flowToken) return
    const path = `/shejane/authorize?flow_token=${encodeURIComponent(flowToken)}`
    saveAuthContinuation(path, window.location.origin)
    if (!user) {
      void navigate({
        to: '/sign-in',
        search: { redirect: path },
        replace: true,
      })
    }
  }, [flowToken, navigate, user])

  const consentQuery = useQuery({
    queryKey: ['shejane', 'authorization', flowToken],
    queryFn: async () => {
      let response
      try {
        response = await getSheJaneAuthorization(flowToken)
      } catch (error) {
        throw new Error(getSheJaneErrorCode(error) || 'SHEJANE_INTERNAL_ERROR')
      }
      if (!response.success || !response.data) {
        throw new Error(response.code || 'SHEJANE_INTERNAL_ERROR')
      }
      return response.data
    },
    enabled: Boolean(flowToken && user),
    retry: false,
  })

  const decisionMutation = useMutation({
    mutationFn: (decision: 'approve' | 'deny') =>
      decideSheJaneAuthorization(flowToken, decision),
    onSuccess: (response) => {
      const redirect = trustedSheJaneLoopbackRedirect(
        response.success ? response.data?.redirect_to : undefined
      )
      if (!redirect) return
      consumeAuthContinuation(window.location.origin)
      window.location.replace(redirect)
    },
  })

  const consent = consentQuery.data
  const invalidStart = parsedSearch.kind === 'invalid'
  const startFailed =
    startMutation.isError || (startMutation.isSuccess && !flowToken)
  const decisionFailed =
    decisionMutation.isError ||
    (decisionMutation.isSuccess &&
      (!decisionMutation.data?.success ||
        !trustedSheJaneLoopbackRedirect(
          decisionMutation.data.data?.redirect_to
        )))
  const flowError =
    consentQuery.error instanceof Error ? consentQuery.error.message : ''

  useEffect(() => {
    if (
      flowError === 'SHEJANE_FLOW_EXPIRED' ||
      flowError === 'SHEJANE_FLOW_INVALID'
    ) {
      consumeAuthContinuation(window.location.origin)
    }
  }, [flowError])

  let content: React.ReactNode
  if (invalidStart || startFailed) {
    content = (
      <AuthorizationError
        title={t('Unable to start authorization')}
        description={t(
          'Return to SheJane Desktop and start the connection again.'
        )}
        onRetry={
          parsedSearch.kind === 'start'
            ? () => startMutation.mutate(parsedSearch.request)
            : undefined
        }
      />
    )
  } else if (!flowToken || !user || consentQuery.isLoading) {
    content = (
      <div
        className='flex items-center justify-center gap-2 py-12'
        role='status'
      >
        <Loader2 aria-hidden='true' className='h-5 w-5 animate-spin' />
        <span>{t('Preparing authorization…')}</span>
      </div>
    )
  } else if (consentQuery.isError) {
    const expired = flowError === 'SHEJANE_FLOW_EXPIRED'
    const invalid = flowError === 'SHEJANE_FLOW_INVALID'
    content = (
      <AuthorizationError
        title={
          expired
            ? t('Authorization request expired')
            : t('Authorization request unavailable')
        }
        description={t(
          'Return to SheJane Desktop and start the connection again.'
        )}
        onRetry={
          !expired && !invalid ? () => consentQuery.refetch() : undefined
        }
      />
    )
  } else if (consent) {
    content = (
      <Card className='w-full' data-card-hover='false'>
        <CardHeader>
          <CardTitle>
            {t('Authorize {{app}}', { app: consent.client.name })}
          </CardTitle>
          <CardDescription>
            {t('Allow this device to use your Cloud account for AI inference.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-5'>
          <dl className='grid gap-3 rounded-lg border p-4 text-sm'>
            <div className='grid gap-1 sm:grid-cols-[8rem_1fr]'>
              <dt className='text-muted-foreground'>{t('Application')}</dt>
              <dd className='font-medium'>{consent.client.name}</dd>
            </div>
            <div className='grid gap-1 sm:grid-cols-[8rem_1fr]'>
              <dt className='text-muted-foreground'>{t('Device')}</dt>
              <dd className='font-medium'>{consent.device.name}</dd>
            </div>
            <div className='grid gap-1 sm:grid-cols-[8rem_1fr]'>
              <dt className='text-muted-foreground'>{t('Platform')}</dt>
              <dd>{consent.device.platform}</dd>
            </div>
            <div className='grid gap-1 sm:grid-cols-[8rem_1fr]'>
              <dt className='text-muted-foreground'>{t('App version')}</dt>
              <dd>{consent.device.app_version}</dd>
            </div>
          </dl>

          <Alert>
            <Monitor aria-hidden='true' />
            <AlertTitle>{t('Inference access only')}</AlertTitle>
            <AlertDescription>
              {t(
                'SheJane Desktop can use models available to your account. It cannot manage your Cloud account.'
              )}
            </AlertDescription>
          </Alert>

          {decisionFailed && (
            <Alert variant='destructive' aria-live='polite'>
              <AlertCircle aria-hidden='true' />
              <AlertTitle>
                {t('Authorization could not be completed')}
              </AlertTitle>
              <AlertDescription>{t('Please try again.')}</AlertDescription>
            </Alert>
          )}

          <div className='flex flex-col-reverse gap-2 sm:flex-row sm:justify-end'>
            <Button
              type='button'
              variant='outline'
              disabled={decisionMutation.isPending}
              onClick={() => decisionMutation.mutate('deny')}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              disabled={decisionMutation.isPending}
              onClick={() => decisionMutation.mutate('approve')}
            >
              {decisionMutation.isPending ? (
                <Loader2 aria-hidden='true' className='animate-spin' />
              ) : (
                <Check aria-hidden='true' />
              )}
              {t('Authorize')}
            </Button>
          </div>
        </CardContent>
      </Card>
    )
  } else {
    content = null
  }

  return (
    <main className='bg-background text-foreground flex min-h-svh items-center px-4 py-10'>
      <div className='mx-auto w-full max-w-xl'>
        <h1 className='mb-6 text-center text-2xl font-semibold'>
          {t('SheJane Cloud authorization')}
        </h1>
        {content}
      </div>
    </main>
  )
}

function AuthorizationError(props: {
  title: string
  description: string
  onRetry?: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='space-y-3'>
      <Alert variant='destructive'>
        <AlertCircle aria-hidden='true' />
        <AlertTitle>{props.title}</AlertTitle>
        <AlertDescription>{props.description}</AlertDescription>
      </Alert>
      {props.onRetry && (
        <Button type='button' variant='outline' onClick={props.onRetry}>
          {t('Retry')}
        </Button>
      )}
    </div>
  )
}
