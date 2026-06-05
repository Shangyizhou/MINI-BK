export interface Task {
  id: number;
  task_uid: string;
  name: string;
  command: string;
  workdir: string;
  env: Record<string, string>;
  cpu_limit: number;
  memory_limit: number;
  timeout_sec: number;
  priority: number;
  status: 'created' | 'pending' | 'running' | 'success' | 'failed' | 'canceled';
  exit_code: number | null;
  stdout: string;
  stderr: string;
  error_message: string;
  pid: number | null;
  started_at: string | null;
  finished_at: string | null;
  created_at: string;
  updated_at: string;
  max_retries: number;
  retry_count: number;
  retry_interval_sec: number;
  idempotency_key: string;
  node_selector: Record<string, string>;
  assigned_node_id: string;
}

export interface Node {
  id: number;
  node_id: string;
  hostname: string;
  ip: string;
  version: string;
  status: 'online' | 'offline' | 'drain' | 'cordon';
  total_cpu: number;
  total_memory_mb: number;
  total_disk_mb: number;
  cpu_usage_percent: number;
  memory_used_mb: number;
  disk_used_mb: number;
  load_avg_1m: number;
  running_tasks: number;
  labels: string[];
  last_heartbeat_at: string | null;
  registered_at: string;
}

export interface TaskListResult {
  tasks: Task[];
  total: number;
  page: number;
  size: number;
}

export interface CreateTaskRequest {
  name: string;
  command: string;
  workdir?: string;
  env?: Record<string, string>;
  cpu_limit?: number;
  memory_limit?: number;
  timeout_sec?: number;
  priority?: number;
  node_selector?: Record<string, string>;
}

export interface DailyStats {
  submitted: string;
  success: string;
  failed: string;
  [key: string]: string;
}
