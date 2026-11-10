import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { login, logout, me, register } from '../api/auth';
import type { Credentials, User } from '../api/types';
import { sessionKey } from './sessionKey';
import { endSession, startSession } from './sessionLifecycle';

export type Session =
  | { status: 'loading' }
  | { status: 'signedOut' }
  | { status: 'signedIn'; user: User }
  | { status: 'error'; error: Error; retry: () => void };

export function useSession(): Session {
  const query = useQuery({ queryKey: sessionKey, queryFn: me, staleTime: Infinity });
  if (query.isPending) return { status: 'loading' };
  if (query.isError) {
    return {
      status: 'error',
      error: query.error,
      retry: () => {
        void query.refetch();
      },
    };
  }
  return query.data === null ? { status: 'signedOut' } : { status: 'signedIn', user: query.data };
}

function useSignIn(fn: (credentials: Credentials) => Promise<User>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: (user) => startSession(queryClient, user),
  });
}

export const useLogin = () => useSignIn(login);
export const useRegister = () => useSignIn(register);

/**
 * Signs out on the server, then cancels in-flight queries and drops every cached query except the
 * session itself (see endSession), so no family data stays in memory. If the request fails (the server was unreachable) the user stays signed
 * in and sees the error, because the session cookie may still be valid.
 */
export function useLogout() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: logout,
    onSuccess: () => {
      endSession(queryClient);
    },
  });
}
