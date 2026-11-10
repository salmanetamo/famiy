import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ApiGraph } from '../api/types';
import { createQueryClient } from '../queryClient';
import { alice, errorResponse, jsonResponse, renderWithClient, stubFetch } from '../test-utils';
import { apiGraphFrom, emptyGraph, messy } from '../test/fixtures';
import { fakeServer, type Stored } from '../test/fakeServer';
import { createInlineEngine } from '../test/inlineEngine';
import { GraphScreen } from './GraphScreen';

afterEach(() => {
  vi.unstubAllGlobals();
});

const engine = createInlineEngine();
const getEngine = () => engine;

function setup(
  family: Parameters<typeof apiGraphFrom>[0] = messy,
  details: Record<number, Partial<Stored>> = {},
  api?: ApiGraph,
) {
  const { api: built, idOf } = apiGraphFrom(family);
  const server = fakeServer(api ?? built, details);
  const calls = stubFetch(server.handler);
  const client = createQueryClient();
  client.setQueryData(['session'], alice);
  renderWithClient(<GraphScreen getLayoutEngine={getEngine} />, client);
  const writes = () => calls.filter((c) => c.method !== 'GET');
  return { idOf, server, calls, writes, client };
}

const people = () => [...document.querySelectorAll<SVGGElement>('[data-person-id]')];
const node = (id: number) => {
  const found = document.querySelector<SVGGElement>(`[data-person-id="${String(id)}"]`);
  if (!found) throw new Error(`no node for ${String(id)}`);
  return found;
};
async function waitForPeople(count: number) {
  await waitFor(() => {
    expect(people()).toHaveLength(count);
  });
}
const panel = (name: string | RegExp) => screen.getByRole('region', { name });
const openAdd = () => userEvent.click(screen.getByRole('button', { name: 'Add person' }));
const typeInto = (label: string | RegExp, text: string) =>
  userEvent.type(screen.getByLabelText(label), text);

