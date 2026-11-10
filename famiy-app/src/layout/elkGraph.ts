import type { ElkExtendedEdge, ElkNode } from 'elkjs/lib/elk-api';
import type { FamilyGraph, Union } from '../domain/family';

/** Fixed-size person cards keep layout and rendering simple (ADR-005, slice 2 design). */
export const CARD_WIDTH = 160;
export const CARD_HEIGHT = 56;
export const UNION_SIZE = 14;

export const personNodeId = (personId: number): string => `p${String(personId)}`;
export const unionNodeId = (union: Pick<Union, 'id'>): string => `u${union.id}`;
export const containerNodeId = (union: Pick<Union, 'id'>): string => `c${union.id}`;

export function edgeKindOf(edgeId: string): 'partner' | 'descent' {
  return edgeId.startsWith('partner:') ? 'partner' : 'descent';
}

// elkjs options that matter for genealogy (each validated in PoC-2, see ADR-002).
const LAYOUT_OPTIONS: Record<string, string> = {
  'elk.algorithm': 'layered',
  'elk.direction': 'DOWN',
  // Pulls a married-in spouse onto their partner's row instead of stranding them near the roots.
  'elk.layered.layering.strategy': 'NETWORK_SIMPLEX',
  'elk.layered.crossingMinimization.strategy': 'LAYER_SWEEP',
  'elk.layered.nodePlacement.strategy': 'BRANDES_KOEPF',
  // Weak tiebreaker: keep our input order (birth date), so siblings read oldest to youngest.
  'elk.layered.considerModelOrder.strategy': 'NODES',
  'elk.layered.spacing.nodeNodeBetweenLayers': '45',
  'elk.spacing.nodeNode': '28',
};

/**
 * Builds the augmented graph ELK lays out (ADR-002): a union node between partners with children
 * hanging from it, and a "couple container" around each monogamous couple so ELK cannot pull the
 * partners apart (PoC-2: a cousin marriage's span fell from 460px to 112px). A person in two
 * unions cannot be in a container (an ELK node has one parent), so remarried people stay at the
 * root. Pure: the same family always gives the same graph.
 */
export function buildElkGraph(family: FamilyGraph): ElkNode {
  const unionsOf = new Map<number, number>();
  for (const union of family.unions) {
    for (const partner of union.partners) {
      unionsOf.set(partner, (unionsOf.get(partner) ?? 0) + 1);
    }
  }

  const containerOf = new Map<number, Union>();
  for (const union of family.unions) {
    const [a, b] = union.partners;
    if (unionsOf.get(a) === 1 && unionsOf.get(b) === 1) {
      containerOf.set(a, union);
      containerOf.set(b, union);
    }
  }

  const personNode = (personId: number): ElkNode => ({
    id: personNodeId(personId),
    width: CARD_WIDTH,
    height: CARD_HEIGHT,
  });
  const inDisplayOrder = (ids: readonly number[]) =>
    family.persons.filter((p) => ids.includes(p.id)).map((p) => p.id);

  const containers: ElkNode[] = [...new Set(containerOf.values())].map((union) => ({
    id: containerNodeId(union),
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': 'DOWN',
      'elk.spacing.nodeNode': '16',
      // No padding: with padding, couple members sat 6px lower than other people in the same
      // generation (found in slice 2 by a test; containers are never drawn, so padding buys nothing).
      'elk.padding': '[top=0,left=0,bottom=0,right=0]',
    },
    children: inDisplayOrder(union.partners).map(personNode),
  }));

  const rootPersons = family.persons
    .filter((p) => !containerOf.has(p.id))
    .map((p) => personNode(p.id));

  const unionNodes: ElkNode[] = family.unions.map((union) => ({
    id: unionNodeId(union),
    width: UNION_SIZE,
    height: UNION_SIZE,
  }));

  const edges: ElkExtendedEdge[] = [];
  for (const union of family.unions) {
    for (const partner of union.partners) {
      edges.push({
        id: `partner:${union.id}:${String(partner)}`,
        sources: [personNodeId(partner)],
        targets: [unionNodeId(union)],
      });
    }
    for (const child of union.children) {
      edges.push({
        id: `descent:${union.id}:${String(child)}`,
        sources: [unionNodeId(union)],
        targets: [personNodeId(child)],
      });
    }
  }
  for (const [parent, kids] of family.singleParentChildren) {
    for (const child of kids) {
      edges.push({
        id: `descent:${String(parent)}:${String(child)}`,
        sources: [personNodeId(parent)],
        targets: [personNodeId(child)],
      });
    }
  }

  const layoutOptions: Record<string, string> = { ...LAYOUT_OPTIONS };
  if (containers.length > 0) layoutOptions['elk.hierarchyHandling'] = 'INCLUDE_CHILDREN';

  return {
    id: 'root',
    layoutOptions,
    children: [...containers, ...rootPersons, ...unionNodes],
    edges,
  };
}
