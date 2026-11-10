import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query';
import { ApiError } from './api/client';
import { endSession } from './auth/sessionLifecycle';

/**
 * One QueryClient for the app. A 401 `unauthenticated` from ANY request means the session ended
 * (expired or signed out elsewhere), so the session is ended (cached data dropped too) and the sign-in screen shows.
 * Other 401s (`invalid_credentials` from a login attempt) are not session expiry.
 */
export function createQueryClient(): QueryClient {
  const handleError = (error: unknown): void => {
    if (error instanceof ApiError && error.code === 'unauthenticated') {
      endSession(client);
    }
  };
  const client: QueryClient = new QueryClient({
    queryCache: new QueryCache({ onError: handleError }),
    mutationCache: new MutationCache({ onError: handleError }),
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return client;
}
