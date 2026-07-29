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
import assert from 'node:assert/strict'
import { after, describe, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { api } = await import('@/lib/api')
const { SheJaneDevicesCard } = await import('../shejane-devices-card')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

describe('SheJane device management', () => {
  after(() => domWindow.close())

  test('shows safe device metadata without rendering inference credentials', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    queryClient.setQueryData(
      ['profile', 'shejane-devices'],
      [
        {
          id: 17,
          client_id: 'shejane-desktop',
          name: "Jimmy's Mac",
          platform: 'macos',
          app_version: '0.1.8',
          created_at: 1_785_232_800,
          revoked_at: 0,
          access_token: 'sk-must-never-render',
          token_id: 99,
        },
      ]
    )

    await act(async () => {
      root.render(
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <SheJaneDevicesCard />
          </I18nextProvider>
        </QueryClientProvider>
      )
    })

    assert.equal(container.textContent?.includes("Jimmy's Mac"), true)
    assert.equal(container.textContent?.includes('0.1.8'), true)
    assert.equal(container.textContent?.includes('sk-must-never-render'), false)
    assert.equal(container.textContent?.includes('99'), false)
    assert.ok(
      [...container.querySelectorAll('button')].find(
        (button) => button.textContent === 'Revoke'
      )
    )

    await act(async () => root.unmount())
    container.remove()
    queryClient.clear()
  })

  test('requires confirmation and refreshes the device as revoked after success', async () => {
    const originalAdapter = api.defaults.adapter
    let revoked = false
    let deleteCount = 0
    api.defaults.adapter = async (config) => {
      if (config.method === 'delete') {
        revoked = true
        deleteCount += 1
        return {
          data: { success: true, message: '', data: { id: 17, revoked: true } },
          status: 200,
          statusText: 'OK',
          headers: {},
          config,
        }
      }
      return {
        data: {
          success: true,
          message: '',
          data: [
            {
              id: 17,
              client_id: 'shejane-desktop',
              name: 'Test Mac',
              platform: 'macos',
              app_version: '0.1.8',
              created_at: 1_785_232_800,
              revoked_at: revoked ? 1_785_233_000 : 0,
            },
          ],
        },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    queryClient.setQueryData(
      ['profile', 'shejane-devices'],
      [
        {
          id: 17,
          client_id: 'shejane-desktop',
          name: 'Test Mac',
          platform: 'macos',
          app_version: '0.1.8',
          created_at: 1_785_232_800,
          revoked_at: 0,
        },
      ]
    )

    await act(async () => {
      root.render(
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <SheJaneDevicesCard />
          </I18nextProvider>
        </QueryClientProvider>
      )
    })
    const revokeButton = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Revoke'
    )
    assert.ok(revokeButton)
    await act(async () => revokeButton.click())
    assert.equal(
      document.body.textContent?.includes('Revoke SheJane device?'),
      true
    )
    assert.equal(deleteCount, 0)

    const confirmButton = [...document.body.querySelectorAll('button')]
      .reverse()
      .find((button) => button.textContent === 'Revoke')
    assert.ok(confirmButton)
    await act(async () => {
      await new Promise<void>((resolve) => {
        const finish = () => {
          const succeeded = queryClient
            .getMutationCache()
            .getAll()
            .some((mutation) => mutation.state.status === 'success')
          if (!succeeded) return
          unsubscribe()
          resolve()
        }
        const unsubscribe = queryClient.getMutationCache().subscribe(finish)
        confirmButton.click()
        finish()
      })
    })

    assert.equal(deleteCount, 1)
    assert.equal(
      [...container.querySelectorAll('button')].some(
        (button) => button.textContent === 'Revoke'
      ),
      false
    )

    await act(async () => root.unmount())
    container.remove()
    queryClient.clear()
    api.defaults.adapter = originalAdapter
  })

  test('announces loading and exposes an accessible retry after failure', async () => {
    const originalAdapter = api.defaults.adapter
    let rejectRequest: ((reason: Error) => void) | undefined
    api.defaults.adapter = () =>
      new Promise((_, reject) => {
        rejectRequest = reject
      })

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    await act(async () => {
      root.render(
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <SheJaneDevicesCard />
          </I18nextProvider>
        </QueryClientProvider>
      )
    })

    const loading = container.querySelector('[role="status"]')
    assert.ok(loading)
    assert.equal(loading.getAttribute('aria-label'), 'Loading devices')
    assert.ok(rejectRequest)

    await act(async () => {
      await new Promise<void>((resolve) => {
        const finish = () => {
          if (
            queryClient.getQueryState(['profile', 'shejane-devices'])
              ?.status !== 'error'
          ) {
            return
          }
          unsubscribe()
          resolve()
        }
        const unsubscribe = queryClient.getQueryCache().subscribe(finish)
        rejectRequest?.(new Error('network unavailable'))
        finish()
      })
    })
    await act(async () => {
      await new Promise<void>((resolve) => {
        const finish = () => {
          if (
            !container.textContent?.includes('Unable to load SheJane devices')
          ) {
            return
          }
          observer.disconnect()
          resolve()
        }
        const observer = new MutationObserver(finish)
        observer.observe(container, { childList: true, subtree: true })
        finish()
      })
    })

    assert.equal(
      container.textContent?.includes('Unable to load SheJane devices'),
      true
    )
    assert.ok(
      [...container.querySelectorAll('button')].find(
        (button) => button.textContent === 'Retry'
      )
    )

    await act(async () => root.unmount())
    container.remove()
    queryClient.clear()
    api.defaults.adapter = originalAdapter
  })
})
