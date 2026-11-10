import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { errorResponse } from '../test-utils';
import { node, panel, people, setupGraph, waitForPeople } from '../test/graphHarness';

afterEach(() => {
  vi.unstubAllGlobals();
});

/*
 * The messy family: Abou+Fati (g1,g2) → Idi (p1), Nana (p2); Idi's partners are Rama (s1) and Zara
 * (s2), Nana's is Moussa (s3); Ali (c1) and Hawa (c2) are Idi+Rama's, Bina (c3) is Idi+Zara's, Sala (c4)
 * is Nana+Moussa's; Umar (d1) is Ali+Sala's. People with a parent have two.
 */

async function open(key: string) {
  const ctx = setupGraph();
  await waitForPeople(12);
  await userEvent.click(node(ctx.idOf[key] ?? -1));
  return ctx;
}
const inPanel = (name: string) => within(panel(name));
const button = (name: string) => screen.getByRole('button', { name });
/** The open picker (the graph and the kind <select> also contain groups and options). */
const picker = (role: string) =>
  within(screen.getByRole('group', { name: new RegExp(`Add ${role} for`) }));
const path = (call: { path: string } | undefined) => call?.path.replace('/api', '');
const body = (call: { body: string | undefined } | undefined): unknown =>
  JSON.parse(call?.body ?? 'null');

/** Pending write that the test lets go of later, to check what happens while it runs. */
function holdWrite(
  server: ReturnType<typeof setupGraph>['server'],
  method: string,
): { release: () => void } {
  const held = { release: (): void => undefined };
  server.intercept.current = (call) =>
    call.method === method
      ? new Promise<Response>((resolve) => {
          held.release = () => {
            server.intercept.current = null;
            resolve(server.handler(call));
          };
        })
      : undefined;
  return held;
}

describe('the relationship lists', () => {
  it('shows parents, partners and children, each with the actions that change them', async () => {
    await open('p1');
    const idi = inPanel('Idi Diallo');
    expect(
      idi.getByRole('button', { name: 'Remove Abou Diallo as a parent of Idi Diallo' }),
    ).toBeInTheDocument();
    expect(
      idi.getByRole('button', { name: 'Remove Fati Diallo as a parent of Idi Diallo' }),
    ).toBeInTheDocument();
    expect(idi.getByLabelText('Kind of partnership with Rama')).toHaveValue('marriage');
    expect(idi.getByRole('button', { name: 'Remove partnership with Zara' })).toBeInTheDocument();
    expect(
      idi.getByRole('button', { name: 'Remove Hawa Diallo as a child of Idi Diallo' }),
    ).toBeInTheDocument();
    expect(idi.getByRole('button', { name: 'Add partner' })).toBeInTheDocument();
    expect(idi.getByRole('button', { name: 'Add child' })).toBeInTheDocument();
  });

  it('offers "Add parent" only while there is room for a parent', async () => {
    await open('p1'); // has two parents
    expect(screen.queryByRole('button', { name: 'Add parent' })).not.toBeInTheDocument();
    await userEvent.click(node(1)); // Abou: none
    expect(screen.getByRole('button', { name: 'Add parent' })).toBeInTheDocument();
  });

  it('says "None recorded" for an empty list', async () => {
    const ctx = setupGraph({
      persons: [{ key: 'a', given_name: 'Solo', family_name: null, birth_year: null }],
      parents: [],
      partners: [],
    });
    await waitForPeople(1);
    await userEvent.click(node(ctx.idOf.a ?? -1));
    expect(within(panel('Solo')).getAllByText('None recorded')).toHaveLength(3);
  });

  it('shows a partner who only shares a child without a kind or a remove button', async () => {
    const ctx = setupGraph({
      persons: [
        { key: 'a', given_name: 'Ana', family_name: null, birth_year: 1960 },
        { key: 'b', given_name: 'Ben', family_name: null, birth_year: 1961 },
        { key: 'c', given_name: 'Cy', family_name: null, birth_year: 1990 },
      ],
      parents: [
        ['a', 'c'],
        ['b', 'c'],
      ],
      partners: [],
    });
    await waitForPeople(3);
    await userEvent.click(node(ctx.idOf.a ?? -1));
    const ana = within(panel('Ana'));
    expect(ana.getByText('(shared child)')).toBeInTheDocument();
    expect(ana.queryByLabelText(/Kind of partnership/)).not.toBeInTheDocument();
    expect(ana.queryByRole('button', { name: /Remove partnership/ })).not.toBeInTheDocument();
  });
});