describe('adding a person', () => {
  it('opens a form focused on the given name, and does not send anything for a blank name', async () => {
    const { writes } = setup();
    await waitForPeople(12);
    await openAdd();
    const form = within(panel('Add a person'));
    expect(form.getByLabelText(/Given name/)).toHaveFocus();
    await userEvent.click(form.getByRole('button', { name: 'Add person' }));
    expect(form.getByText('Enter a given name.')).toBeInTheDocument();
    expect(form.getByLabelText(/Given name/)).toHaveAttribute('aria-invalid', 'true');
    expect(form.getByLabelText(/Given name/)).toHaveFocus();
    expect(writes()).toHaveLength(0);
  });

  it('takes a field error away as soon as the user starts fixing that field', async () => {
    setup();
    await waitForPeople(12);
    await openAdd();
    const form = within(panel('Add a person'));
    await userEvent.click(form.getByRole('button', { name: 'Add person' }));
    expect(form.getByText('Enter a given name.')).toBeInTheDocument();
    await userEvent.type(form.getByLabelText(/Given name/), 'N');
    expect(form.queryByText('Enter a given name.')).not.toBeInTheDocument();
    expect(form.getByLabelText(/Given name/)).not.toHaveAttribute('aria-invalid');
  });

  it('saves, shows the new person in the graph, selects them and moves focus to their card', async () => {
    const { writes } = setup();
    await waitForPeople(12);
    await openAdd();
    await typeInto(/Given name/, 'Nia');
    await typeInto(/Family name/, 'Traoré');
    fireEvent.change(screen.getByLabelText('Birth date'), { target: { value: '1990-05-04' } });
    await userEvent.click(
      within(panel('Add a person')).getByRole('button', { name: 'Add person' }),
    );

    await waitForPeople(13);
    expect(JSON.parse(writes()[0]?.body ?? '')).toMatchObject({
      given_name: 'Nia',
      family_name: 'Traoré',
      birth_date: '1990-05-04',
      birth_approx: false,
    });
    const card = screen.getByRole('button', { name: 'Nia Traoré, b. 1990' });
    expect(card).toHaveAttribute('aria-pressed', 'true');
    await waitFor(() => {
      expect(card).toHaveFocus();
    });
    expect(screen.queryByRole('region', { name: 'Add a person' })).not.toBeInTheDocument();
    expect(panel('Nia Traoré')).toBeInTheDocument();
  });

  it("shows the server's message and keeps what was typed when the save is refused", async () => {
    const { server, writes } = setup();
    await waitForPeople(12);
    server.intercept.current = (call) =>
      call.method === 'POST'
        ? errorResponse(400, 'validation_failed', 'birth_date must not be in the future')
        : undefined;
    await openAdd();
    await typeInto(/Given name/, 'Nia');
    await userEvent.click(
      within(panel('Add a person')).getByRole('button', { name: 'Add person' }),
    );
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'birth_date must not be in the future',
    );
    expect(screen.getByLabelText(/Given name/)).toHaveValue('Nia');
    expect(writes()).toHaveLength(1);
    expect(people()).toHaveLength(12);
  });

  it('explains the person limit', async () => {
    const { server } = setup();
    await waitForPeople(12);
    server.intercept.current = (call) =>
      call.method === 'POST'
        ? errorResponse(409, 'person_limit_reached', 'this account has reached its limit')
        : undefined;
    await openAdd();
    await typeInto(/Given name/, 'Nia');
    await userEvent.click(
      within(panel('Add a person')).getByRole('button', { name: 'Add person' }),
    );
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'this account has reached its limit',
    );
  });

  it('sends one request however often the button is pressed while saving', async () => {
    const { server, writes } = setup();
    await waitForPeople(12);
    let release: () => void = () => undefined;
    server.intercept.current = (call) =>
      call.method === 'POST'
        ? new Promise<Response>((resolve) => {
            release = () => {
              server.intercept.current = null;
              resolve(jsonResponse(201, { ...call, id: 2000 }));
            };
          })
        : undefined;
    await openAdd();
    await typeInto(/Given name/, 'Nia');
    const submit = within(panel('Add a person')).getByRole('button', { name: 'Add person' });
    await userEvent.click(submit);
    expect(await screen.findByRole('button', { name: 'Saving…' })).toHaveAttribute(
      'aria-disabled',
      'true',
    );
    const form = screen.getByLabelText(/Given name/).closest('form');
    if (!form) throw new Error('no form');
    fireEvent.submit(form); // as Enter in a field would, while the first save is still running
    // a second request would reach fetch a moment later, not synchronously, so give it the time
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(writes()).toHaveLength(1);
    release();
  });

  it('does not take over the screen when the user moved on while the save was running', async () => {
    const { server, idOf } = setup();
    await waitForPeople(12);
    let release: () => void = () => undefined;
    server.intercept.current = (call) =>
      call.method === 'POST'
        ? new Promise<Response>((resolve) => {
            release = () => {
              server.intercept.current = null;
              resolve(server.handler(call));
            };
          })
        : undefined;
    await openAdd();
    await typeInto(/Given name/, 'Nia');
    await userEvent.click(
      within(panel('Add a person')).getByRole('button', { name: 'Add person' }),
    );
    await screen.findByRole('button', { name: 'Saving…' });
    await userEvent.click(node(idOf.c1 ?? -1)); // choose someone else while it is saving
    release();
    await waitForPeople(13); // the person was added all the same
    expect(node(idOf.c1 ?? -1)).toHaveAttribute('aria-pressed', 'true');
    expect(panel('Ali Diallo')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Nia' })).toHaveAttribute('aria-pressed', 'false');
  });

  it('cancels back to the Add person button without sending anything', async () => {
    const { writes } = setup();
    await waitForPeople(12);
    await openAdd();
    await userEvent.click(within(panel('Add a person')).getByRole('button', { name: 'Cancel' }));
    expect(screen.queryByRole('region')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add person' })).toHaveFocus();
    expect(writes()).toHaveLength(0);
  });

  it('closes the form when a person in the graph is chosen instead', async () => {
    const { idOf } = setup();
    await waitForPeople(12);
    await openAdd();
    await userEvent.click(node(idOf.c1 ?? -1));
    expect(screen.queryByRole('region', { name: 'Add a person' })).not.toBeInTheDocument();
    expect(panel('Ali Diallo')).toBeInTheDocument();
  });

  it('starts a tree from an empty account', async () => {
    setup({ persons: [], parents: [], partners: [] }, {}, emptyGraph());
    expect(await screen.findByText(/your tree is empty/i)).toBeInTheDocument();
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
    await openAdd();
    await typeInto(/Given name/, 'Nia');
    await userEvent.click(
      within(panel('Add a person')).getByRole('button', { name: 'Add person' }),
    );
    await waitForPeople(1);
    expect(screen.queryByText(/your tree is empty/i)).not.toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Nia' })).toHaveFocus();
    });
  });
});

