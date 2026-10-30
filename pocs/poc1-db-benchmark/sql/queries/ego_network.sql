-- Q4 — EGO NETWORK ("center on this person"): the immediate family.
-- Parents, children, partners, and DERIVED siblings (people who share at least
-- one parent). This is the hot path the UI hits on every node selection, so it
-- must be cheap. No recursion. Parameter: %(id)s.
SELECT 'parent'::text AS relation, pc.parent_id AS person_id
FROM   parent_child pc
WHERE  pc.child_id = %(id)s
UNION ALL
SELECT 'child', pc.child_id
FROM   parent_child pc
WHERE  pc.parent_id = %(id)s
UNION ALL
SELECT 'partner',
       CASE WHEN p.partner_a = %(id)s THEN p.partner_b ELSE p.partner_a END
FROM   partnership p
WHERE  p.partner_a = %(id)s OR p.partner_b = %(id)s
UNION ALL
SELECT DISTINCT 'sibling', pc2.child_id
FROM   parent_child pc1
JOIN   parent_child pc2 ON pc2.parent_id = pc1.parent_id
WHERE  pc1.child_id = %(id)s
  AND  pc2.child_id <> %(id)s;
