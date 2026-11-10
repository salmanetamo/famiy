import { describe, expect, it } from 'vitest';
import { buildFamilyGraph } from '../domain/family';
import { apiGraphFrom, emptyGraph, messy } from '../test/fixtures';
import { buildElkGraph, CARD_HEIGHT, CARD_WIDTH, personNodeId } from './elkGraph';

function setup() {
  const { api, idOf } = apiGraphFrom(messy);
  const elk = buildElkGraph(buildFamilyGraph(api));
  const id = (key: string) => idOf[key] ?? -1;
  return { elk, id };
}

describe('buildElkGraph', () => {
  it('is an empty root for an empty account, without containers or hierarchy handling', () => {
    const elk = buildElkGraph(buildFamilyGraph(emptyGraph()));
    expect(elk.children).toEqual([]);
    expect(elk.edges).toEqual([]);
    expect(elk.layoutOptions?.['elk.hierarchyHandling']).toBeUndefined();
  });

  it('wraps only monogamous couples in containers; a remarried person stays at the root', () => {
    const { elk, id } = setup();
    const containers = (elk.children ?? []).filter((n) => n.children !== undefined);
    const inContainers = containers.flatMap((c) => (c.children ?? []).map((n) => n.id));
    // Abou+Fati, Nana+Moussa and Ali+Sala are each in exactly one union
    expect(containers).toHaveLength(3);
    for (const key of ['g1', 'g2', 'p2', 's3', 'c1', 'c4']) {
      expect(inContainers).toContain(personNodeId(id(key)));
    }
    // Idi is in two unions, so neither he nor his two wives can be in a container
    for (const key of ['p1', 's1', 's2']) {
      expect(inContainers).not.toContain(personNodeId(id(key)));
      expect((elk.children ?? []).map((n) => n.id)).toContain(personNodeId(id(key)));
    }
    expect(elk.layoutOptions?.['elk.hierarchyHandling']).toBe('INCLUDE_CHILDREN');
  });

  it('gives every person a fixed-size card, and puts each person in the graph once', () => {
    const { elk } = setup();
    const people = [
      ...(elk.children ?? []).filter((n) => n.children === undefined && n.id.startsWith('p')),
      ...(elk.children ?? []).flatMap((n) => n.children ?? []),
    ];
    expect(people).toHaveLength(12);
    expect(new Set(people.map((n) => n.id)).size).toBe(12);
    for (const p of people) {
      expect(p.width).toBe(CARD_WIDTH);
      expect(p.height).toBe(CARD_HEIGHT);
    }
  });

  it('routes partners into a union and children out of it', () => {
    const { elk, id } = setup();
    const edges = elk.edges ?? [];
    expect(edges.filter((e) => e.id.startsWith('partner:'))).toHaveLength(10); // 5 unions x 2
    expect(edges.filter((e) => e.id.startsWith('descent:'))).toHaveLength(7); // 7 children with two parents
    const toUmar = edges.find((e) => e.targets.includes(personNodeId(id('d1'))));
    expect(toUmar?.sources[0]?.startsWith('u')).toBe(true);
  });
});
