CREATE TABLE queue (
  name text PRIMARY KEY,
  is_paused boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT NOW(),
  updated_at timestamptz NOT NULL DEFAULT NOW(),
  CONSTRAINT queue_name_length CHECK (char_length(name) > 0 AND char_length(name) < 128)
);

INSERT INTO queue (name) VALUES ('default') ON CONFLICT DO NOTHING;

CREATE TABLE node (
  id text PRIMARY KEY,
  hostname text NOT NULL,
  pid integer NOT NULL,
  queues text[] NOT NULL DEFAULT '{}',
  started_at timestamptz NOT NULL DEFAULT NOW(),
  last_heartbeat_at timestamptz NOT NULL DEFAULT NOW()
);

CREATE INDEX node_last_heartbeat_idx ON node(last_heartbeat_at);

CREATE TYPE job_state AS ENUM(
  'available',
  'cancelled',
  'completed',
  'running',
  'scheduled',
  'pending'
);

CREATE TABLE job(
  id bigserial PRIMARY KEY,
  name text NOT NULL,
  state job_state NOT NULL DEFAULT 'available'::job_state,
  priority smallint NOT NULL DEFAULT 3,
  attempt smallint NOT NULL DEFAULT 0,
  max_attempts smallint NOT NULL,

  created_at timestamptz NOT NULL DEFAULT NOW(),
  scheduled_at timestamptz NOT NULL DEFAULT NOW(),
  finalized_at timestamptz,

  kind text NOT NULL,
  queue text NOT NULL DEFAULT 'default'::text,
  args jsonb,
  errors jsonb[] NOT NULL DEFAULT '{}'::jsonb[],

  locked_by text REFERENCES node(id) ON DELETE SET NULL,
  attempted_at timestamptz,
  attempted_by text[],
  depends_on bigint[] DEFAULT '{}',
  remaining_dependencies INTEGER DEFAULT 0,

  CONSTRAINT finalized_or_finalized_at_null CHECK ((state IN ('cancelled', 'completed') AND finalized_at IS NOT NULL) OR finalized_at IS NULL),
  CONSTRAINT max_attempts_is_positive CHECK (max_attempts > 0),
  CONSTRAINT priority_in_range CHECK (priority >= 1 AND priority <= 4),
  CONSTRAINT queue_length CHECK (char_length(queue) > 0 AND char_length(queue) < 128),
  CONSTRAINT kind_length CHECK (char_length(kind) > 0 AND char_length(kind) < 128),
  CONSTRAINT no_self_dependency CHECK (NOT (id = ANY(depends_on))),
  CONSTRAINT name_length CHECK (char_length(name) > 0 AND char_length(name) < 128),
  CONSTRAINT non_negative_dependencies CHECK (remaining_dependencies >= 0),
  CONSTRAINT fk_job_queue FOREIGN KEY (queue) REFERENCES queue(name) ON DELETE RESTRICT
);

CREATE INDEX job_kind ON job USING btree(kind);
CREATE INDEX job_state_and_finalized_at_index ON job USING btree(state, finalized_at) WHERE finalized_at IS NOT NULL;
CREATE INDEX job_prioritized_fetching_index ON job USING btree(state, queue, priority, scheduled_at, id);
CREATE INDEX job_args_index ON job USING GIN(args);
CREATE INDEX job_find_children_idx ON job USING GIN(depends_on) WHERE array_length(depends_on, 1) > 0 AND state = 'pending';