describe('adding a parent or a child', () => {
  it('links an existing person as a parent, closes the picker and moves focus to the name', async () => {
    const { idOf, writes } = await open('g1');
    await userEvent.click(button('Add parent'));
    const picker = screen.getByRole('group', { name: 'Add parent for Abou Diallo' });
    expect(within(picker).getByRole('combobox', { name: 'Find a parent' })).toHaveFocus();
    await userEvent.type(within(picker).getByRole('combobox'), 'fati');
    await userEvent.click(within(picker).getByRole('option', { name: 'Fati Diallo, c. 1934' }));

    await waitFor(() => {
      expect(
        inPanel('Abou Diallo').getByRole('button', {
          name: 'Remove Fati Diallo as a parent of Abou Diallo',
        }),
      ).toBeInTheDocument();
    });
    expect(writes()).toHaveLength(1);
    expect(writes()[0]?.method).toBe('POST');
    expect(path(writes()[0])).toBe(`/persons/${String(idOf.g1)}/parents`);
    expect(body(writes()[0])).toEqual({ parent_id: idOf.g2 });
    expect(screen.queryByRole('group', { name: /Add parent for/ })).not.toBeInTheDocument();
    expect(inPanel('Abou Diallo').getByRole('heading', { level: 2 })).toHaveFocus();
  });

  it('offers only people the server would accept as a parent', async () => {
    await open('g1');
    await userEvent.click(button('Add parent'));
    const box = screen.getByRole('combobox', { name: 'Find a parent' });
    await userEvent.type(box, 'idi'); // a child of Abou: that would be a cycle
    expect(screen.getByText(/No one matches/)).toBeInTheDocument();
    await userEvent.clear(box);
    await userEvent.type(box, 'abou'); // himself
    expect(screen.getByText(/No one matches/)).toBeInTheDocument();
    await userEvent.clear(box);
    await userEvent.type(box, 'fati');
    expect(picker('parent').getAllByRole('option')).toHaveLength(1);
  });

  it('links an existing person as a child, from the parent side', async () => {
    const { idOf, writes } = await open('s2'); // Zara
    await userEvent.click(button('Add child'));
    const box = screen.getByRole('combobox', { name: 'Find a child' });
    await userEvent.type(box, 'ali'); // already has two parents: not offered
    expect(screen.getByText(/No one matches/)).toBeInTheDocument();
    await userEvent.clear(box);
    await userEvent.type(box, 'moussa');
    await userEvent.click(screen.getByRole('option', { name: 'Moussa, c. 1959' }));
    await waitFor(() => {
      expect(
        inPanel('Zara').getByRole('button', { name: 'Remove Moussa as a child of Zara' }),
      ).toBeInTheDocument();
    });
    expect(path(writes()[0])).toBe(`/persons/${String(idOf.s3)}/parents`);
    expect(body(writes()[0])).toEqual({ parent_id: idOf.s2 });
  });

  it('ignores "Add a new person", Cancel and Escape while a write is running', async () => {
    const { server } = await open('g1');
    const held = holdWrite(server, 'POST');
    await userEvent.click(button('Add parent'));
    await userEvent.type(screen.getByRole('combobox', { name: 'Find a parent' }), 'fati');
    await userEvent.click(picker('parent').getByRole('option'));
    await userEvent.click(button('Add a new person as parent…'));
    expect(screen.queryByRole('region', { name: /Add a parent of/ })).not.toBeInTheDocument();
    await userEvent.click(picker('parent').getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('group', { name: /Add parent for/ })).toBeInTheDocument();
    await userEvent.type(screen.getByRole('combobox', { name: 'Find a parent' }), '{Escape}');
    expect(screen.getByRole('group', { name: /Add parent for/ })).toBeInTheDocument();
    held.release();
    await waitFor(() => {
      expect(screen.queryByRole('group', { name: /Add parent for/ })).not.toBeInTheDocument();
    });
  });

  it('shows the server’s refusal and keeps the picker open', async () => {
    const { server, writes } = await open('g1');
    server.intercept.current = (call) =>
      call.method === 'POST' ? errorResponse(409, 'too_many_parents') : undefined;
    await userEvent.click(button('Add parent'));
    await userEvent.type(screen.getByRole('combobox', { name: 'Find a parent' }), 'fati');
    await userEvent.click(picker('parent').getByRole('option'));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'That person already has two parents.',
    );
    expect(screen.getByRole('group', { name: 'Add parent for Abou Diallo' })).toBeInTheDocument();
    expect(writes()).toHaveLength(1);
  });

  it('announces the same refusal again when it happens twice in a row', async () => {
    const { server } = await open('g1');
    server.intercept.current = (call) =>
      call.method === 'POST' ? errorResponse(409, 'too_many_parents') : undefined;
    await userEvent.click(button('Add parent'));
    const box = screen.getByRole('combobox', { name: 'Find a parent' });
    await userEvent.type(box, 'fati');
    await userEvent.click(picker('parent').getByRole('option'));
    const first = await screen.findByRole('alert');
    await userEvent.type(box, 'fati');
    await userEvent.click(picker('parent').getByRole('option'));
    await waitFor(() => {
      expect(screen.getByRole('alert')).not.toBe(first); // a new element, so it is read out again
    });
    expect(screen.getByRole('alert')).toHaveTextContent('That person already has two parents.');
  });

  it('closes the picker with Escape (after the list and the text), and returns focus to the button', async () => {
    await open('g1');
    await userEvent.click(button('Add parent'));
    await userEvent.type(screen.getByRole('combobox', { name: 'Find a parent' }), 'fa');
    await userEvent.keyboard('{Escape}'); // closes the list
    expect(screen.getByRole('group', { name: /Add parent for/ })).toBeInTheDocument();
    await userEvent.keyboard('{Escape}'); // clears the text
    expect(screen.getByRole('combobox', { name: 'Find a parent' })).toHaveValue('');
    await userEvent.keyboard('{Escape}'); // nothing left: closes the picker
    expect(screen.queryByRole('group', { name: /Add parent for/ })).not.toBeInTheDocument();
    expect(button('Add parent')).toHaveFocus();
  });

  it('closes the picker with Escape from any control inside it, not only the search box', async () => {
    await open('c2');
    await userEvent.click(button('Add partner'));
    screen.getByLabelText('Kind of partnership').focus();
    await userEvent.keyboard('{Escape}');
    expect(screen.queryByRole('group', { name: /Add partner for/ })).not.toBeInTheDocument();
    expect(button('Add partner')).toHaveFocus();
  });

  it('Cancel closes the picker without writing, and returns focus to the button', async () => {
    const { writes } = await open('g1');
    await userEvent.click(button('Add parent'));
    await userEvent.click(picker('parent').getByRole('button', { name: 'Cancel' }));
    expect(screen.queryByRole('group', { name: /Add parent for/ })).not.toBeInTheDocument();
    expect(button('Add parent')).toHaveFocus();
    expect(writes()).toHaveLength(0);
  });

  it('says so when no one can be chosen', async () => {
    const ctx = setupGraph({
      persons: [{ key: 'a', given_name: 'Solo', family_name: null, birth_year: null }],
      parents: [],
      partners: [],
    });
    await waitForPeople(1);
    await userEvent.click(node(ctx.idOf.a ?? -1));
    await userEvent.click(button('Add parent'));
    expect(screen.getByText(/No one else in the tree can be added here/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Add a new person as parent/ })).toBeInTheDocument();
  });
});

