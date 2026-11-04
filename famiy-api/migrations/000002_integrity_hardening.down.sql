COMMENT ON TABLE parent_child IS NULL;

DROP TRIGGER  IF EXISTS trg_parent_link_rules ON parent_child;
DROP FUNCTION IF EXISTS enforce_parent_link_rules();

-- Restore the 000001 two-parent trigger exactly as 000001 defines it.
CREATE FUNCTION enforce_max_two_parents() RETURNS TRIGGER AS $$
BEGIN
    PERFORM 1 FROM person WHERE id = NEW.child_id FOR UPDATE;
    IF EXISTS (SELECT 1 FROM parent_child
               WHERE parent_id = NEW.parent_id AND child_id = NEW.child_id) THEN
        RETURN NEW;
    END IF;
    IF (SELECT count(*) FROM parent_child WHERE child_id = NEW.child_id) >= 2 THEN
        RAISE EXCEPTION 'person % already has two parents', NEW.child_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_max_two_parents
    BEFORE INSERT ON parent_child
    FOR EACH ROW EXECUTE FUNCTION enforce_max_two_parents();

ALTER TABLE person
    DROP CONSTRAINT IF EXISTS death_date_min,
    DROP CONSTRAINT IF EXISTS birth_date_min,
    DROP CONSTRAINT IF EXISTS family_name_max_len,
    DROP CONSTRAINT IF EXISTS given_name_max_len;
