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

/**
 * Text for a failed write (create, edit, delete). The server's own message is shown for rules the
 * user can act on (a blank name, a date in the future, the account's person limit); those codes are
 * listed in the OpenAPI file and their messages are written for people.
 */
export function describeWriteError(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.code) {
      case 'validation_failed':
      case 'person_limit_reached':
        return error.message;
      case 'person_not_found':
        return 'That person no longer exists. Your family has been refreshed.';
      case 'request_too_large':
        return 'That is too much text to save at once.';
    }
  }
  return describeApiError(error);
}
