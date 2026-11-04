-- 000004_person_owner: every person belongs to one account (ADR-004).
--
-- NOT NULL with no default: this FAILS if person already has rows. That is
-- intended while nothing is deployed. Once real data exists, ownership needs a
-- backfill migration instead (decide the owner for each existing row first).

ALTER TABLE person
    ADD COLUMN owner_user_id BIGINT NOT NULL REFERENCES app_user(id) ON DELETE RESTRICT;
CREATE INDEX idx_person_owner ON person (owner_user_id);

-- Ownership is permanent: the same_owner rule below is only checked when a link
-- is created, so changing a person's owner afterwards would silently break it.
CREATE FUNCTION forbid_owner_change() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.owner_user_id IS DISTINCT FROM OLD.owner_user_id THEN
        RAISE EXCEPTION 'person.owner_user_id cannot be changed'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'owner_immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_person_owner_immutable
    BEFORE UPDATE OF owner_user_id ON person
    FOR EACH ROW EXECUTE FUNCTION forbid_owner_change();

-- Parent links: same body as 000002 plus the same_owner rule, placed first so a
-- cross-owner attempt is never reported as anything else. Lock order is
-- unchanged: advisory lock first (see 000002 for why that order matters).
CREATE OR REPLACE FUNCTION enforce_parent_link_rules() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.parent_id = NEW.child_id THEN
        RETURN NEW;
    END IF;

    PERFORM pg_advisory_xact_lock(727001);

    -- Both ends must belong to the same account. If either person does not
    -- exist the join is empty and the foreign key reports it instead.
    IF EXISTS (SELECT 1
               FROM   person p JOIN person c ON c.id = NEW.child_id
               WHERE  p.id = NEW.parent_id AND p.owner_user_id <> c.owner_user_id) THEN
        RAISE EXCEPTION 'persons % and % belong to different accounts', NEW.parent_id, NEW.child_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'same_owner';
    END IF;

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

-- Partnerships (no API yet): same rule, so the schema cannot be used to join
-- two accounts' people once the API exists.
CREATE FUNCTION enforce_partnership_same_owner() RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (SELECT 1
               FROM   person a JOIN person b ON b.id = NEW.partner_b
               WHERE  a.id = NEW.partner_a AND a.owner_user_id <> b.owner_user_id) THEN
        RAISE EXCEPTION 'persons % and % belong to different accounts', NEW.partner_a, NEW.partner_b
            USING ERRCODE = 'check_violation', CONSTRAINT = 'same_owner';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_partnership_same_owner
    BEFORE INSERT ON partnership
    FOR EACH ROW EXECUTE FUNCTION enforce_partnership_same_owner();
