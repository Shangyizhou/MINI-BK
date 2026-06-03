CREATE TABLE IF NOT EXISTS nodes (
    id BIGSERIAL PRIMARY KEY,
    node_id VARCHAR(36) NOT NULL UNIQUE,
    hostname VARCHAR(255) NOT NULL,
    ip VARCHAR(45) NOT NULL,
    version VARCHAR(20) DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'offline',
    total_cpu INT DEFAULT 0,
    total_memory_mb INT DEFAULT 0,
    total_disk_mb INT DEFAULT 0,
    cpu_usage_percent DOUBLE PRECISION DEFAULT 0,
    memory_used_mb INT DEFAULT 0,
    disk_used_mb INT DEFAULT 0,
    load_avg_1m DOUBLE PRECISION DEFAULT 0,
    running_tasks INT DEFAULT 0,
    labels TEXT[] DEFAULT '{}',
    last_heartbeat_at TIMESTAMPTZ,
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status);
