DROP TRIGGER IF EXISTS trg_partnership_same_owner ON partnership;
CREATE TRIGGER trg_partnership_same_owner
    BEFORE INSERT ON partnership
    FOR EACH ROW EXECUTE FUNCTION enforce_partnership_same_owner();
