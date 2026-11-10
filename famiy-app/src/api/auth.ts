import { ApiError, request } from './client';
import type { Credentials, User } from './types';

export function register(credentials: Credentials): Promise<User> {
  return request<User>('POST', '/auth/register', credentials);
}

export function login(credentials: Credentials): Promise<User> {
  return request<User>('POST', '/auth/login', credentials);
}

export function logout(): Promise<void> {
  return request<undefined>('POST', '/auth/logout');
}

/** The signed-in account, or `null` when there is no valid session (a 401 is an answer, not a failure). */
export async function me(): Promise<User | null> {
  try {
    return await request<User>('GET', '/auth/me');
  } catch (error) {
    if (error instanceof ApiError && error.code === 'unauthenticated') return null;
    throw error;
  }
}
