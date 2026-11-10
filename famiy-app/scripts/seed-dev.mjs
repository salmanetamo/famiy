// Seeds the local dev account with a family through the real API (no database access).
//
//   node scripts/seed-dev.mjs            the messy PoC-2 family plus an incomplete-data family
//   node scripts/seed-dev.mjs --count N  about N synthetic people (for scale checks)
//
// Needs the API running (default http://localhost:8080, override with API_URL) and the dev account
// from famiy-app/.env.local (E2E_EMAIL, E2E_PASSWORD; see .env.example). It refuses to run when the
// account already has people, and never deletes anything. If it fails halfway the account is left
// with a partial family and the next run refuses; clear the dev account's people (for example by
// resetting the dev database) and run it again.
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const API_URL = process.env.API_URL ?? 'http://localhost:8080';

// The dev account's password is sent to this URL, so plain HTTP is only accepted for this machine.
const { hostname, protocol } = new URL(API_URL);
const isLocal = ['localhost', '127.0.0.1', '[::1]'].includes(hostname);
if (!isLocal && protocol !== 'https:' && process.env.ALLOW_INSECURE_API !== '1') {
  console.error(
    `Refusing to send credentials over plain HTTP to ${hostname}. Use https, or set ALLOW_INSECURE_API=1.`,
  );
  process.exit(1);
}

function loadEnv() {
  const env = { ...process.env };
  try {
    const file = fileURLToPath(new URL('../.env.local', import.meta.url));
    for (const line of readFileSync(file, 'utf8').split(/\r?\n/)) {
      const match = /^\s*([A-Z0-9_]+)\s*=\s*(.*)\s*$/.exec(line);
      if (match && env[match[1]] === undefined) env[match[1]] = match[2];
    }
  } catch {
    // no .env.local: rely on the real environment
  }
  return env;
}

const env = loadEnv();
if (!env.E2E_EMAIL || !env.E2E_PASSWORD) {
  console.error('Set E2E_EMAIL and E2E_PASSWORD (copy .env.example to .env.local).');
  process.exit(1);
}

let cookie = '';

async function api(method, path, body) {
  const headers = { Accept: 'application/json' };
  if (cookie) headers.Cookie = cookie;
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  const res = await fetch(`${API_URL}${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (path === '/auth/login' && res.ok) {
    const set = res.headers.getSetCookie()[0];
    cookie = set ? set.split(';')[0] : '';
  }
  const text = await res.text();
  const json = text ? JSON.parse(text) : undefined;
  if (!res.ok) {
    const detail = json?.error ? `${json.error.code}: ${json.error.message}` : text;
    throw new Error(`${method} ${path} -> ${String(res.status)} ${detail}`);
  }
  return json;
}

// Deterministic generator: founder couples, children who bring in a spouse, until the budget is used.
function synthetic(count) {
  let seed = 7;
  const random = () => (seed = (seed * 1664525 + 1013904223) >>> 0) / 4294967296;
  const persons = [];
  const parents = [];
  const partners = [];
  const make = (year) => {
    const key = `x${String(persons.length + 1)}`;
    persons.push({
      key,
      given_name: `Person ${String(persons.length + 1)}`,
      family_name: null,
      birth_year: year,
    });
    return { key, year };
  };
  const queue = [];
  while (persons.length < Math.max(2, count * 0.1)) {
    const a = make(1850 + Math.floor(random() * 10));
    const b = make(a.year + 2);
    partners.push([a.key, b.key, 'marriage']);
    queue.push([a, b]);
  }
  while (persons.length < count && queue.length > 0) {
    const [a, b] = queue.shift();
    const children = 1 + Math.floor(random() * 3);
    for (let i = 0; i < children && persons.length < count; i++) {
      const child = make(Math.max(a.year, b.year) + 25 + i * 2);
      parents.push([a.key, child.key], [b.key, child.key]);
      if (persons.length < count && random() < 0.8) {
        const spouse = make(child.year + Math.floor(random() * 6) - 3);
        partners.push([child.key, spouse.key, 'marriage']);
        queue.push([child, spouse]);
      }
    }
  }
  return { persons, parents, partners };
}

async function create(family) {
  const idOf = new Map();
  for (const p of family.persons) {
    const person = await api('POST', '/persons', {
      given_name: p.given_name,
      ...(p.family_name ? { family_name: p.family_name } : {}),
      ...(p.birth_year === null
        ? {}
        : { birth_date: `${String(p.birth_year)}-01-01`, birth_approx: true }),
    });
    idOf.set(p.key, person.id);
  }
  for (const [parent, child] of family.parents) {
    await api('POST', `/persons/${String(idOf.get(child))}/parents`, {
      parent_id: idOf.get(parent),
    });
  }
  for (const [a, b, kind] of family.partners) {
    await api('POST', `/persons/${String(idOf.get(a))}/partners`, {
      partner_id: idOf.get(b),
      kind,
    });
  }
  return family.persons.length;
}

const countFlag = process.argv.indexOf('--count');
const count = countFlag === -1 ? null : Number(process.argv[countFlag + 1]);
if (count !== null && (!Number.isInteger(count) || count < 2)) {
  console.error('--count needs a whole number of at least 2');
  process.exit(1);
}

await api('POST', '/auth/login', { email: env.E2E_EMAIL, password: env.E2E_PASSWORD });
const existing = await api('GET', '/graph');
if (existing.persons.length > 0) {
  console.error(
    `Refusing to seed: the account already has ${String(existing.persons.length)} people.`,
  );
  process.exit(1);
}

const started = Date.now();
let created = 0;
if (count === null) {
  const fixtures = JSON.parse(
    readFileSync(fileURLToPath(new URL('./fixtures/families.json', import.meta.url)), 'utf8'),
  );
  created += await create(fixtures.messy);
  created += await create(fixtures.edgeCases);
} else {
  created += await create(synthetic(count));
}
const graph = await api('GET', '/graph');
console.log(
  `Created ${String(created)} people in ${String(Date.now() - started)} ms; the account now has ` +
    `${String(graph.persons.length)} people, ${String(graph.parent_links.length)} parent links and ` +
    `${String(graph.partnerships.length)} partnerships.`,
);
