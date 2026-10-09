CREATE TABLE synk_queues (
  name text PRIMARY KEY,
  is_paused boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT NOW(),
  updated_at timestamptz NOT NULL DEFAULT NOW(),
  CONSTRAINT synk_queue_name_length CHECK (char_length(name) > 0 AND char_length(name) < 128)
);

INSERT INTO synk_queues (name) VALUES ('default') ON CONFLICT DO NOTHING;

CREATE TABLE synk_nodes (
  id text PRIMARY KEY,
  hostname text NOT NULL,
  pid integer NOT NULL,
  queues text[] NOT NULL DEFAULT '{}',
  started_at timestamptz NOT NULL DEFAULT NOW(),
  last_heartbeat_at timestamptz NOT NULL DEFAULT NOW()
);

CREATE INDEX synk_nodes_last_heartbeat_idx ON synk_nodes(last_heartbeat_at);

CREATE TYPE synk_job_state AS ENUM(
  'available',
  'cancelled',
  'completed',
  'running',
  'scheduled',
  'pending'
);

CREATE TABLE IF NOT EXISTS synk_jobs (
  id bigserial PRIMARY KEY,
  name text NOT NULL,
  state synk_job_state NOT NULL DEFAULT 'available'::synk_job_state,
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

  locked_by text REFERENCES synk_nodes(id) ON DELETE SET NULL,
  attempted_at timestamptz,
  attempted_by text[],
  depends_on bigint[] DEFAULT '{}',
  remaining_dependencies INTEGER DEFAULT 0,

  CONSTRAINT synk_finalized_or_finalized_at_null CHECK ((state IN ('cancelled', 'completed') AND finalized_at IS NOT NULL) OR finalized_at IS NULL),
  CONSTRAINT synk_max_attempts_is_positive CHECK (max_attempts > 0),
  CONSTRAINT synk_priority_in_range CHECK (priority >= 1 AND priority <= 4),
  CONSTRAINT synk_queue_length CHECK (char_length(queue) > 0 AND char_length(queue) < 128),
  CONSTRAINT synk_kind_length CHECK (char_length(kind) > 0 AND char_length(kind) < 128),
  CONSTRAINT synk_no_self_dependency CHECK (NOT (id = ANY(depends_on))),
  CONSTRAINT synk_name_length CHECK (char_length(name) > 0 AND char_length(name) < 128),
  CONSTRAINT synk_non_negative_dependencies CHECK (remaining_dependencies >= 0),
  CONSTRAINT synk_fk_job_queue FOREIGN KEY (queue) REFERENCES synk_queues(name) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS synk_job_kind ON synk_jobs USING btree(kind);
CREATE INDEX IF NOT EXISTS synk_job_state_and_finalized_at_index ON synk_jobs USING btree(state, finalized_at) WHERE finalized_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS synk_job_prioritized_fetching_index ON synk_jobs USING btree(state, queue, priority, scheduled_at, id);
CREATE INDEX IF NOT EXISTS synk_job_args_index ON synk_jobs USING GIN(args);
CREATE INDEX IF NOT EXISTS synk_job_find_children_idx ON synk_jobs USING GIN(depends_on) WHERE array_length(depends_on, 1) > 0 AND state = 'pending';

CREATE TABLE IF NOT EXISTS synk_heartbeat (
  node_id VARCHAR NOT NULL REFERENCES synk_nodes(id) ON DELETE CASCADE,
  queue_name VARCHAR NOT NULL REFERENCES synk_queues(name) ON DELETE CASCADE,

  active_jobs INT DEFAULT 0,
  jobs_fetched INT DEFAULT 0,
  jobs_started INT DEFAULT 0,
  jobs_completed INT DEFAULT 0,
  jobs_failed INT DEFAULT 0,
  jobs_cancelled INT DEFAULT 0,

  heartbeats_sent INT DEFAULT 0,
  heartbeat_errors INT DEFAULT 0,
  last_heartbeat TIMESTAMPTZ,

  last_job_started TIMESTAMPTZ,
  last_job_completed TIMESTAMPTZ,

  PRIMARY KEY (node_id, queue_name)
);

CREATE INDEX IF NOT EXISTS synk_idx_synk_heartbeat_queue ON synk_heartbeat(queue_name);

CREATE TABLE IF NOT EXISTS synk_heartbeat_history (
  id BIGSERIAL PRIMARY KEY,
  node_id VARCHAR(100) NOT NULL, 
  queue_name VARCHAR(50) NOT NULL REFERENCES synk_queues(name) ON DELETE CASCADE,
  recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

  active_jobs INT NOT NULL DEFAULT 0,
  jobs_fetched INT NOT NULL DEFAULT 0,
  jobs_started INT NOT NULL DEFAULT 0,
  jobs_completed INT NOT NULL DEFAULT 0,
  jobs_failed INT NOT NULL DEFAULT 0,
  jobs_cancelled INT NOT NULL DEFAULT 0,

  FOREIGN KEY (node_id, queue_name) REFERENCES synk_heartbeat(node_id, queue_name) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS synk_idx_synk_heartbeat_history_time ON synk_heartbeat_history(recorded_at);