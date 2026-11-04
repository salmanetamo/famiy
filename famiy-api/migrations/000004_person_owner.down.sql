DROP TRIGGER  IF EXISTS trg_partnership_same_owner ON partnership;
DROP FUNCTION IF EXISTS enforce_partnership_same_owner();

-- Restore the 000002 version of the parent-link rules (no same_owner check).
CREATE OR REPLACE FUNCTION enforce_parent_link_rules() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.parent_id = NEW.child_id THEN
        RETURN NEW;
    END IF;

    PERFORM pg_advisory_xact_lock(727001);

    IF EXISTS (SELECT 1 FROM parent_child
               WHERE parent_id = NEW.parent_id AND child_id = NEW.child_id) THEN
        RETURN NEW;
    END IF;

    IF (SELECT count(*) FROM parent_child WHERE child_id = NEW.child_id) >= 2 THEN
        RAISE EXCEPTION 'person % already has two parents', NEW.child_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'max_two_parents';
    END IF;

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

DROP TRIGGER  IF EXISTS trg_person_owner_immutable ON person;
DROP FUNCTION IF EXISTS forbid_owner_change();

DROP INDEX IF EXISTS idx_person_owner;
ALTER TABLE person DROP COLUMN IF EXISTS owner_user_id;