describe('removing a parent or a child', () => {
  it('removes a parent link, and the Add parent button comes back', async () => {
    const { idOf, writes } = await open('c3'); // Bina: Idi and Zara
    await userEvent.click(button('Remove Zara as a parent of Bina Diallo'));
    await waitFor(() => {
      expect(
        screen.queryByRole('button', { name: 'Remove Zara as a parent of Bina Diallo' }),
      ).not.toBeInTheDocument();
    });
    expect(writes()[0]?.method).toBe('DELETE');
    expect(path(writes()[0])).toBe(`/persons/${String(idOf.c3)}/parents/${String(idOf.s2)}`);
    expect(button('Add parent')).toBeInTheDocument();
    expect(inPanel('Bina Diallo').getByRole('heading', { level: 2 })).toHaveFocus();
  });

  it('removes a child with the same call, from the child’s side', async () => {
    const { idOf, writes } = await open('p1');
    await userEvent.click(button('Remove Hawa Diallo as a child of Idi Diallo'));
    await waitFor(() => {
      expect(
        screen.queryByRole('button', { name: /Remove Hawa Diallo as a child/ }),
      ).not.toBeInTheDocument();
    });
    expect(path(writes()[0])).toBe(`/persons/${String(idOf.c2)}/parents/${String(idOf.p1)}`);
  });

  it('refreshes the tree when the link was already gone', async () => {
    const { server, idOf } = await open('c3');
    server.intercept.current = (call) => {
      if (call.method !== 'DELETE') return undefined;
      const at = server.links.findIndex((l) => l.child_id === idOf.c3 && l.parent_id === idOf.s2);
      server.links.splice(at, 1); // someone else removed it first
      return errorResponse(404, 'parent_link_not_found');
    };
    await userEvent.click(button('Remove Zara as a parent of Bina Diallo'));
    expect(await screen.findByRole('alert')).toHaveTextContent(/no longer exists/i);
    await waitFor(() => {
      expect(
        screen.queryByRole('button', { name: 'Remove Zara as a parent of Bina Diallo' }),
      ).not.toBeInTheDocument();
    });
  });

  it('sends one request however often Remove is pressed while it runs', async () => {
    const { server, writes } = await open('c3');
    const held = holdWrite(server, 'DELETE');
    const remove = button('Remove Zara as a parent of Bina Diallo');
    await userEvent.click(remove);
    fireEvent.click(remove);
    await new Promise((resolve) => setTimeout(resolve, 50)); // a second request would arrive later
    expect(writes()).toHaveLength(1);
    held.release();
    await waitFor(() => {
      expect(
        screen.queryByRole('button', { name: /Remove Zara as a parent/ }),
      ).not.toBeInTheDocument();
    });
  });

  it('does not take over the screen when the user chose someone else while it ran', async () => {
    const { server, idOf } = await open('c3');
    const held = holdWrite(server, 'DELETE');
    await userEvent.click(button('Remove Zara as a parent of Bina Diallo'));
    await userEvent.click(node(idOf.c2 ?? -1));
    held.release();
    await waitFor(() => {
      expect(server.links.some((l) => l.child_id === idOf.c3 && l.parent_id === idOf.s2)).toBe(
        false,
      );
    });
    expect(node(idOf.c2 ?? -1)).toHaveAttribute('aria-pressed', 'true');
    expect(panel('Hawa Diallo')).toBeInTheDocument();
  });
});

