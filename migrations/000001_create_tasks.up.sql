CREATE TABLE IF NOT EXISTS tasks (
    id            BIGSERIAL PRIMARY KEY,
    task_uid      VARCHAR(36)  NOT NULL UNIQUE,
    name          VARCHAR(255) NOT NULL,
    command       TEXT         NOT NULL,
    workdir       VARCHAR(512) DEFAULT '/tmp',
    env           JSONB        DEFAULT '{}',
    cpu_limit     INT          DEFAULT 0,
    memory_limit  INT          DEFAULT 0,
    timeout_sec   INT          DEFAULT 300,
    priority      INT          DEFAULT 0,
    status        VARCHAR(20)  NOT NULL DEFAULT 'created',
    exit_code     INT,
    stdout        TEXT,
    stderr        TEXT,
    error_message TEXT,
    pid           INT,
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_priority ON tasks(status, priority DESC);
CREATE INDEX idx_tasks_created_at ON tasks(created_at);