describe('editing a person', () => {
  const detail = { bio: 'Raised in Kayes.', residence: 'Bamako' };

  async function openEdit(personKey = 'c1') {
    const ctx = setup(messy, {
      // detail for every fixture person is the same; the tests only open one
      ...Object.fromEntries([...Array(30).keys()].map((i) => [i + 1, detail])),
    });
    await waitForPeople(12);
    await userEvent.click(node(ctx.idOf[personKey] ?? -1));
    const edit = screen.getByRole('button', { name: 'Edit' });
    await waitFor(() => {
      expect(edit).toBeEnabled();
    });
    await userEvent.click(edit);
    return ctx;
  }

  it('cannot start before the notes and residence have arrived', async () => {
    const { idOf, server } = setup();
    await waitForPeople(12);
    server.intercept.current = (call) =>
      call.method === 'GET' && call.path.includes('/persons/')
        ? new Promise<Response>(() => undefined)
        : undefined;
    await userEvent.click(node(idOf.c1 ?? -1));
    expect(screen.getByRole('button', { name: 'Edit' })).toBeDisabled();
  });

  it('opens a form filled with the stored values', async () => {
    await openEdit();
    expect(screen.getByLabelText(/Given name/)).toHaveValue('Ali');
    expect(screen.getByLabelText(/Family name/)).toHaveValue('Diallo');
    expect(screen.getByLabelText('Birth date')).toHaveValue('1982-01-01');
    expect(screen.getByLabelText(/approximate/)).toBeChecked();
    expect(screen.getByLabelText('Residence')).toHaveValue('Bamako');
    expect(screen.getByLabelText('Notes')).toHaveValue('Raised in Kayes.');
  });

  it('sends only what changed, then shows the new name in the panel and on the card', async () => {
    const { writes, idOf } = await openEdit();
    const given = screen.getByLabelText(/Given name/);
    await userEvent.clear(given);
    await userEvent.type(given, 'Alassane');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(panel('Alassane Diallo')).toBeInTheDocument();
    });
    expect(writes()).toHaveLength(1);
    expect(writes()[0]).toMatchObject({ method: 'PATCH', path: `/api/persons/${String(idOf.c1)}` });
    expect(JSON.parse(writes()[0]?.body ?? '')).toEqual({ given_name: 'Alassane' });
    expect(node(idOf.c1 ?? -1)).toHaveAccessibleName('Alassane Diallo, c. 1982');
    expect(screen.queryByLabelText(/Given name/)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Edit' })).toHaveFocus();
  });

  it('shows the new residence in the panel straight after saving, not the old cached one', async () => {
    await openEdit();
    const residence = screen.getByLabelText('Residence');
    await userEvent.clear(residence);
    await userEvent.type(residence, 'Dakar');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('Dakar')).toBeInTheDocument();
    expect(screen.queryByText('Bamako')).not.toBeInTheDocument();
  });

  it('clears an optional field by sending null', async () => {
    const { writes } = await openEdit();
    await userEvent.clear(screen.getByLabelText('Residence'));
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => {
      expect(screen.queryByLabelText('Residence')).not.toBeInTheDocument();
    });
    expect(JSON.parse(writes()[0]?.body ?? '')).toEqual({ residence: null });
  });

  it('sends nothing when nothing changed', async () => {
    const { writes } = await openEdit();
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(screen.queryByLabelText(/Given name/)).not.toBeInTheDocument();
    expect(writes()).toHaveLength(0);
  });

  it('keeps the form and what was typed when the save is refused', async () => {
    const { server, writes } = await openEdit();
    server.intercept.current = (call) =>
      call.method === 'PATCH'
        ? errorResponse(400, 'validation_failed', 'death_date is before birth_date')
        : undefined;
    await userEvent.type(screen.getByLabelText(/Given name/), 'X');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('death_date is before birth_date');
    expect(screen.getByLabelText(/Given name/)).toHaveValue('AliX');
    expect(writes()).toHaveLength(1);
  });

  it('discards the changes on Cancel and returns focus to Edit', async () => {
    const { writes } = await openEdit();
    await userEvent.type(screen.getByLabelText(/Given name/), 'X');
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('heading', { name: 'Ali Diallo' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Edit' })).toHaveFocus();
    expect(writes()).toHaveLength(0);
    await userEvent.click(screen.getByRole('button', { name: 'Edit' }));
    expect(screen.getByLabelText(/Given name/)).toHaveValue('Ali');
  });

  it('checks the dates before sending', async () => {
    const { writes } = await openEdit();
    fireEvent.change(screen.getByLabelText('Death date'), { target: { value: '1900-01-01' } });
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(screen.getByText(/death date cannot be before/i)).toBeInTheDocument();
    expect(writes()).toHaveLength(0);
  });

  it('compares against what the form was opened with, not data that changed underneath it', async () => {
    const { server, client, writes, idOf } = await openEdit();
    const stored = server.store.get(idOf.c1 ?? -1);
    if (!stored) throw new Error('no stored person');
    stored.residence = 'Dakar'; // changed on the server while the form is open
    await client.refetchQueries({ queryKey: ['person', idOf.c1] });
    await userEvent.type(screen.getByLabelText(/Given name/), 'X');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => {
      expect(writes()).toHaveLength(1);
    });
    // the residence field was not touched, so it must not be sent (it would undo the other change)
    expect(JSON.parse(writes()[0]?.body ?? '')).toEqual({ given_name: 'AliX' });
  });

  it('closes the form with Escape and discards the changes', async () => {
    const { writes } = await openEdit();
    await userEvent.type(screen.getByLabelText(/Given name/), 'X');
    await userEvent.keyboard('{Escape}');
    expect(screen.queryByLabelText(/Given name/)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Edit' })).toHaveFocus();
    expect(writes()).toHaveLength(0);
  });

  it('refreshes the tree when the person was already deleted elsewhere', async () => {
    const { server, idOf } = await openEdit();
    server.intercept.current = (call) => {
      if (call.method !== 'PATCH') return undefined;
      server.store.delete(idOf.c1 ?? -1);
      return errorResponse(404, 'person_not_found');
    };
    await userEvent.type(screen.getByLabelText(/Given name/), 'X');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitForPeople(11);
    expect(screen.queryByRole('button', { name: /Ali Diallo/ })).not.toBeInTheDocument();
  });

  it('ends the session when a save says the user is signed out', async () => {
    const { server, client } = await openEdit();
    server.intercept.current = (call) =>
      call.method === 'PATCH' ? errorResponse(401, 'unauthenticated') : undefined;
    await userEvent.type(screen.getByLabelText(/Given name/), 'X');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => {
      expect(client.getQueryData(['session'])).toBeNull();
    });
  });
});

