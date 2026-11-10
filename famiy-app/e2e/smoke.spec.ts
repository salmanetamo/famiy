import { expect, test, type Page } from '@playwright/test';
import { assertApiIsUp, ensureAccount, signInAndEmpty, type Account } from './support';

/**
 * One walk through the product, against the real API and database: sign in, add people, link them,
 * edit, search, reload to prove the server holds it all, then delete everything it created.
 */
let account: Account;

test.beforeAll(async ({ playwright, baseURL }) => {
  const setup = await playwright.request.newContext({ baseURL: baseURL ?? '' });
  await assertApiIsUp(setup);
  account = await ensureAccount(setup);
  await setup.dispose();
  // A fresh context for signing in: reusing the one that just registered hung up the connection
  // (ECONNRESET through the dev proxy on the first run).
  const api = await playwright.request.newContext({ baseURL: baseURL ?? '' });
  await signInAndEmpty(api, account); // a failed earlier run must not leave people behind
  await api.dispose();
});

const graph = (page: Page) =>
  page.request.get('/api/graph').then(
    (response) =>
      response.json() as Promise<{
        persons: { id: number; given_name: string }[];
        parent_links: { parent_id: number; child_id: number }[];
        partnerships: { partners: number[]; kind: string }[];
      }>,
  );

/** A person's card in the graph (the panel also has buttons with the same names for relatives). */
const card = (page: Page, name: string) => page.locator(`[data-person-id][aria-label^="${name}"]`);

async function addPerson(page: Page, fields: { given: string; family?: string; born?: string }) {
  await page.getByRole('button', { name: 'Add person' }).first().click();
  const form = page.getByRole('region', { name: 'Add a person' });
  await form.getByLabel(/Given name/).fill(fields.given);
  if (fields.family) await form.getByLabel('Family name').fill(fields.family);
  if (fields.born) await form.getByLabel('Birth date', { exact: true }).fill(fields.born);
  await form.getByRole('button', { name: 'Add person' }).click();
  await expect(page.getByRole('region', { name: new RegExp(fields.given) })).toBeVisible();
}

test('sign in, add, link, edit, search, reload, delete', async ({ page }) => {
  const problems: string[] = [];
  page.on('pageerror', (error) => problems.push(`pageerror: ${error.message}`));
  page.on('console', (message) => {
    // the first session check answers 401 before sign-in, which the browser logs; that is expected
    if (message.type() === 'error' && !message.text().includes('401')) {
      problems.push(`console: ${message.text()}`);
    }
  });

  await test.step('sign in to an empty tree', async () => {
    await page.goto('/');
    await page.getByLabel('Email').fill(account.email);
    await page.getByLabel('Password').fill(account.password);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.getByText(/your tree is empty/i)).toBeVisible();
  });

  await test.step('add two people', async () => {
    await addPerson(page, { given: 'Ada', family: 'Lovelace', born: '1815-12-10' });
    await addPerson(page, { given: 'Byron', family: 'Gordon', born: '1788-01-22' });
    await expect(page.locator('[data-person-id]')).toHaveCount(2);
  });

  await test.step('link Byron as Ada’s parent through the picker', async () => {
    await card(page, 'Ada Lovelace').click();
    const panel = page.getByRole('region', { name: 'Ada Lovelace' });
    await panel.getByRole('button', { name: 'Add parent' }).click();
    await panel.getByRole('combobox', { name: 'Find a parent' }).fill('byron');
    await panel.getByRole('option', { name: /Byron Gordon/ }).click();
    await expect(
      panel.getByRole('button', { name: 'Remove Byron Gordon as a parent of Ada Lovelace' }),
    ).toBeVisible();
  });

  await test.step('add a new person as Ada’s partner, then change the kind', async () => {
    const panel = page.getByRole('region', { name: 'Ada Lovelace' });
    await panel.getByRole('button', { name: 'Add partner' }).click();
    await panel.getByRole('button', { name: /Add a new person as partner/ }).click();
    const form = page.getByRole('region', { name: 'Add a partner of Ada Lovelace' });
    await form.getByLabel(/Given name/).fill('William');
    await form.getByLabel('Family name').fill('King');
    await form.getByRole('button', { name: 'Add person' }).click();
    await expect(page.getByRole('region', { name: 'William King' })).toBeVisible();

    await card(page, 'Ada Lovelace').click();
    const kind = page
      .getByRole('region', { name: 'Ada Lovelace' })
      .getByLabel('Kind of partnership with William King');
    await expect(kind).toHaveValue('marriage');
    await kind.selectOption('partnership');
    await expect(kind).toHaveValue('partnership');
  });

  await test.step('edit Ada', async () => {
    const panel = page.getByRole('region', { name: 'Ada Lovelace' });
    await expect(panel.getByRole('button', { name: 'Edit' })).toBeEnabled();
    await panel.getByRole('button', { name: 'Edit' }).click();
    await panel.getByLabel('Residence').fill('London');
    await panel.getByLabel('Notes').fill('Wrote the first program.');
    await panel.getByRole('button', { name: 'Save' }).click();
    await expect(panel.getByText('London')).toBeVisible();
  });

  await test.step('reload: the server holds everything', async () => {
    await page.reload();
    await expect(page.locator('[data-person-id]')).toHaveCount(3);
    const state = await graph(page);
    expect(state.persons.map((p) => p.given_name).sort()).toEqual(['Ada', 'Byron', 'William']);
    const id = (name: string) => state.persons.find((p) => p.given_name === name)?.id;
    expect(state.parent_links).toEqual([{ parent_id: id('Byron'), child_id: id('Ada') }]);
    expect(state.partnerships).toHaveLength(1);
    expect(state.partnerships[0]?.kind).toBe('partnership');

    await card(page, 'Ada Lovelace').click();
    const panel = page.getByRole('region', { name: 'Ada Lovelace' });
    await expect(panel.getByText('London')).toBeVisible();
    await expect(panel.getByText('Wrote the first program.')).toBeVisible();
  });

  await test.step('find Byron by name and see Ada as his child', async () => {
    await page.getByRole('combobox', { name: 'Find a person' }).fill('byron');
    await page.getByRole('option', { name: /Byron Gordon/ }).click();
    const panel = page.getByRole('region', { name: 'Byron Gordon' });
    await expect(
      panel.getByRole('button', { name: 'Remove Ada Lovelace as a child of Byron Gordon' }),
    ).toBeVisible();
  });

  await test.step('delete everyone the test added', async () => {
    for (const name of ['William King', 'Byron Gordon', 'Ada Lovelace']) {
      await card(page, name).click();
      const panel = page.getByRole('region', { name });
      await panel.getByRole('button', { name: 'Delete', exact: true }).click();
      await panel.getByRole('button', { name: 'Delete permanently' }).click();
      await expect(card(page, name)).toHaveCount(0);
    }
    await expect(page.getByText(/your tree is empty/i)).toBeVisible();
    expect((await graph(page)).persons).toHaveLength(0);
  });

  expect(problems).toEqual([]);
});