describe('partners', () => {
  it('adds a partner with the chosen kind', async () => {
    const { idOf, writes } = await open('c2'); // Hawa has no partner
    await userEvent.click(button('Add partner'));
    await userEvent.selectOptions(screen.getByLabelText('Kind of partnership'), 'partnership');
    await userEvent.type(screen.getByRole('combobox', { name: 'Find a partner' }), 'umar');
    await userEvent.click(screen.getByRole('option', { name: 'Umar Diallo, c. 2008' }));
    await waitFor(() => {
      expect(
        inPanel('Hawa Diallo').getByLabelText('Kind of partnership with Umar Diallo'),
      ).toHaveValue('partnership');
    });
    expect(path(writes()[0])).toBe(`/persons/${String(idOf.c2)}/partners`);
    expect(body(writes()[0])).toEqual({ partner_id: idOf.d1, kind: 'partnership' });
  });

  it('does not offer someone who is already a recorded partner', async () => {
    await open('p1');
    await userEvent.click(button('Add partner'));
    await userEvent.type(screen.getByRole('combobox', { name: 'Find a partner' }), 'rama');
    expect(screen.getByText(/No one matches/)).toBeInTheDocument();
  });

  it('changes the kind of a partnership', async () => {
    const { idOf, writes } = await open('p1');
    await userEvent.selectOptions(
      screen.getByLabelText('Kind of partnership with Rama'),
      'partnership',
    );
    await waitFor(() => {
      expect(screen.getByLabelText('Kind of partnership with Rama')).toHaveValue('partnership');
    });
    expect(writes()[0]?.method).toBe('PATCH');
    expect(path(writes()[0])).toBe(`/persons/${String(idOf.p1)}/partners/${String(idOf.s1)}`);
    expect(body(writes()[0])).toEqual({ kind: 'partnership' });
  });

  it('keeps the old kind on screen when the change is refused', async () => {
    const { server } = await open('p1');
    server.intercept.current = (call) =>
      call.method === 'PATCH' ? errorResponse(500, 'internal') : undefined;
    await userEvent.selectOptions(screen.getByLabelText('Kind of partnership with Rama'), 'other');
    expect(await screen.findByRole('alert')).toHaveTextContent(/something went wrong/i);
    expect(screen.getByLabelText('Kind of partnership with Rama')).toHaveValue('marriage');
  });

  it('removes a recorded partnership; a shared child keeps them linked, without a kind', async () => {
    const { idOf, writes } = await open('g1'); // Abou and Fati are married and share children
    await userEvent.click(button('Remove partnership with Fati Diallo'));
    await waitFor(() => {
      expect(
        screen.queryByRole('button', { name: 'Remove partnership with Fati Diallo' }),
      ).not.toBeInTheDocument();
    });
    expect(writes()[0]?.method).toBe('DELETE');
    expect(path(writes()[0])).toBe(`/persons/${String(idOf.g1)}/partners/${String(idOf.g2)}`);
    expect(inPanel('Abou Diallo').getByText('(shared child)')).toBeInTheDocument();
    expect(screen.queryByLabelText('Kind of partnership with Fati Diallo')).not.toBeInTheDocument();
  });
});

