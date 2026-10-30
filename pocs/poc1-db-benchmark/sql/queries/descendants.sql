-- Q2 — DESCENDANTS of a person, with shortest generational distance to each.
-- Walks parent_child DOWNWARD (parent -> child). Parameter: %(id)s.
WITH RECURSIVE descendant(id, depth) AS (
        SELECT child_id, 1
        FROM   parent_child
        WHERE  parent_id = %(id)s
    UNION ALL
        SELECT pc.child_id, d.depth + 1
        FROM   parent_child pc
        JOIN   descendant d ON pc.parent_id = d.id
) CYCLE id SET is_cycle USING path
SELECT id AS descendant_id, min(depth) AS generations
FROM   descendant
GROUP  BY id
ORDER  BY generations, descendant_id;
