import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Table,
  Select,
  Badge,
  Progress,
  Tag,
  Button,
  Space,
  Popconfirm,
  message,
  Typography,
  Result,
} from 'antd';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import 'dayjs/locale/zh-cn';
import { useNodes, useDrainNode, useUncordonNode } from '../../../hooks/useNodes';
import type { Node } from '../../../types';

dayjs.extend(relativeTime);
dayjs.locale('zh-cn');

const statusOptions = [
  { value: '', label: '全部状态' },
  { value: 'online', label: '在线' },
  { value: 'offline', label: '离线' },
  { value: 'drain', label: '排空' },
  { value: 'cordon', label: '封锁' },
];

const statusBadgeStatus: Record<string, 'success' | 'default' | 'warning' | 'error'> = {
  online: 'success',
  offline: 'default',
  drain: 'warning',
  cordon: 'error',
};

export default function NodeList() {
  const navigate = useNavigate();
  const [statusFilter, setStatusFilter] = useState<string>('');
  const { data: nodes, isLoading, isError, error } = useNodes({ status: statusFilter || undefined });
  const drainMutation = useDrainNode();
  const uncordonMutation = useUncordonNode();

  const handleDrain = (nodeId: string) => {
    drainMutation.mutate(nodeId, {
      onSuccess: () => message.success('节点已设置为排空状态'),
      onError: (err) =>
        message.error(`操作失败: ${err instanceof Error ? err.message : '未知错误'}`),
    });
  };

  const handleUncordon = (nodeId: string) => {
    uncordonMutation.mutate(nodeId, {
      onSuccess: () => message.success('节点已解除封锁'),
      onError: (err) =>
        message.error(`操作失败: ${err instanceof Error ? err.message : '未知错误'}`),
    });
  };

  const columns = [
    {
      title: '主机名',
      dataIndex: 'hostname',
      key: 'hostname',
      ellipsis: true,
    },
    {
      title: 'IP',
      dataIndex: 'ip',
      key: 'ip',
      width: 140,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => (
        <Badge status={statusBadgeStatus[status] || 'default'} text={status} />
      ),
    },
    {
      title: 'CPU 使用率',
      dataIndex: 'cpu_usage_percent',
      key: 'cpu_usage_percent',
      width: 160,
      render: (percent: number) => (
        <Progress
          percent={Math.round(percent)}
          size="small"
          status={percent >= 90 ? 'exception' : undefined}
        />
      ),
    },
    {
      title: '内存使用率',
      key: 'memory_usage',
      width: 160,
      render: (_: unknown, record: Node) => {
        const percent =
          record.total_memory_mb > 0
            ? Math.round((record.memory_used_mb / record.total_memory_mb) * 100)
            : 0;
        return (
          <Progress
            percent={percent}
            size="small"
            status={percent >= 90 ? 'exception' : undefined}
          />
        );
      },
    },
    {
      title: '运行任务',
      dataIndex: 'running_tasks',
      key: 'running_tasks',
      width: 100,
    },
    {
      title: '标签',
      dataIndex: 'labels',
      key: 'labels',
      width: 200,
      ellipsis: true,
      render: (labels: string[]) =>
        labels && labels.length > 0
          ? labels.map((label) => (
              <Tag key={label} color="blue" style={{ marginBottom: 2 }}>
                {label}
              </Tag>
            ))
          : '-',
    },
    {
      title: '最后心跳',
      dataIndex: 'last_heartbeat_at',
      key: 'last_heartbeat_at',
      width: 140,
      render: (val: string | null) =>
        val ? dayjs(val).fromNow() : '-',
    },
    {
      title: '操作',
      key: 'actions',
      width: 180,
      render: (_: unknown, record: Node) => {
        const isDrainOrCordon = record.status === 'drain' || record.status === 'cordon';
        return (
          <Space size="small">
            {record.status !== 'drain' && record.status !== 'cordon' && (
              <Popconfirm
                title="确认排空"
                description="将节点设置为排空状态，不再调度新任务？"
                onConfirm={(e) => {
                  e?.stopPropagation();
                  handleDrain(record.node_id);
                }}
                okText="确定"
                cancelText="取消"
              >
                <Button
                  type="link"
                  size="small"
                  danger
                  onClick={(e) => e.stopPropagation()}
                  loading={drainMutation.isPending}
                >
                  排空
                </Button>
              </Popconfirm>
            )}
            {isDrainOrCordon && (
              <Popconfirm
                title="确认解除"
                description="解除节点的排空/封锁状态？"
                onConfirm={(e) => {
                  e?.stopPropagation();
                  handleUncordon(record.node_id);
                }}
                okText="确定"
                cancelText="取消"
              >
                <Button
                  type="link"
                  size="small"
                  onClick={(e) => e.stopPropagation()}
                  loading={uncordonMutation.isPending}
                >
                  解除
                </Button>
              </Popconfirm>
            )}
          </Space>
        );
      },
    },
  ];

  if (isError) {
    return (
      <div>
        <Typography.Title level={3} style={{ marginTop: 0 }}>节点列表</Typography.Title>
        <Result status="error" title="加载失败" subTitle={String(error)} />
      </div>
    );
  }

  return (
    <div>
      <Typography.Title level={3} style={{ marginTop: 0 }}>节点列表</Typography.Title>

      <div style={{ marginBottom: 16 }}>
        <Select
          value={statusFilter}
          onChange={setStatusFilter}
          options={statusOptions}
          style={{ width: 160 }}
        />
      </div>

      <Table
        dataSource={nodes}
        columns={columns}
        rowKey="node_id"
        loading={isLoading}
        pagination={false}
        size="middle"
        onRow={(record) => ({
          onClick: () => navigate(`/nodes/${record.node_id}`),
          style: { cursor: 'pointer' },
        })}
      />
    </div>
  );
}
