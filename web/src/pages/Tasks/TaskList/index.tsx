import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Table, Select, Button, Space, message, Typography, Result } from 'antd';
import { EyeOutlined, CloseCircleOutlined, ReloadOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { useTasks, useCancelTask, useRerunTask } from '../../../hooks/useTasks';
import TaskStatusTag from '../../../components/TaskStatusTag';
import type { Task } from '../../../types';

const terminalStatuses: Task['status'][] = ['success', 'failed', 'canceled'];

const statusOptions = [
  { value: '', label: '全部状态' },
  { value: 'created', label: '已创建' },
  { value: 'pending', label: '等待中' },
  { value: 'running', label: '运行中' },
  { value: 'success', label: '成功' },
  { value: 'failed', label: '失败' },
  { value: 'canceled', label: '已取消' },
];

export default function TaskList() {
  const navigate = useNavigate();
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [page, setPage] = useState(1);
  const [size] = useState(20);

  const { data, isLoading, isError, error } = useTasks({ status: statusFilter || undefined, page, size });
  const cancelMutation = useCancelTask();
  const rerunMutation = useRerunTask();

  const tasks = data?.tasks ?? [];
  const total = data?.total ?? 0;

  const handleCancel = (taskUid: string) => {
    cancelMutation.mutate(taskUid, {
      onSuccess: () => message.success('任务已取消'),
      onError: (err) => message.error(`取消失败: ${err instanceof Error ? err.message : '未知错误'}`),
    });
  };

  const handleRerun = (taskUid: string) => {
    rerunMutation.mutate(taskUid, {
      onSuccess: () => message.success('任务已重跑'),
      onError: (err) => message.error(`重跑失败: ${err instanceof Error ? err.message : '未知错误'}`),
    });
  };

  const columns = [
    {
      title: '任务UID',
      dataIndex: 'task_uid',
      key: 'task_uid',
      width: 100,
      render: (val: string) => val?.substring(0, 8) + '...',
    },
    {
      title: '任务名称',
      dataIndex: 'name',
      key: 'name',
      ellipsis: true,
    },
    {
      title: '命令',
      dataIndex: 'command',
      key: 'command',
      ellipsis: true,
      width: 200,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: Task['status']) => <TaskStatusTag status={status} />,
    },
    {
      title: '优先级',
      dataIndex: 'priority',
      key: 'priority',
      width: 80,
    },
    {
      title: '超时(秒)',
      dataIndex: 'timeout_sec',
      key: 'timeout_sec',
      width: 100,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (val: string) => dayjs(val).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: '操作',
      key: 'actions',
      width: 180,
      render: (_: unknown, record: Task) => {
        const isTerminal = terminalStatuses.includes(record.status);
        return (
          <Space size="small">
            <Button
              type="link"
              size="small"
              icon={<EyeOutlined />}
              onClick={(e) => { e.stopPropagation(); navigate(`/tasks/${record.task_uid}`); }}
            >
              查看
            </Button>
            {!isTerminal && (
              <Button
                type="link"
                size="small"
                danger
                icon={<CloseCircleOutlined />}
                loading={cancelMutation.isPending}
                onClick={(e) => { e.stopPropagation(); handleCancel(record.task_uid); }}
              >
                取消
              </Button>
            )}
            {isTerminal && (
              <Button
                type="link"
                size="small"
                icon={<ReloadOutlined />}
                loading={rerunMutation.isPending}
                onClick={(e) => { e.stopPropagation(); handleRerun(record.task_uid); }}
              >
                重跑
              </Button>
            )}
          </Space>
        );
      },
    },
  ];

  if (isError) {
    return (
      <div>
        <Typography.Title level={3} style={{ marginTop: 0 }}>任务列表</Typography.Title>
        <Result status="error" title="加载失败" subTitle={String(error)} />
      </div>
    );
  }

  return (
    <div>
      <Typography.Title level={3} style={{ marginTop: 0 }}>任务列表</Typography.Title>

      <div style={{ marginBottom: 16 }}>
        <Select
          value={statusFilter}
          onChange={(val) => { setStatusFilter(val); setPage(1); }}
          options={statusOptions}
          style={{ width: 160 }}
        />
      </div>

      <Table
        dataSource={tasks}
        columns={columns}
        rowKey="task_uid"
        loading={isLoading}
        pagination={{
          current: page,
          pageSize: size,
          total,
          onChange: (p) => setPage(p),
          showSizeChanger: false,
          showTotal: (t) => `共 ${t} 条`,
        }}
        size="middle"
        onRow={(record) => ({
          onClick: () => navigate(`/tasks/${record.task_uid}`),
          style: { cursor: 'pointer' },
        })}
      />
    </div>
  );
}
