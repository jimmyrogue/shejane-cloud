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
import { api } from '@/lib/api'

import type {
  SheJaneAuthorizationConsent,
  SheJaneAuthorizationStartRequest,
  SheJaneDevice,
  SheJaneEnvelope,
} from './types'

const controlledRequest = {
  skipBusinessError: true,
  skipErrorHandler: true,
}

export function getSheJaneErrorCode(error: unknown): string | undefined {
  const value = error as { response?: { data?: { code?: unknown } } }
  return typeof value.response?.data?.code === 'string'
    ? value.response.data.code
    : undefined
}

export async function startSheJaneAuthorization(
  request: SheJaneAuthorizationStartRequest
): Promise<SheJaneEnvelope<{ flow_token: string; expires_at: number }>> {
  const response = await api.post(
    '/api/shejane/authorize/start',
    request,
    controlledRequest
  )
  return response.data
}

export async function getSheJaneAuthorization(
  flowToken: string
): Promise<SheJaneEnvelope<SheJaneAuthorizationConsent>> {
  const response = await api.get(
    `/api/shejane/authorize/${encodeURIComponent(flowToken)}`,
    { ...controlledRequest, disableDuplicate: true }
  )
  return response.data
}

export async function decideSheJaneAuthorization(
  flowToken: string,
  decision: 'approve' | 'deny'
): Promise<SheJaneEnvelope<{ redirect_to: string }>> {
  const response = await api.post(
    `/api/shejane/authorize/${encodeURIComponent(flowToken)}`,
    { decision },
    controlledRequest
  )
  return response.data
}

export async function getSheJaneDevices(): Promise<
  SheJaneEnvelope<SheJaneDevice[]>
> {
  const response = await api.get('/api/shejane/devices', controlledRequest)
  return response.data
}

export async function revokeSheJaneDevice(
  id: number
): Promise<SheJaneEnvelope<{ id: number; revoked: boolean }>> {
  const response = await api.delete(
    `/api/shejane/devices/${encodeURIComponent(String(id))}`,
    controlledRequest
  )
  return response.data
}
