import { ApiError } from '../api/client';
import { describeApiError } from '../api/describeError';

function waitText(seconds: number | undefined): string {
  if (seconds === undefined) return 'Try again later.';
  const minutes = Math.max(1, Math.ceil(seconds / 60));
  return `Try again in ${String(minutes)} minute${minutes === 1 ? '' : 's'}.`;
}

/** Plain-language text for a failed sign-in or registration. Branches on the error `code`. */
export function authErrorMessage(error: unknown): string {
  if (!(error instanceof ApiError)) return describeApiError(error);
  switch (error.code) {
    case 'invalid_credentials':
      return 'Email or password is incorrect.';
    case 'too_many_attempts':
      return `Too many sign-in attempts. ${waitText(error.retryAfterSeconds)}`;
    case 'too_many_registrations':
      return `Too many registration attempts. ${waitText(error.retryAfterSeconds)}`;
    case 'registration_closed':
      return 'Registration is closed. Ask the owner of this site for an account.';
    case 'email_taken':
      return 'An account with this email already exists. Try signing in instead.';
    case 'validation_failed':
      return error.message;
    default:
      return describeApiError(error);
  }
}
