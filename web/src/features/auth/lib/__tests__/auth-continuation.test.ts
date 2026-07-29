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
  consumeAuthContinuation,
  resolveAuthRedirect,
  saveAuthContinuation,
} from '../auth-redirect'

class MemoryStorage implements Storage {
  private values = new Map<string, string>()

  get length() {
    return this.values.size
  }

  clear() {
    this.values.clear()
  }

  getItem(key: string) {
    return this.values.get(key) ?? null
  }

  key(index: number) {
    return [...this.values.keys()][index] ?? null
  }

  removeItem(key: string) {
    this.values.delete(key)
  }

  setItem(key: string, value: string) {
    this.values.set(key, value)
  }
}

const origin = 'https://cloud.example.com'
const continuation = '/shejane/authorize?flow_token=opaque-flow'

describe('SheJane authentication continuation', () => {
  test('stores a safe flow path for ten minutes and consumes it once', () => {
    const storage = new MemoryStorage()

    assert.equal(
      saveAuthContinuation(continuation, origin, storage, 1_000),
      true
    )
    assert.equal(
      consumeAuthContinuation(origin, storage, 1_000 + 10 * 60 * 1_000 - 1),
      continuation
    )
    assert.equal(consumeAuthContinuation(origin, storage, 1_001), null)
  })

  test('rejects expired, external, and ambiguous destinations', () => {
    const rejected = [
      'https://attacker.example/shejane/authorize?flow_token=opaque-flow',
      '//attacker.example/shejane/authorize?flow_token=opaque-flow',
      '/shejane/authorize?flow_token=opaque-flow&redirect=https://attacker.example',
      '/shejane/authorize?flow_token=',
      '/dashboard?flow_token=opaque-flow',
    ]

    for (const value of rejected) {
      assert.equal(
        saveAuthContinuation(value, origin, new MemoryStorage(), 1_000),
        false
      )
    }

    const storage = new MemoryStorage()
    assert.equal(
      saveAuthContinuation(continuation, origin, storage, 1_000),
      true
    )
    assert.equal(
      consumeAuthContinuation(origin, storage, 1_000 + 10 * 60 * 1_000),
      null
    )
  })

  test('prefers a valid explicit redirect and otherwise consumes the continuation', () => {
    const storage = new MemoryStorage()
    assert.equal(
      saveAuthContinuation(continuation, origin, storage, 1_000),
      true
    )

    assert.equal(
      resolveAuthRedirect('/profile', origin, storage, 1_001),
      '/profile'
    )
    assert.equal(
      resolveAuthRedirect('https://attacker.example', origin, storage, 1_001),
      continuation
    )
    assert.equal(
      resolveAuthRedirect(undefined, origin, storage, 1_001),
      '/dashboard'
    )
  })
})
