import { Card, Col, Row, Statistic, Table, Spin, Empty, Typography, Progress, Result } from 'antd';
import { CheckCircleOutlined, CloseCircleOutlined, FileTextOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import dayjs from 'dayjs';
import { useTasks } from '../../hooks/useTasks';
import { useNodes } from '../../hooks/useNodes';
import { useStats } from '../../hooks/useStats';
import TaskStatusTag from '../../components/TaskStatusTag';
import ResourceBar from '../../components/ResourceBar';
import type { Task } from '../../types';

export default function Dashboard() {
  const navigate = useNavigate();

  const { data: stats, isLoading: statsLoading, isError: statsError, error: statsErr } = useStats();
  const { data: tasksData, isLoading: tasksLoading, isError: tasksError, error: tasksErr } = useTasks({ size: 10 });
  const { data: nodes, isLoading: nodesLoading, isError: nodesError, error: nodesErr } = useNodes();

  const tasks = tasksData?.tasks ?? [];

  const totalTasks = stats?.total ?? 0;
  const successCount = stats?.success ?? 0;
  const failedCount = stats?.failed ?? 0;
  const successRate = totalTasks > 0 ? Math.round((successCount / totalTasks) * 100) : 0;

  const recentColumns = [
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
      render: (status: Task['status']) => <TaskStatusTag status={status} />,
    },
    {
      title: '优先级',
      dataIndex: 'priority',
      key: 'priority',
      width: 80,
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
      <Typography.Title level={3} style={{ marginTop: 0 }}>仪表盘</Typography.Title>

      {/* Error Banner */}
      {(statsError || tasksError || nodesError) && (
        <div style={{ marginBottom: 16 }}>
          <Result
            status="warning"
            title="部分数据加载失败"
            subTitle={
              [statsErr, tasksErr, nodesErr]
                .filter(Boolean)
                .map((e) => String(e))
                .join('; ')
            }
          />
        </div>
      )}

      {/* Stats Cards */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={8}>
          <Card loading={statsLoading}>
            <Statistic
              title="总提交任务"
              value={totalTasks}
              prefix={<FileTextOutlined />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card loading={statsLoading}>
            <Statistic
              title="成功率"
              value={successRate}
              suffix="%"
              prefix={<CheckCircleOutlined style={{ color: '#52c41a' }} />}
            />
            <Progress
              type="circle"
              percent={successRate}
              size={60}
              style={{ position: 'absolute', right: 24, top: 16 }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card loading={statsLoading}>
            <Statistic
              title="失败任务"
              value={failedCount}
              valueStyle={{ color: '#ff4d4f' }}
              prefix={<CloseCircleOutlined />}
            />
          </Card>
        </Col>
      </Row>

      {/* Recent Tasks */}
      <Typography.Title level={5}>最近任务</Typography.Title>
      <Spin spinning={tasksLoading}>
        {tasks.length > 0 ? (
          <Table
            dataSource={tasks}
            columns={recentColumns}
            rowKey="task_uid"
            pagination={false}
            size="small"
            onRow={(record) => ({
              onClick: () => navigate(`/tasks/${record.task_uid}`),
              style: { cursor: 'pointer' },
            })}
          />
        ) : (
          <Empty description="暂无任务" />
        )}
      </Spin>

      {/* Node Overview */}
      <Typography.Title level={5} style={{ marginTop: 24 }}>节点概览</Typography.Title>
      <Spin spinning={nodesLoading}>
        {nodes && nodes.length > 0 ? (
          <Row gutter={[16, 16]}>
            {nodes.map((node) => (
              <Col xs={24} sm={12} lg={8} key={node.node_id}>
                <Card
                  title={node.hostname || node.node_id}
                  hoverable
                  onClick={() => navigate(`/nodes/${node.node_id}`)}
                  size="small"
                >
                  <Typography.Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>
                    IP: {node.ip} | 状态: {node.status}
                  </Typography.Text>
                  <ResourceBar percent={node.cpu_usage_percent} label="CPU" />
                  <ResourceBar
                    percent={node.total_memory_mb > 0 ? Math.round((node.memory_used_mb / node.total_memory_mb) * 100) : 0}
                    label="内存"
                  />
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    运行任务: {node.running_tasks} | 负载: {node.load_avg_1m?.toFixed(2)}
                  </Typography.Text>
                </Card>
              </Col>
            ))}
          </Row>
        ) : (
          <Empty description="暂无节点" />
        )}
      </Spin>
    </div>
  );
}
