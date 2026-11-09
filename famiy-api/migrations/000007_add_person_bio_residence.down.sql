-- Dropping the columns destroys every stored bio and residence. Fine before
-- anything is deployed; once real data exists, back it up first.
ALTER TABLE person
    DROP CONSTRAINT IF EXISTS bio_not_blank,
    DROP CONSTRAINT IF EXISTS bio_max_len,
    DROP CONSTRAINT IF EXISTS residence_not_blank,
    DROP CONSTRAINT IF EXISTS residence_max_len,
    DROP COLUMN IF EXISTS bio,
    DROP COLUMN IF EXISTS residence;
