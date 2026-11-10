import type { QueryClient } from '@tanstack/react-query';
import type { User } from '../api/types';
import { sessionKey } from './sessionKey';

const isNotSession = (query: { queryKey: readonly unknown[] }) =>
  query.queryKey[0] !== sessionKey[0];

/**
 * Ends the session: removes every cached query except the session, then marks the session as
 * signed out. Used for sign out and for a 401 `unauthenticated` from anywhere, so the next person
 * to sign in on this tab never sees the previous account's data. `removeQueries` destroys each
 * query, which also silently cancels a fetch still in flight, so a late response cannot put data
 * back (shown by tests: removing an extra `cancelQueries` here changed no test result).
 */
export function endSession(client: QueryClient): void {
  client.removeQueries({ predicate: isNotSession });
  client.setQueryData(sessionKey, null);
}

/**
 * Starts a session: clears leftover data (which also cancels requests begun under the old session,
 * so their late 401 cannot end this new session), cancels a pending session check (the session
 * query is kept, so it is not removed), then stores the account. Without that cancel, a slow
 * "no session" answer to the first `me` check would overwrite a sign-in that finished first.
 */
export async function startSession(client: QueryClient, user: User): Promise<void> {
  client.removeQueries({ predicate: isNotSession });
  await client.cancelQueries({ queryKey: sessionKey });
  client.setQueryData(sessionKey, user);
}
