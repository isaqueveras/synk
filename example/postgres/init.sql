-- Cria o schema se não existir
CREATE SCHEMA IF NOT EXISTS synk;

-- Garante que tudo será criado no schema 'synk'
SET search_path TO synk;

CREATE TYPE job_state AS ENUM(
  'available',
  'cancelled',
  'completed',
  'discarded',
  'retryable',
  'running',
  'scheduled'
);

CREATE TABLE job(
  id bigserial PRIMARY KEY,
  state job_state NOT NULL DEFAULT 'available'::job_state,
  attempt smallint NOT NULL DEFAULT 0,
  max_attempts smallint NOT NULL,

  attempted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT NOW(),
  finalized_at timestamptz,
  scheduled_at timestamptz NOT NULL DEFAULT NOW(),

  priority smallint NOT NULL DEFAULT 1,

  args jsonb,
  attempted_by text[],
  errors jsonb[] NOT NULL DEFAULT '{}'::jsonb[],
  kind text NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  queue text NOT NULL DEFAULT 'default'::text,

  CONSTRAINT finalized_or_finalized_at_null CHECK ((state IN ('cancelled', 'completed', 'discarded') AND finalized_at IS NOT NULL) OR finalized_at IS NULL),
  CONSTRAINT max_attempts_is_positive CHECK (max_attempts > 0),
  CONSTRAINT priority_in_range CHECK (priority >= 1 AND priority <= 4),
  CONSTRAINT queue_length CHECK (char_length(queue) > 0 AND char_length(queue) < 128),
  CONSTRAINT kind_length CHECK (char_length(kind) > 0 AND char_length(kind) < 128)
);

CREATE INDEX job_kind ON job USING btree(kind);
CREATE INDEX job_state_and_finalized_at_index ON job USING btree(state, finalized_at) WHERE finalized_at IS NOT NULL;
CREATE INDEX job_prioritized_fetching_index ON job USING btree(state, queue, priority, scheduled_at, id);
CREATE INDEX job_args_index ON job USING GIN(args);
CREATE INDEX job_metadata_index ON job USING GIN(metadata);
