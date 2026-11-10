import { randomBytes } from 'node:crypto';
import { appendFileSync, existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import type { APIRequestContext } from '@playwright/test';

const ENV_FILE = join(import.meta.dirname, '..', '.env.local');
const EMAIL_KEY = 'E2E_TEST_EMAIL';
const PASSWORD_KEY = 'E2E_TEST_PASSWORD';

export interface Account {
  email: string;
  password: string;
}

function readEnvFile(): Record<string, string> {
  if (!existsSync(ENV_FILE)) return {};
  const entries = readFileSync(ENV_FILE, 'utf8')
    .split(/\r?\n/)
    .filter((line) => line.includes('=') && !line.trimStart().startsWith('#'))
    .map((line) => {
      const at = line.indexOf('=');
      return [line.slice(0, at).trim(), line.slice(at + 1).trim()] as const;
    });
  return Object.fromEntries(entries);
}

/**
 * The account the smoke test signs in as. It is dedicated to the test (never the dev account, which
 * holds seeded data) and is kept in the gitignored `.env.local`. The first run creates it: the API has
 * no way to delete an account and caps how many exist (MAX_USERS), so a new one per run would use
 * them up. The test removes the people it adds, so the account is empty between runs.
 */
export async function ensureAccount(api: APIRequestContext): Promise<Account> {
  const env = readEnvFile();
  const email = env[EMAIL_KEY];
  const password = env[PASSWORD_KEY];
  if (email && password) return { email, password };

  const created: Account = {
    email: `e2e-${randomBytes(4).toString('hex')}@famiy.local`,
    password: randomBytes(18).toString('base64url'),
  };
  const response = await api.post('/api/auth/register', { data: created });
  if (!response.ok()) {
    throw new Error(
      `Could not create the e2e account (HTTP ${String(response.status())}: ${await response.text()}). ` +
        `If registration is closed or the account limit is reached, put an existing empty account's ` +
        `${EMAIL_KEY} and ${PASSWORD_KEY} in famiy-app/.env.local.`,
    );
  }
  appendFileSync(
    ENV_FILE,
    `\n${EMAIL_KEY}=${created.email}\n${PASSWORD_KEY}=${created.password}\n`,
  );
  return created;
}

/** Fails with a plain explanation when the API is not running, instead of a wall of timeouts. */
export async function assertApiIsUp(api: APIRequestContext): Promise<void> {
  const response = await api.get('/api/healthz').catch(() => null);
  if (!response?.ok()) {
    throw new Error(
      'The API is not reachable through the dev server (GET /api/healthz). Start the API and its ' +
        'database first, then run the e2e test again.',
    );
  }
}

/** Signs the API context in as the account and removes every person, so the test starts empty. */
export async function signInAndEmpty(api: APIRequestContext, account: Account): Promise<void> {
  const login = await api.post('/api/auth/login', { data: account });
  if (!login.ok())
    throw new Error(`Could not sign in as the e2e account (HTTP ${String(login.status())}).`);
  const graph = (await (await api.get('/api/graph')).json()) as { persons: { id: number }[] };
  for (const person of graph.persons) {
    const removed = await api.delete(`/api/persons/${String(person.id)}`);
    if (!removed.ok()) throw new Error(`Could not remove leftover person ${String(person.id)}.`);
  }
}
