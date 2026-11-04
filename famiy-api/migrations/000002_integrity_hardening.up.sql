-- 000002_integrity_hardening: findings from the Phase 4 review.
--   * Bound free-text and date input at the database too.
--   * Replace the two-parent trigger with one trigger that enforces every
--     parent-link rule under a single lock, adding acyclicity.
--   * Identify each rule by CONSTRAINT name, so the application maps errors
--     without matching message text.

-- 1. Bounds on person (mirrored by person.New in the domain layer).
ALTER TABLE person
    ADD CONSTRAINT given_name_max_len  CHECK (char_length(given_name) <= 200),
    ADD CONSTRAINT family_name_max_len CHECK (family_name IS NULL OR char_length(family_name) <= 200),
    ADD CONSTRAINT birth_date_min      CHECK (birth_date IS NULL OR birth_date >= DATE '1000-01-01'),
    ADD CONSTRAINT death_date_min      CHECK (death_date IS NULL OR death_date >= DATE '1000-01-01');

-- 2. One trigger for all parent-link rules.
--
-- Concurrency design. Both "at most two parents" and "no cycle" read other rows,
-- so concurrent inserts could each pass the check and together break the rule
-- (e.g. A->B and B->A lock different children, so a per-child lock cannot
-- serialize them). Every INSERT therefore takes ONE transaction-scoped advisory
-- lock first and does all its checking under it, so link inserts run one at a
-- time. That is fine for a write-light app.
--
-- Lock order matters: the advisory lock must be taken BEFORE any row lock. An
-- earlier draft took a per-child FOR UPDATE first; A->B and B->A then deadlocked
-- (each held a person row the other needed for its foreign-key check while
-- waiting on the advisory lock). One trigger, lock first, avoids relying on
-- alphabetical trigger order too.
--
-- The checks read a fresh snapshot after the lock, which relies on READ
-- COMMITTED (the default); do not run link inserts under REPEATABLE READ.
-- If links ever become per-tree, key the lock by tree.
DROP TRIGGER  IF EXISTS trg_max_two_parents ON parent_child;
DROP FUNCTION IF EXISTS enforce_max_two_parents();

CREATE FUNCTION enforce_parent_link_rules() RETURNS TRIGGER AS $$
BEGIN
    -- A self-link is reported by the no_self_parent CHECK (CHECKs run after
    -- BEFORE triggers, so stepping aside keeps that error accurate).
    IF NEW.parent_id = NEW.child_id THEN
        RETURN NEW;
    END IF;

    PERFORM pg_advisory_xact_lock(727001);  -- arbitrary app-wide key for parent_child writes

    -- An exact duplicate edge is the primary key's error to report ("already
    -- exists"), not ours, so step aside.
    IF EXISTS (SELECT 1 FROM parent_child
               WHERE parent_id = NEW.parent_id AND child_id = NEW.child_id) THEN
        RETURN NEW;
    END IF;

    IF (SELECT count(*) FROM parent_child WHERE child_id = NEW.child_id) >= 2 THEN
        RAISE EXCEPTION 'person % already has two parents', NEW.child_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'max_two_parents';
    END IF;

    -- Acyclicity: reject if the would-be child is already an ancestor of the
    -- would-be parent (the new edge would close a loop).
    IF EXISTS (
        WITH RECURSIVE anc(id) AS (
                SELECT NEW.parent_id
            UNION
                SELECT pc.parent_id
                FROM   parent_child pc
                JOIN   anc a ON pc.child_id = a.id
        )
        SELECT 1 FROM anc WHERE id = NEW.child_id
    ) THEN
        RAISE EXCEPTION 'linking % as a parent of % would create a cycle', NEW.parent_id, NEW.child_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'no_cycle';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_parent_link_rules
    BEFORE INSERT ON parent_child
    FOR EACH ROW EXECUTE FUNCTION enforce_parent_link_rules();

-- The trigger fires on INSERT only. Edges are inserted or deleted, never
-- updated; changing an edge means delete + insert.
COMMENT ON TABLE parent_child IS
    'Insert/delete only. Invariants (two parents, no cycle) are enforced by an INSERT trigger; do not UPDATE rows.';
