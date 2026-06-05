import { useRef, useEffect } from 'react';
import { Tabs, Button, Space, Tag } from 'antd';
import { ClearOutlined, CheckCircleFilled, CloseCircleFilled } from '@ant-design/icons';
import type { LogEntry } from '../hooks/useSSE';

interface LogStreamProps {
  logs: LogEntry[];
  connected: boolean;
  onClear: () => void;
}

function LogViewer({ logs, filter }: { logs: LogEntry[]; filter?: 'stdout' | 'stderr' }) {
  const ref = useRef<HTMLPreElement>(null);
  const filtered = filter ? logs.filter((l) => l.stream === filter) : logs;

  useEffect(() => {
    if (ref.current) {
      ref.current.scrollTop = ref.current.scrollHeight;
    }
  }, [filtered.length]);

  return (
    <pre
      ref={ref}
      style={{
        height: 400,
        overflow: 'auto',
        background: '#1e1e1e',
        color: '#d4d4d4',
        padding: 12,
        borderRadius: 4,
        fontSize: 13,
        lineHeight: 1.5,
        fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
      }}
    >
      {filtered.length === 0 ? (
        <span style={{ color: '#666' }}>等待日志...</span>
      ) : (
        filtered.map((entry, i) => (
          <div
            key={i}
            style={{ color: entry.stream === 'stderr' ? '#f44747' : '#d4d4d4' }}
          >
            {entry.line}
          </div>
        ))
      )}
    </pre>
  );
}

export default function LogStream({ logs, connected, onClear }: LogStreamProps) {
  const statusColor = connected ? '#52c41a' : '#ff4d4f';
  const statusLabel = connected ? '已连接' : '已断开';

  const tabItems = [
    {
      key: 'all',
      label: '全部',
      children: (
        <LogViewer
          logs={logs}
        />
      ),
    },
    {
      key: 'stdout',
      label: '标准输出',
      children: <LogViewer logs={logs} filter="stdout" />,
    },
    {
      key: 'stderr',
      label: '标准错误',
      children: (
        <LogViewer logs={logs} filter="stderr" />
      ),
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 8, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Space>
          <span>实时日志</span>
          <Tag icon={connected ? <CheckCircleFilled /> : <CloseCircleFilled />} color={statusColor}>
            {statusLabel}
          </Tag>
        </Space>
        <Button
          size="small"
          icon={<ClearOutlined />}
          onClick={onClear}
          disabled={logs.length === 0}
        >
          清空
        </Button>
      </div>
      <Tabs items={tabItems} />
    </div>
  );
}
