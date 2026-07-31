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
import { Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

type SheJaneHomeProps = {
  isAuthenticated: boolean
}

export const SHEJANE_MARK_DATA_URI =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Cpath d='M25.4 8.1a11.5 11.5 0 1 0 1.8 12.7' fill='none' stroke='%232B2A28' stroke-width='2.6' stroke-linecap='round'/%3E%3Cpath d='M27.2 20.8c.5-1.4.7-2.9.6-4.3' fill='none' stroke='%23B3532F' stroke-width='2.6' stroke-linecap='round'/%3E%3C/svg%3E"

export function SheJaneMark(props: { className?: string }) {
  return (
    <svg aria-hidden='true' viewBox='0 0 32 32' className={props.className}>
      <path
        d='M25.4 8.1a11.5 11.5 0 1 0 1.8 12.7'
        fill='none'
        stroke='currentColor'
        strokeWidth='2.6'
        strokeLinecap='round'
      />
      <path
        d='M27.2 20.8c.5-1.4.7-2.9.6-4.3'
        fill='none'
        stroke='#b3532f'
        strokeWidth='2.6'
        strokeLinecap='round'
      />
    </svg>
  )
}

export function SheJaneHome(props: SheJaneHomeProps) {
  const { t } = useTranslation()
  const benefits = [
    {
      title: t('Your files stay on your computer'),
      description: t(
        'SheJane works with local files instead of uploading your whole workspace.'
      ),
    },
    {
      title: t('Important actions need your approval'),
      description: t(
        'You decide before SheJane sends, deletes, or runs anything sensitive.'
      ),
    },
    {
      title: t('Account tools are easy to find'),
      description: t(
        'Devices, API keys, balance, and usage are all in one small console.'
      ),
    },
  ]
  const steps = [
    t('Create account'),
    t('Authorize SheJane Desktop'),
    t('Open the desktop app and start chatting'),
  ]

  return (
    <main className='shejane-user-surface bg-background text-foreground'>
      <section className='border-border relative overflow-hidden border-b px-5 pt-28 pb-20 md:px-8 md:pt-36 md:pb-24'>
        <div
          aria-hidden='true'
          className='pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_72%_24%,rgba(179,83,47,0.07),transparent_23%),linear-gradient(rgba(232,229,222,0.45)_1px,transparent_1px),linear-gradient(90deg,rgba(232,229,222,0.45)_1px,transparent_1px)] [mask-image:linear-gradient(to_bottom,black,transparent_82%)] bg-[size:auto,3.5rem_3.5rem,3.5rem_3.5rem]'
        />
        <div className='relative mx-auto grid max-w-6xl items-center gap-14 lg:grid-cols-[minmax(0,1fr)_24rem] lg:gap-20'>
          <div>
            <div className='mb-7 flex items-center gap-3 text-sm'>
              <SheJaneMark className='text-foreground size-8' />
              <span className='font-serif font-semibold tracking-[0.12em]'>
                石间 · SheJane
              </span>
            </div>
            <h1 className='max-w-3xl text-[clamp(2.8rem,7vw,5.4rem)] leading-[0.98] font-semibold tracking-[-0.055em] text-balance'>
              {t('Let AI help you get work done.')}
            </h1>
            <p className='text-muted-foreground mt-7 max-w-xl text-base leading-8 text-pretty md:text-lg'>
              {t(
                'SheJane runs on your computer. It can read and write files, use tools, and asks before important actions.'
              )}
            </p>
            <div className='mt-9 flex flex-wrap items-center gap-3'>
              <Button
                className='h-10 px-5'
                render={
                  <Link
                    to={props.isAuthenticated ? '/dashboard' : '/sign-up'}
                  />
                }
              >
                {t(
                  props.isAuthenticated ? 'Go to Dashboard' : 'Create account'
                )}
                <ArrowRight aria-hidden='true' className='ml-1.5 size-4' />
              </Button>
              <Button
                variant='ghost'
                className='h-10 px-4'
                render={
                  <Link to={props.isAuthenticated ? '/devices' : '/sign-in'} />
                }
              >
                {t(props.isAuthenticated ? 'SheJane devices' : 'Sign in')}
              </Button>
            </div>
          </div>

          <aside className='border-border bg-card relative rounded-xl border p-5 shadow-[0_2px_8px_rgba(43,42,40,0.06)]'>
            <div className='space-y-5'>
              <div>
                <p className='text-muted-foreground text-xs'>{t('You')}</p>
                <p className='mt-2 text-sm leading-6'>
                  {t("Help me turn this week's project notes into a report.")}
                </p>
              </div>
              <div className='bg-secondary rounded-lg p-4'>
                <p className='text-muted-foreground text-xs'>SheJane</p>
                <p className='mt-2 text-sm leading-6'>
                  {t(
                    'I will read the documents in this workspace and draft a report.'
                  )}
                </p>
              </div>
            </div>
            <div className='border-border mt-5 border-t pt-4'>
              <p className='text-sm font-medium'>{t('Read workspace files')}</p>
              <div className='mt-4 flex gap-2'>
                <span className='bg-foreground text-background rounded-md px-3 py-2 text-xs font-medium'>
                  {t('Allow once')}
                </span>
                <span className='border-border rounded-md border px-3 py-2 text-xs font-medium'>
                  {t('Not now')}
                </span>
              </div>
            </div>
          </aside>
        </div>
      </section>

      <section className='px-5 py-16 md:px-8 md:py-20'>
        <div className='mx-auto max-w-6xl'>
          <h2 className='max-w-xl text-3xl leading-tight font-semibold tracking-[-0.03em] text-balance md:text-4xl'>
            {t('Simple, useful, and under your control')}
          </h2>
          <ul className='border-border mt-10 border-t'>
            {benefits.map((benefit) => (
              <li
                key={benefit.title}
                className='border-border grid gap-2 border-b py-6 md:grid-cols-[17rem_1fr] md:gap-10'
              >
                <h3 className='font-semibold'>{benefit.title}</h3>
                <p className='text-muted-foreground leading-7'>
                  {benefit.description}
                </p>
              </li>
            ))}
          </ul>
        </div>
      </section>

      <section className='bg-secondary/70 border-border border-y px-5 py-16 md:px-8 md:py-20'>
        <div className='mx-auto max-w-6xl'>
          <h2 className='text-3xl font-semibold tracking-[-0.03em] md:text-4xl'>
            {t('Start in three steps')}
          </h2>
          <ol className='border-border mt-10 border-t'>
            {steps.map((step, index) => (
              <li
                key={step}
                className='border-border flex items-center gap-5 border-b py-5'
              >
                <span className='bg-foreground text-background flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold'>
                  {index + 1}
                </span>
                <span className='font-medium'>{step}</span>
              </li>
            ))}
          </ol>
          <Button
            className='mt-8 h-10 px-5'
            render={
              <Link to={props.isAuthenticated ? '/dashboard' : '/sign-up'} />
            }
          >
            {t(props.isAuthenticated ? 'Go to Dashboard' : 'Create account')}
            <ArrowRight aria-hidden='true' className='ml-1.5 size-4' />
          </Button>
        </div>
      </section>
    </main>
  )
}
