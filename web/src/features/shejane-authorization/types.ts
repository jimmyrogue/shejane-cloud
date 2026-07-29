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
export type SheJaneDeviceMetadata = {
  name: string
  platform: string
  app_version: string
}

export type SheJaneAuthorizationStartRequest = {
  response_type: string
  client_id: string
  redirect_uri: string
  code_challenge: string
  code_challenge_method: string
  state: string
  device: SheJaneDeviceMetadata
}

export type SheJaneAuthorizationConsent = {
  client: { id: string; name: string }
  device: SheJaneDeviceMetadata
  expires_at: number
}

export type SheJaneDevice = SheJaneDeviceMetadata & {
  id: number
  client_id: string
  created_at: number
  revoked_at: number
}

export type SheJaneEnvelope<T> = {
  success: boolean
  message: string
  code?: string
  data?: T
}
