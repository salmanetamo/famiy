import { useEffect, useRef } from 'react';
import { AuthPage } from './auth/AuthPage';
import { authErrorMessage } from './auth/messages';
import { useLogout, useSession, type Session } from './auth/session';
import { GraphScreen } from './graph/GraphScreen';
import type { LayoutEngine } from './layout/types';
import { getAppLayoutEngine } from './layout/workerEngine';

interface Props {
  /** The layout engine is a Web Worker in the app; tests pass a plain in-thread engine. */
  getLayoutEngine?: () => LayoutEngine;
}

export function App({ getLayoutEngine = getAppLayoutEngine }: Props) {
  const session = useSession();
  const logout = useLogout();
  useMoveFocusOnSignInOrOut(session.status);

  switch (session.status) {
    case 'loading':
    case 'error':
      // One persistent live region for both states, so the error is announced when it replaces
      // the loading text (a region inserted already filled is not reliably announced).
      return (
        <main>
          <h1 tabIndex={-1}>Family Graph</h1>
          <div role="status">
            {session.status === 'loading'
              ? 'Checking your session…'
              : authErrorMessage(session.error)}
          </div>
          {session.status === 'error' && (
            <button type="button" onClick={session.retry}>
              Try again
            </button>
          )}
        </main>
      );
    case 'signedOut':
      return <AuthPage />;
    case 'signedIn':
      return (
        <>
          <header>
            <span>Signed in as {session.user.email}</span>{' '}
            <button
              type="button"
              disabled={logout.isPending}
              onClick={() => {
                logout.mutate();
              }}
            >
              Sign out
            </button>
            <div role="alert">{logout.isError ? authErrorMessage(logout.error) : null}</div>
          </header>
          <main>
            <h1 tabIndex={-1} className="visually-hidden">
              Family Graph
            </h1>
            <GraphScreen getLayoutEngine={getLayoutEngine} />
          </main>
        </>
      );
  }
}

/**
 * When the screen changes between sign-in and the signed-in view, the control the user was on is
 * gone and focus would fall to <body>. Move it to the new page's heading (tabIndex -1, so it takes
 * focus without joining the tab order). Not done on first load, which would steal normal tab order.
 */
function useMoveFocusOnSignInOrOut(status: Session['status']) {
  const previous = useRef(status);
  useEffect(() => {
    const was = previous.current;
    previous.current = status;
    const switched =
      (was === 'signedOut' && status === 'signedIn') ||
      (was === 'signedIn' && status === 'signedOut');
    if (switched) document.querySelector<HTMLElement>('main h1')?.focus();
  }, [status]);
}
