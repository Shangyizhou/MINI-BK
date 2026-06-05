import { useEffect, useRef, useState, useCallback } from 'react';

export interface LogEntry {
  id: string;
  task_uid: string;
  line: string;
  stream: 'stdout' | 'stderr';
  timestamp: number;
}

export function useSSE(taskUid: string | undefined) {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [connected, setConnected] = useState(false);
  const eventSourceRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (!taskUid) return;

    const url = `/api/v1/tasks/${taskUid}/log/stream`;
    const es = new EventSource(url);
    eventSourceRef.current = es;

    es.onopen = () => setConnected(true);
    es.onerror = () => {
      setConnected(false);
      es.close();
    };

    es.onmessage = (event) => {
      try {
        const entry: LogEntry = JSON.parse(event.data);
        setLogs((prev) => [...prev, entry]);
      } catch {
        // non-JSON messages (like "done") are ignored for log display
        if (event.data === 'done') {
          es.close();
          setConnected(false);
        }
      }
    };

    return () => {
      es.close();
      setConnected(false);
    };
  }, [taskUid]);

  const clearLogs = useCallback(() => setLogs([]), []);

  return { logs, connected, clearLogs };
}
