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
import { describe, test } from 'node:test'

import {
  parseSheJaneAuthorizationSearch,
  trustedSheJaneLoopbackRedirect,
} from '../authorization'

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

describe('SheJane authorization route input', () => {
  test('maps the exact native start query into the backend request', () => {
    assert.deepEqual(parseSheJaneAuthorizationSearch(startSearch), {
      kind: 'start',
      request: {
        response_type: 'code',
        client_id: 'shejane-desktop',
        redirect_uri: 'http://127.0.0.1:49152/shejane/auth/callback',
        code_challenge: 'A'.repeat(43),
        code_challenge_method: 'S256',
        state: 'B'.repeat(43),
        device: {
          name: "Jimmy's Mac",
          platform: 'macos',
          app_version: '0.1.8',
        },
      },
    })
  })

  test('accepts only an opaque flow token or one complete start request', () => {
    assert.deepEqual(
      parseSheJaneAuthorizationSearch({ flow_token: 'opaque-flow' }),
      { kind: 'resume', flowToken: 'opaque-flow' }
    )
    assert.deepEqual(
      parseSheJaneAuthorizationSearch({
        ...startSearch,
        flow_token: 'ambiguous',
      }),
      { kind: 'invalid' }
    )
    assert.deepEqual(
      parseSheJaneAuthorizationSearch({ ...startSearch, state: undefined }),
      { kind: 'invalid' }
    )
    assert.deepEqual(parseSheJaneAuthorizationSearch({}), { kind: 'invalid' })
  })
})

describe('SheJane decision redirect', () => {
  test('accepts only the exact loopback success and denial callbacks', () => {
    const success =
      'http://127.0.0.1:49152/shejane/auth/callback?code=opaque-code&state=opaque-state'
    const denied =
      'http://127.0.0.1:49152/shejane/auth/callback?error=access_denied&state=opaque-state'

    assert.equal(trustedSheJaneLoopbackRedirect(success), success)
    assert.equal(trustedSheJaneLoopbackRedirect(denied), denied)
  })

  test('rejects remote, alternate, and over-specified redirects', () => {
    const rejected = [
      'https://cloud.example.com/dashboard',
      'http://localhost:49152/shejane/auth/callback?code=x&state=y',
      'http://127.0.0.1:49152/other?code=x&state=y',
      'http://127.0.0.1:49152/shejane/auth/callback?code=x&state=y&next=/dashboard',
      'http://127.0.0.1:49152/shejane/auth/callback?error=server_error&state=y',
      'http://127.0.0.1:49152/shejane/auth/callback?code=x&error=access_denied&state=y',
    ]

    for (const value of rejected) {
      assert.equal(trustedSheJaneLoopbackRedirect(value), null)
    }
  })
})
