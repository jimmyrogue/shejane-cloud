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
import { z } from 'zod'

import type { SheJaneAuthorizationStartRequest } from '../types'

const sheJaneAuthorizationStartSearchSchema = z.object({
  response_type: z.string(),
  client_id: z.string(),
  redirect_uri: z.string(),
  code_challenge: z.string(),
  code_challenge_method: z.string(),
  state: z.string(),
  device_name: z.string(),
  platform: z.string(),
  app_version: z.string(),
})

export const sheJaneAuthorizationSearchSchema =
  sheJaneAuthorizationStartSearchSchema.partial().extend({
    flow_token: z.string().optional(),
  })

type SheJaneAuthorizationSearch = z.infer<
  typeof sheJaneAuthorizationSearchSchema
>

export type ParsedSheJaneAuthorizationSearch =
  | { kind: 'resume'; flowToken: string }
  | { kind: 'start'; request: SheJaneAuthorizationStartRequest }
  | { kind: 'invalid' }

export function parseSheJaneAuthorizationSearch(
  value: unknown
): ParsedSheJaneAuthorizationSearch {
  const parsed = sheJaneAuthorizationSearchSchema.safeParse(value)
  if (!parsed.success) return { kind: 'invalid' }

  const search: SheJaneAuthorizationSearch = parsed.data
  const startValues = [
    search.response_type,
    search.client_id,
    search.redirect_uri,
    search.code_challenge,
    search.code_challenge_method,
    search.state,
    search.device_name,
    search.platform,
    search.app_version,
  ]
  const hasStartValue = startValues.some((item) => item !== undefined)
  if (search.flow_token) {
    return hasStartValue
      ? { kind: 'invalid' }
      : { kind: 'resume', flowToken: search.flow_token }
  }
  const start = sheJaneAuthorizationStartSearchSchema.safeParse(search)
  if (!start.success) return { kind: 'invalid' }

  return {
    kind: 'start',
    request: {
      response_type: start.data.response_type,
      client_id: start.data.client_id,
      redirect_uri: start.data.redirect_uri,
      code_challenge: start.data.code_challenge,
      code_challenge_method: start.data.code_challenge_method,
      state: start.data.state,
      device: {
        name: start.data.device_name,
        platform: start.data.platform,
        app_version: start.data.app_version,
      },
    },
  }
}

export function trustedSheJaneLoopbackRedirect(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const match = value.match(
    /^http:\/\/127\.0\.0\.1:(\d{1,5})\/shejane\/auth\/callback\?/
  )
  if (!match) return null
  const port = Number(match[1])
  if (!Number.isInteger(port) || port < 1 || port > 65535) return null

  let target: URL
  try {
    target = new URL(value)
  } catch {
    return null
  }
  if (target.hash) return null

  const keys = [...target.searchParams.keys()]
  const state = target.searchParams.getAll('state')
  const codes = target.searchParams.getAll('code')
  const errors = target.searchParams.getAll('error')
  const success =
    codes.length === 1 &&
    Boolean(codes[0]) &&
    errors.length === 0 &&
    keys.every((key) => key === 'code' || key === 'state')
  const denied =
    codes.length === 0 &&
    errors.length === 1 &&
    errors[0] === 'access_denied' &&
    keys.every((key) => key === 'error' || key === 'state')
  return state.length === 1 && Boolean(state[0]) && (success || denied)
    ? value
    : null
}