describe('deleting a person', () => {
  async function askToDelete(key = 'd1') {
    const ctx = setup();
    await waitForPeople(12);
    await userEvent.click(node(ctx.idOf[key] ?? -1));
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }));
    return ctx;
  }

  it('asks first, says what goes with the person, and puts focus on the safe choice', async () => {
    const { writes } = await askToDelete();
    const dialog = screen.getByRole('alertdialog');
    expect(dialog).toHaveTextContent('Delete Umar Diallo permanently?');
    expect(dialog).toHaveTextContent(/links to parents, children and partners/i);
    expect(dialog).toHaveTextContent(/cannot be undone/i);
    expect(within(dialog).getByRole('button', { name: 'Cancel' })).toHaveFocus();
    expect(writes()).toHaveLength(0);
  });

  it('Cancel changes nothing and returns focus to Delete', async () => {
    const { writes } = await askToDelete();
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete' })).toHaveFocus();
    expect(writes()).toHaveLength(0);
    expect(people()).toHaveLength(12);
  });

  it('deletes, removes them from the graph, closes the panel and moves focus to the search box', async () => {
    const { writes, idOf } = await askToDelete();
    await userEvent.click(screen.getByRole('button', { name: 'Delete permanently' }));
    await waitForPeople(11);
    expect(writes()).toHaveLength(1);
    expect(writes()[0]).toMatchObject({
      method: 'DELETE',
      path: `/api/persons/${String(idOf.d1)}`,
    });
    expect(screen.queryByRole('region')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Umar Diallo/ })).not.toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByRole('combobox')).toHaveFocus();
    });
  });

  it('stays open with the reason when the server refuses', async () => {
    const { server } = await askToDelete();
    server.intercept.current = (call) =>
      call.method === 'DELETE' ? errorResponse(500, 'internal', 'boom') : undefined;
    await userEvent.click(screen.getByRole('button', { name: 'Delete permanently' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      /something went wrong \(internal\)/i,
    );
    expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    expect(people()).toHaveLength(12);
  });

  it('sends one delete however often Delete permanently is pressed while it runs', async () => {
    const { server, writes } = await askToDelete();
    let release: () => void = () => undefined;
    server.intercept.current = (call) =>
      call.method === 'DELETE'
        ? new Promise<Response>((resolve) => {
            release = () => {
              server.intercept.current = null;
              resolve(server.handler(call));
            };
          })
        : undefined;
    const button = screen.getByRole('button', { name: 'Delete permanently' });
    await userEvent.click(button);
    fireEvent.click(await screen.findByRole('button', { name: 'Deleting…' }));
    await new Promise((resolve) => setTimeout(resolve, 50)); // a second request would arrive later
    expect(writes()).toHaveLength(1);
    release();
    await waitForPeople(11);
  });

  it('closes the confirmation with Escape, without deleting, and returns focus to Delete', async () => {
    const { writes } = await askToDelete();
    await userEvent.keyboard('{Escape}');
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete' })).toHaveFocus();
    expect(writes()).toHaveLength(0);
  });

  it('refreshes the tree when the person was already deleted elsewhere', async () => {
    const { server, idOf } = await askToDelete();
    server.intercept.current = (call) => {
      if (call.method !== 'DELETE') return undefined;
      server.store.delete(idOf.d1 ?? -1); // someone else deleted them first
      return errorResponse(404, 'person_not_found');
    };
    await userEvent.click(screen.getByRole('button', { name: 'Delete permanently' }));
    await waitForPeople(11);
    expect(screen.queryByRole('button', { name: /Umar Diallo/ })).not.toBeInTheDocument();
  });

  it('does not deselect or move focus when the user moved on while the delete was running', async () => {
    const { server, idOf } = await askToDelete();
    let release: () => void = () => undefined;
    server.intercept.current = (call) =>
      call.method === 'DELETE'
        ? new Promise<Response>((resolve) => {
            release = () => {
              server.intercept.current = null;
              resolve(server.handler(call));
            };
          })
        : undefined;
    await userEvent.click(screen.getByRole('button', { name: 'Delete permanently' }));
    await userEvent.click(node(idOf.c2 ?? -1)); // choose someone else while it is running
    release();
    await waitForPeople(11);
    expect(node(idOf.c2 ?? -1)).toHaveAttribute('aria-pressed', 'true');
    expect(panel('Hawa Diallo')).toBeInTheDocument();
    expect(screen.getByRole('combobox')).not.toHaveFocus();
  });

  it('forgets the cached details of a deleted person', async () => {
    const { client, idOf } = await askToDelete();
    await userEvent.click(screen.getByRole('button', { name: 'Delete permanently' }));
    await waitForPeople(11);
    await waitFor(() => {
      expect(client.getQueryData(['person', idOf.d1])).toBeUndefined();
    });
  });

  it('does not keep the confirmation when another person is chosen', async () => {
    const { idOf } = await askToDelete();
    await userEvent.click(node(idOf.c2 ?? -1));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    expect(panel('Hawa Diallo')).toBeInTheDocument();
  });
});
