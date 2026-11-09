-- 000006_add_partnership_kind_check: partnership.kind becomes a closed set.
--
-- The column was free text (default 'marriage'). The API now accepts only these
-- three values and the layout code switches on them, so the database enforces the
-- same list. Extending it later means dropping and re-adding this constraint.
--
-- Fails if a row holds any other kind. That is intended: no API ever wrote
-- partnerships before this slice and nothing is deployed, so a failure means
-- someone inserted odd rows by hand and should look at them before they are
-- rewritten.
ALTER TABLE partnership
    ADD CONSTRAINT partnership_kind_allowed CHECK (kind IN ('marriage', 'partnership', 'other'));
