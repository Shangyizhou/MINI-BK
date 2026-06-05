import { useParams, useNavigate } from 'react-router-dom';
import {
  Card,
  Row,
  Col,
  Statistic,
  Progress,
  Tag,
  Button,
  Space,
  Spin,
  Typography,
  Popconfirm,
  Table,
  message,
  Empty,
  Descriptions,
} from 'antd';
import {
  ArrowLeftOutlined,
  CloudDownloadOutlined,
  CloudUploadOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import 'dayjs/locale/zh-cn';
import { useNode, useDrainNode, useUncordonNode } from '../../../hooks/useNodes';
import { useTasks } from '../../../hooks/useTasks';
import TaskStatusTag from '../../../components/TaskStatusTag';
import type { Task } from '../../../types';

dayjs.extend(relativeTime);
dayjs.locale('zh-cn');

export default function NodeDetail() {
  const { nodeId } = useParams<{ nodeId: string }>();
  const navigate = useNavigate();

  const { data: node, isLoading, isError } = useNode(nodeId!);
  const drainMutation = useDrainNode();
  const uncordonMutation = useUncordonNode();
  const { data: tasksData, isLoading: tasksLoading } = useTasks({
    node_id: nodeId,
    size: 50,
  });

  const handleDrain = () => {
    if (!nodeId) return;
    drainMutation.mutate(nodeId, {
      onSuccess: () => message.success('节点已设置为排空状态'),
      onError: (err) =>
        message.error(`操作失败: ${err instanceof Error ? err.message : '未知错误'}`),
    });
  };

  const handleUncordon = () => {
    if (!nodeId) return;
    uncordonMutation.mutate(nodeId, {
      onSuccess: () => message.success('节点已解除封锁'),
      onError: (err) =>
        message.error(`操作失败: ${err instanceof Error ? err.message : '未知错误'}`),
    });
  };

  if (isLoading) {
    return (
      <div style={{ textAlign: 'center', padding: 80 }}>
        <Spin size="large" />
      </div>
    );
  }

  if (isError || !node) {
    return (
      <div style={{ textAlign: 'center', padding: 80 }}>
        <Empty description="节点不存在或加载失败" />
        <Button
          type="primary"
          onClick={() => navigate('/nodes')}
          style={{ marginTop: 16 }}
        >
          返回节点列表
        </Button>
      </div>
    );
  }

  const memoryPercent =
    node.total_memory_mb > 0
      ? Math.round((node.memory_used_mb / node.total_memory_mb) * 100)
      : 0;
  const diskPercent =
    node.total_disk_mb > 0
      ? Math.round((node.disk_used_mb / node.total_disk_mb) * 100)
      : 0;

  const isDrainOrCordon = node.status === 'drain' || node.status === 'cordon';

  const runningTasks = tasksData?.tasks ?? [];

  const taskColumns = [
    {
      title: '任务 UID',
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
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: Task['status']) => <TaskStatusTag status={status} />,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (val: string) => dayjs(val).format('YYYY-MM-DD HH:mm:ss'),
    },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/nodes')}>
          返回
        </Button>
      </Space>

      <Typography.Title level={3} style={{ marginTop: 0 }}>
        <Space>
          <span>{node.hostname || node.node_id}</span>
          <Tag
            color={
              node.status === 'online'
                ? 'green'
                : node.status === 'offline'
                  ? 'default'
                  : node.status === 'drain'
                    ? 'orange'
                    : 'red'
            }
          >
            {node.status}
          </Tag>
        </Space>
      </Typography.Title>

      {/* Action Buttons */}
      <Card size="small" style={{ marginBottom: 16 }}>
        <Space>
          {!isDrainOrCordon && (
            <Popconfirm
              title="确认排空"
              description="将节点设置为排空状态，不再调度新任务？"
              onConfirm={handleDrain}
              okText="确定"
              cancelText="取消"
            >
              <Button
                danger
                icon={<CloudDownloadOutlined />}
                loading={drainMutation.isPending}
              >
                排空节点
              </Button>
            </Popconfirm>
          )}
          {isDrainOrCordon && (
            <Popconfirm
              title="确认解除"
              description="解除节点的排空/封锁状态？"
              onConfirm={handleUncordon}
              okText="确定"
              cancelText="取消"
            >
              <Button
                icon={<CloudUploadOutlined />}
                loading={uncordonMutation.isPending}
              >
                解除封锁
              </Button>
            </Popconfirm>
          )}
        </Space>
      </Card>

      {/* Node Basic Info */}
      <Card title="基本信息" size="small" style={{ marginBottom: 16 }}>
        <Descriptions column={2} size="small" bordered>
          <Descriptions.Item label="节点 ID">{node.node_id}</Descriptions.Item>
          <Descriptions.Item label="主机名">{node.hostname}</Descriptions.Item>
          <Descriptions.Item label="IP">{node.ip}</Descriptions.Item>
          <Descriptions.Item label="版本">{node.version}</Descriptions.Item>
          <Descriptions.Item label="运行任务数">{node.running_tasks}</Descriptions.Item>
          <Descriptions.Item label="注册时间">
            {node.registered_at ? dayjs(node.registered_at).format('YYYY-MM-DD HH:mm:ss') : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="最后心跳">
            {node.last_heartbeat_at ? dayjs(node.last_heartbeat_at).fromNow() : '-'}
          </Descriptions.Item>
        </Descriptions>
      </Card>

      {/* Resource Cards */}
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col span={6}>
          <Card size="small">
            <Typography.Text type="secondary">CPU 使用率</Typography.Text>
            <Progress
              percent={Math.round(node.cpu_usage_percent)}
              status={node.cpu_usage_percent >= 90 ? 'exception' : undefined}
              type="dashboard"
              size={120}
            />
            <div style={{ textAlign: 'center', marginTop: 8 }}>
              <Typography.Text type="secondary">
                总核数: {node.total_cpu}
              </Typography.Text>
            </div>
          </Card>
        </Col>
        <Col span={6}>
          <Card size="small">
            <Typography.Text type="secondary">内存使用率</Typography.Text>
            <Progress
              percent={memoryPercent}
              status={memoryPercent >= 90 ? 'exception' : undefined}
              type="dashboard"
              size={120}
            />
            <div style={{ textAlign: 'center', marginTop: 8 }}>
              <Typography.Text type="secondary">
                {node.memory_used_mb} / {node.total_memory_mb} MB
              </Typography.Text>
            </div>
          </Card>
        </Col>
        <Col span={6}>
          <Card size="small">
            <Typography.Text type="secondary">磁盘使用率</Typography.Text>
            <Progress
              percent={diskPercent}
              status={diskPercent >= 90 ? 'exception' : undefined}
              type="dashboard"
              size={120}
            />
            <div style={{ textAlign: 'center', marginTop: 8 }}>
              <Typography.Text type="secondary">
                {node.disk_used_mb} / {node.total_disk_mb} MB
              </Typography.Text>
            </div>
          </Card>
        </Col>
        <Col span={6}>
          <Card size="small">
            <Typography.Text type="secondary">负载 (1分钟)</Typography.Text>
            <div style={{ textAlign: 'center', padding: '20px 0' }}>
              <Statistic
                value={node.load_avg_1m}
                precision={2}
                suffix=""
              />
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                总核数: {node.total_cpu}
              </Typography.Text>
            </div>
          </Card>
        </Col>
      </Row>

      {/* Labels */}
      <Card title="标签" size="small" style={{ marginBottom: 16 }}>
        {node.labels && node.labels.length > 0 ? (
          node.labels.map((label) => (
            <Tag key={label} color="blue" style={{ marginBottom: 4 }}>
              {label}
            </Tag>
          ))
        ) : (
          <Typography.Text type="secondary">暂无标签</Typography.Text>
        )}
      </Card>

      {/* Running Tasks */}
      <Card title="运行任务" size="small">
        <Spin spinning={tasksLoading}>
          {runningTasks.length > 0 ? (
            <Table
              dataSource={runningTasks}
              columns={taskColumns}
              rowKey="task_uid"
              pagination={false}
              size="small"
              onRow={(record) => ({
                onClick: () => navigate(`/tasks/${record.task_uid}`),
                style: { cursor: 'pointer' },
              })}
            />
          ) : (
            <Empty description="该节点暂无任务" />
          )}
        </Spin>
      </Card>
    </div>
  );
}
