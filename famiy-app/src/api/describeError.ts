import { ApiError } from './client';

/** Plain-language text for failures that can happen on any request. Branches on the error `code`. */
export function describeApiError(error: unknown): string {
  if (!(error instanceof ApiError)) return 'Something went wrong. Please try again.';
  switch (error.code) {
    case 'network_error':
      return 'Could not reach the server. Check your connection and try again.';
    case 'server_unavailable':
      return "The server isn't reachable. If you are running it locally, is the API started?";
    case 'unauthenticated':
      return 'Your session has ended. Please sign in again.';
    default: {
      const reference = error.requestId ? ` Reference: ${error.requestId}.` : '';
      return `Something went wrong (${error.code}).${reference}`;
    }
  }
}