describe('adding a new person as a relative', () => {
  async function startNew(key: string, add: string, role: string) {
    const ctx = await open(key);
    await userEvent.click(button(add));
    await userEvent.click(button(`Add a new person as ${role}…`));
    return ctx;
  }
  const fillAndSave = async (heading: string, given = 'Kadi') => {
    const form = within(panel(heading));
    await userEvent.type(form.getByLabelText(/Given name/), given);
    await userEvent.click(form.getByRole('button', { name: 'Add person' }));
  };

  it('creates the person and links them as a parent, then selects them', async () => {
    const { idOf, writes } = await startNew('s1', 'Add parent', 'parent'); // Rama has no parents
    await fillAndSave('Add a parent of Rama');
    await waitForPeople(13);
    expect(writes().map((c) => `${c.method} ${path(c) ?? ''}`)).toEqual([
      'POST /persons',
      `POST /persons/${String(idOf.s1)}/parents`,
    ]);
    expect(body(writes()[1])).toEqual({ parent_id: 1000 });
    expect(screen.getByRole('button', { name: 'Kadi' })).toHaveAttribute('aria-pressed', 'true');
    expect(
      inPanel('Kadi').getByRole('button', { name: 'Remove Rama as a child of Kadi' }),
    ).toBeInTheDocument();
  });

  it('links a new person as a child', async () => {
    const { idOf, writes } = await startNew('c2', 'Add child', 'child');
    await fillAndSave('Add a child of Hawa Diallo');
    await waitForPeople(13);
    expect(path(writes()[1])).toBe('/persons/1000/parents');
    expect(body(writes()[1])).toEqual({ parent_id: idOf.c2 });
  });

  it('links a new person as a partner, recorded as a marriage', async () => {
    const { idOf, writes } = await startNew('c2', 'Add partner', 'partner');
    await fillAndSave('Add a partner of Hawa Diallo');
    await waitForPeople(13);
    expect(path(writes()[1])).toBe(`/persons/${String(idOf.c2)}/partners`);
    expect(body(writes()[1])).toEqual({ partner_id: 1000, kind: 'marriage' });
  });

  it('says when the person was saved but could not be linked, and keeps them', async () => {
    const { server } = await startNew('s1', 'Add parent', 'parent');
    server.intercept.current = (call) =>
      call.method === 'POST' && call.path.endsWith('/parents')
        ? errorResponse(409, 'too_many_parents')
        : undefined;
    await fillAndSave('Add a parent of Rama');
    await waitForPeople(13);
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('The new person was added, but not linked to Rama');
    expect(alert).toHaveTextContent('already has two parents');
    expect(screen.getByRole('button', { name: 'Kadi' })).toHaveAttribute('aria-pressed', 'true');
    await userEvent.click(within(alert).getByRole('button', { name: 'Dismiss' }));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('uses the partnership kind chosen in the picker for the new person too', async () => {
    const { writes } = await open('c2');
    await userEvent.click(button('Add partner'));
    await userEvent.selectOptions(screen.getByLabelText('Kind of partnership'), 'partnership');
    await userEvent.click(button('Add a new person as partner…'));
    await fillAndSave('Add a partner of Hawa Diallo');
    await waitForPeople(13);
    expect(body(writes()[1])).toMatchObject({ partner_id: 1000, kind: 'partnership' });
  });

  it('starts a fresh form when the top Add person is pressed while a relative form is open', async () => {
    await startNew('s1', 'Add parent', 'parent');
    await userEvent.type(
      within(panel('Add a parent of Rama')).getByLabelText(/Given name/),
      'Kadi',
    );
    // two buttons share this name: the one at the top of the screen comes first in the page
    const [topButton] = screen.getAllByRole('button', { name: 'Add person' });
    if (!topButton) throw new Error('no Add person button');
    await userEvent.click(topButton);
    const form = within(panel('Add a person'));
    expect(form.getByLabelText(/Given name/)).toHaveValue('');
  });

  it('still reports a failed link when the user moved on while it was running', async () => {
    const { server, idOf, writes } = await startNew('s1', 'Add parent', 'parent');
    server.intercept.current = (call) =>
      call.method === 'POST' && call.path.endsWith('/parents')
        ? new Promise<Response>((resolve) => {
            held = () => {
              server.intercept.current = null;
              resolve(errorResponse(409, 'too_many_parents'));
            };
          })
        : undefined;
    let held: () => void = () => undefined;
    await fillAndSave('Add a parent of Rama');
    await waitFor(() => {
      expect(writes()).toHaveLength(2); // the person is saved and the link is running
    });
    await userEvent.click(node(idOf.c2 ?? -1)); // the user moves on
    held();
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('The new person was added, but not linked to Rama');
    expect(node(idOf.c2 ?? -1)).toHaveAttribute('aria-pressed', 'true'); // and is not pulled away
    expect(panel('Hawa Diallo')).toBeInTheDocument();
  });

  it('Cancel goes back to the person and gives their card focus', async () => {
    const { idOf, writes } = await startNew('s1', 'Add parent', 'parent');
    await userEvent.click(
      within(panel('Add a parent of Rama')).getByRole('button', { name: 'Cancel' }),
    );
    expect(panel('Rama')).toBeInTheDocument();
    expect(node(idOf.s1 ?? -1)).toHaveFocus();
    expect(writes()).toHaveLength(0);
    expect(people()).toHaveLength(12);
  });
});

describe('signed out during a relationship write', () => {
  it('ends the session', async () => {
    const { server, client } = await open('g1');
    server.intercept.current = (call) =>
      call.method === 'POST' ? errorResponse(401, 'unauthenticated') : undefined;
    await userEvent.click(button('Add parent'));
    await userEvent.type(screen.getByRole('combobox', { name: 'Find a parent' }), 'fati');
    await userEvent.click(picker('parent').getByRole('option'));
    await waitFor(() => {
      expect(client.getQueryData(['session'])).toBeNull();
    });
  });
});
