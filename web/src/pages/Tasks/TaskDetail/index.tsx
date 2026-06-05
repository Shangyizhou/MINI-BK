import { useParams, useNavigate } from 'react-router-dom';
import {
  Descriptions,
  Tabs,
  Button,
  Space,
  Spin,
  Typography,
  Popconfirm,
  message,
  Empty,
  Card,
} from 'antd';
import {
  ArrowLeftOutlined,
  CloseCircleOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { useTask, useCancelTask, useRerunTask } from '../../../hooks/useTasks';
import { useSSE } from '../../../hooks/useSSE';
import LogStream from '../../../components/LogStream';
import TaskStatusTag from '../../../components/TaskStatusTag';
import type { Task } from '../../../types';

const terminalStatuses: Task['status'][] = ['success', 'failed', 'canceled'];

export default function TaskDetail() {
  const { taskUid } = useParams<{ taskUid: string }>();
  const navigate = useNavigate();

  const { data: task, isLoading, isError } = useTask(taskUid!);
  const { logs, connected, clearLogs } = useSSE(
    task && !terminalStatuses.includes(task.status) ? taskUid : undefined,
  );
  const cancelMutation = useCancelTask();
  const rerunMutation = useRerunTask();

  const isTerminal = task ? terminalStatuses.includes(task.status) : false;

  const handleCancel = () => {
    if (!taskUid) return;
    cancelMutation.mutate(taskUid, {
      onSuccess: () => message.success('任务已取消'),
      onError: (err) =>
        message.error(`取消失败: ${err instanceof Error ? err.message : '未知错误'}`),
    });
  };

  const handleRerun = () => {
    if (!taskUid) return;
    rerunMutation.mutate(taskUid, {
      onSuccess: (data) => {
        message.success('任务已重跑');
        navigate(`/tasks/${data.task_uid}`);
      },
      onError: (err) =>
        message.error(`重跑失败: ${err instanceof Error ? err.message : '未知错误'}`),
    });
  };

  if (isLoading) {
    return (
      <div style={{ textAlign: 'center', padding: 80 }}>
        <Spin size="large" />
      </div>
    );
  }

  if (isError || !task) {
    return (
      <div style={{ textAlign: 'center', padding: 80 }}>
        <Empty description="任务不存在或加载失败" />
        <Button
          type="primary"
          onClick={() => navigate('/tasks')}
          style={{ marginTop: 16 }}
        >
          返回任务列表
        </Button>
      </div>
    );
  }

  const nodeSelectorEntries = task.node_selector
    ? Object.entries(task.node_selector)
    : [];

  const outputTabItems = [
    {
      key: 'stdout',
      label: '标准输出',
      children: (
        <pre
          style={{
            background: '#1e1e1e',
            color: '#d4d4d4',
            padding: 12,
            borderRadius: 4,
            fontSize: 13,
            lineHeight: 1.5,
            maxHeight: 400,
            overflow: 'auto',
            fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
          }}
        >
          {task.stdout || <span style={{ color: '#666' }}>无输出</span>}
        </pre>
      ),
    },
    {
      key: 'stderr',
      label: '标准错误',
      children: (
        <pre
          style={{
            background: '#1e1e1e',
            color: '#f44747',
            padding: 12,
            borderRadius: 4,
            fontSize: 13,
            lineHeight: 1.5,
            maxHeight: 400,
            overflow: 'auto',
            fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
          }}
        >
          {task.stderr || <span style={{ color: '#666' }}>无输出</span>}
        </pre>
      ),
    },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/tasks')}>
          返回
        </Button>
      </Space>

      <Typography.Title level={3} style={{ marginTop: 0 }}>
        <Space>
          <span>{task.name}</span>
          <TaskStatusTag status={task.status} />
        </Space>
      </Typography.Title>

      {/* Action Buttons */}
      <Card size="small" style={{ marginBottom: 16 }}>
        <Space>
          {!isTerminal && (
            <Popconfirm
              title="确认取消"
              description="确定要取消这个任务吗？"
              onConfirm={handleCancel}
              okText="确定"
              cancelText="取消"
            >
              <Button
                danger
                icon={<CloseCircleOutlined />}
                loading={cancelMutation.isPending}
              >
                取消任务
              </Button>
            </Popconfirm>
          )}
          {isTerminal && (
            <Popconfirm
              title="确认重跑"
              description="确定要重新运行这个任务吗？"
              onConfirm={handleRerun}
              okText="确定"
              cancelText="取消"
            >
              <Button
                icon={<ReloadOutlined />}
                loading={rerunMutation.isPending}
              >
                重新运行
              </Button>
            </Popconfirm>
          )}
        </Space>
      </Card>

      {/* Task Metadata */}
      <Card title="任务信息" size="small" style={{ marginBottom: 16 }}>
        <Descriptions column={2} size="small" bordered>
          <Descriptions.Item label="任务 UID">{task.task_uid}</Descriptions.Item>
          <Descriptions.Item label="命令">{task.command}</Descriptions.Item>
          <Descriptions.Item label="工作目录">{task.workdir}</Descriptions.Item>
          <Descriptions.Item label="状态">
            <TaskStatusTag status={task.status} />
          </Descriptions.Item>
          {task.exit_code !== null && (
            <Descriptions.Item label="退出码">{task.exit_code}</Descriptions.Item>
          )}
          <Descriptions.Item label="优先级">{task.priority}</Descriptions.Item>
          <Descriptions.Item label="最大重试次数">{task.max_retries}</Descriptions.Item>
          <Descriptions.Item label="已重试次数">{task.retry_count}</Descriptions.Item>
          <Descriptions.Item label="超时时间">{task.timeout_sec}秒</Descriptions.Item>
          {task.cpu_limit > 0 && (
            <Descriptions.Item label="CPU 限制">{task.cpu_limit} 核</Descriptions.Item>
          )}
          {task.memory_limit > 0 && (
            <Descriptions.Item label="内存限制">{task.memory_limit} MB</Descriptions.Item>
          )}
          {task.assigned_node_id && (
            <Descriptions.Item label="分配节点">{task.assigned_node_id}</Descriptions.Item>
          )}
          {nodeSelectorEntries.length > 0 && (
            <Descriptions.Item label="节点选择器">
              {nodeSelectorEntries.map(([k, v]) => (
                <div key={k}>
                  {k}: {v}
                </div>
              ))}
            </Descriptions.Item>
          )}
          <Descriptions.Item label="创建时间">
            {task.created_at ? dayjs(task.created_at).format('YYYY-MM-DD HH:mm:ss') : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="开始时间">
            {task.started_at ? dayjs(task.started_at).format('YYYY-MM-DD HH:mm:ss') : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="完成时间">
            {task.finished_at ? dayjs(task.finished_at).format('YYYY-MM-DD HH:mm:ss') : '-'}
          </Descriptions.Item>
        </Descriptions>
      </Card>

      {/* Output tabs — only for terminal tasks */}
      {isTerminal && (
        <Card title="任务输出" size="small" style={{ marginBottom: 16 }}>
          <Tabs items={outputTabItems} />
        </Card>
      )}

      {/* Real-time logs — shown for non-terminal tasks */}
      {!isTerminal && (
        <Card size="small" style={{ marginBottom: 16 }}>
          <LogStream logs={logs} connected={connected} onClear={clearLogs} />
        </Card>
      )}
    </div>
  );
}
