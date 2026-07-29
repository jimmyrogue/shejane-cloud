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

const domWindow = new Window({
  url: 'https://cloud.example.com/shejane/authorize',
})
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
  'scrollTo',
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
const {
  Outlet,
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} = await import('@tanstack/react-router')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { api } = await import('@/lib/api')
const { consumeAuthContinuation } =
  await import('@/features/auth/lib/auth-redirect')
const { useAuthStore } = await import('@/stores/auth-store')
const { AuthorizationPage } = await import('../authorization-page')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const startSearch = {
  response_type: 'code',
  client_id: 'shejane-desktop',
  redirect_uri: 'http://127.0.0.1:49152/shejane/auth/callback',
  code_challenge: 'A'.repeat(43),
  code_challenge_method: 'S256',
  state: 'B'.repeat(43),
  device_name: "Jimmy's Mac",
  platform: 'macos',
  app_version: '0.1.8',
}

function createTestRouter(search: unknown) {
  const rootRoute = createRootRoute({ component: () => <Outlet /> })
  const authorizationRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/shejane/authorize',
    component: () => <AuthorizationPage search={search} />,
  })
  const signInRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/sign-in',
    component: () => <p>Sign in</p>,
  })
  return createRouter({
    routeTree: rootRoute.addChildren([authorizationRoute, signInRoute]),
    history: createMemoryHistory({ initialEntries: ['/shejane/authorize'] }),
  })
}

async function waitForText(container: HTMLElement, value: string) {
  await new Promise<void>((resolve) => {
    const finish = () => {
      if (!container.textContent?.includes(value)) return
      observer.disconnect()
      resolve()
    }
    const observer = new MutationObserver(finish)
    observer.observe(container, { childList: true, subtree: true })
    finish()
  })
}

describe('SheJane authorization page', () => {
  after(() => domWindow.close())

  test('starts once, replaces native fields with a flow token, and continues through sign-in', async () => {
    useAuthStore.getState().auth.reset()
    window.sessionStorage.clear()
    window.history.replaceState(null, '', '/shejane/authorize')
    const requests: unknown[] = []
    const originalAdapter = api.defaults.adapter
    api.defaults.adapter = async (config) => {
      requests.push(JSON.parse(String(config.data)))
      return {
        data: {
          success: true,
          message: '',
          data: { flow_token: 'opaque-flow', expires_at: 1_900_000_000 },
        },
        status: 201,
        statusText: 'Created',
        headers: {},
        config,
      }
    }

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const queryClient = new QueryClient()
    const router = createTestRouter(startSearch)

    await act(async () => {
      root.render(
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <RouterProvider router={router} />
          </I18nextProvider>
        </QueryClientProvider>
      )
      await router.load()
    })
    await act(async () => {
      await waitForText(container, 'Sign in')
    })

    assert.equal(requests.length, 1)
    assert.deepEqual(requests[0], {
      response_type: 'code',
      client_id: 'shejane-desktop',
      redirect_uri: 'http://127.0.0.1:49152/shejane/auth/callback',
      code_challenge: 'A'.repeat(43),
      code_challenge_method: 'S256',
      state: 'B'.repeat(43),
      device: { name: "Jimmy's Mac", platform: 'macos', app_version: '0.1.8' },
    })
    assert.equal(window.location.search, '?flow_token=opaque-flow')
    assert.equal(
      consumeAuthContinuation(window.location.origin),
      '/shejane/authorize?flow_token=opaque-flow'
    )

    await act(async () => root.unmount())
    container.remove()
    queryClient.clear()
    api.defaults.adapter = originalAdapter
  })

  test('shows stored consent metadata and never renders credential-shaped response fields', async () => {
    useAuthStore.getState().auth.setUser({ id: 1, username: 'user', role: 1 })
    window.history.replaceState(
      null,
      '',
      '/shejane/authorize?flow_token=opaque-flow'
    )
    const originalAdapter = api.defaults.adapter
    api.defaults.adapter = async (config) => ({
      data: {
        success: true,
        message: '',
        data: {
          client: { id: 'shejane-desktop', name: 'SheJane Desktop' },
          device: {
            name: "Jimmy's Mac",
            platform: 'macos',
            app_version: '0.1.8',
          },
          expires_at: 1_900_000_000,
          access_token: 'sk-must-never-render',
          token_id: 99,
        },
      },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    })

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const router = createTestRouter({ flow_token: 'opaque-flow' })

    await act(async () => {
      root.render(
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <RouterProvider router={router} />
          </I18nextProvider>
        </QueryClientProvider>
      )
      await router.load()
    })
    await act(async () => {
      await waitForText(container, "Jimmy's Mac")
    })

    assert.equal(container.textContent?.includes('SheJane Desktop'), true)
    assert.equal(container.textContent?.includes('sk-must-never-render'), false)
    assert.equal(container.textContent?.includes('99'), false)
    const decisions = new Set(
      [...container.querySelectorAll('button')].map(
        (button) => button.textContent
      )
    )
    assert.equal(decisions.has('Cancel'), true)
    assert.equal(decisions.has('Authorize'), true)

    await act(async () => root.unmount())
    container.remove()
    queryClient.clear()
    api.defaults.adapter = originalAdapter
    useAuthStore.getState().auth.reset()
  })

  test('requires an explicit cancel decision and follows only the stored denial callback', async () => {
    useAuthStore.getState().auth.setUser({ id: 1, username: 'user', role: 1 })
    domWindow.location.href =
      'https://cloud.example.com/shejane/authorize?flow_token=opaque-flow'
    const originalAdapter = api.defaults.adapter
    let postedDecision = ''
    api.defaults.adapter = async (config) => {
      if (config.method === 'post') {
        postedDecision = JSON.parse(String(config.data)).decision
        return {
          data: {
            success: true,
            message: '',
            data: {
              redirect_to:
                'http://127.0.0.1:49152/shejane/auth/callback?error=access_denied&state=opaque-state',
            },
          },
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
          data: {
            client: { id: 'shejane-desktop', name: 'SheJane Desktop' },
            device: {
              name: 'Test Mac',
              platform: 'macos',
              app_version: '0.1.8',
            },
            expires_at: 1_900_000_000,
          },
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
      defaultOptions: { queries: { retry: false } },
    })
    const router = createTestRouter({ flow_token: 'opaque-flow' })

    await act(async () => {
      root.render(
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <RouterProvider router={router} />
          </I18nextProvider>
        </QueryClientProvider>
      )
      await router.load()
    })
    await act(async () => waitForText(container, 'Test Mac'))

    const cancelButton = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Cancel'
    )
    assert.ok(cancelButton)
    await act(async () => {
      cancelButton.click()
    })

    assert.equal(postedDecision, 'deny')
    assert.equal(
      window.location.href,
      'http://127.0.0.1:49152/shejane/auth/callback?error=access_denied&state=opaque-state'
    )

    await act(async () => root.unmount())
    container.remove()
    queryClient.clear()
    api.defaults.adapter = originalAdapter
    useAuthStore.getState().auth.reset()
  })
})
