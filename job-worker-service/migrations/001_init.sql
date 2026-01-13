CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS jobs (
                                    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    type text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending','processing','done','error')),
    input jsonb NOT NULL DEFAULT '{}'::jsonb,
    output jsonb,
    error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
    );

CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at DESC);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_jobs_updated_at ON jobs;
CREATE TRIGGER trg_jobs_updated_at
    BEFORE UPDATE ON jobs
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 1;

ALTER TABLE jobs
    ADD CONSTRAINT IF NOT EXISTS chk_jobs_priority CHECK (priority IN (0,1,2));

CREATE INDEX IF NOT EXISTS idx_jobs_priority_created_at ON jobs(priority DESC, created_at DESC);

