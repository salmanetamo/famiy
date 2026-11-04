-- 000005_partnership_update_guard: the same_owner rule from 000004 only fired on
-- INSERT. No API updates partnerships yet, but the schema should not allow an
-- UPDATE to join two accounts' people once one exists.
DROP TRIGGER IF EXISTS trg_partnership_same_owner ON partnership;
CREATE TRIGGER trg_partnership_same_owner
    BEFORE INSERT OR UPDATE OF partner_a, partner_b ON partnership
    FOR EACH ROW EXECUTE FUNCTION enforce_partnership_same_owner();
