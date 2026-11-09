-- 000007_add_person_bio_residence: a short biography and a current residence.
--
-- Both are optional free text. NULL means "nothing": the domain stores a blank
-- value as NULL, and the constraints below refuse a blank string, so there is
-- one representation of an empty field (as for family_name). Limits are in
-- characters and repeat person.MaxBioLen / person.MaxResidenceLen.
--
-- Adding nullable columns without a default does not rewrite the table.
ALTER TABLE person
    ADD COLUMN bio       TEXT,
    ADD COLUMN residence TEXT,
    -- "Not blank" means "has a non-whitespace character". btrim() would only strip
    -- spaces, letting a bio of just line breaks or tabs through. \S follows the
    -- server's locale, which is why the domain's TrimSpace is the first gate.
    ADD CONSTRAINT bio_not_blank       CHECK (bio IS NULL OR bio ~ '\S'),
    ADD CONSTRAINT bio_max_len         CHECK (bio IS NULL OR char_length(bio) <= 2000),
    ADD CONSTRAINT residence_not_blank CHECK (residence IS NULL OR residence ~ '\S'),
    ADD CONSTRAINT residence_max_len   CHECK (residence IS NULL OR char_length(residence) <= 200);
