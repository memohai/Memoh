import { fetchApi } from '@/utils/request'

export interface LoginRequest {
  username: string
  password: string
}

export interface LoginResponse {
  access_token: string
  user_id: string
  username: string
}

export async function login(data: LoginRequest): Promise<LoginResponse> {
  return fetchApi<LoginResponse>('/auth/login', {
    method: 'POST',
    body: data,
    noAuth: true,
  })
}
