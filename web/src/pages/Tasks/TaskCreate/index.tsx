import { useNavigate } from 'react-router-dom';
import {
  Form,
  Input,
  InputNumber,
  Button,
  Row,
  Col,
  message,
  Typography,
  Card,
} from 'antd';
import { PlusOutlined, MinusCircleOutlined } from '@ant-design/icons';
import { useCreateTask } from '../../../hooks/useTasks';

interface FormValues {
  name: string;
  command: string;
  workdir: string;
  cpu_limit?: number;
  memory_limit?: number;
  timeout_sec?: number;
  priority?: number;
  node_selector?: { key: string; value: string }[];
}

export default function TaskCreate() {
  const navigate = useNavigate();
  const [form] = Form.useForm<FormValues>();
  const createMutation = useCreateTask();

  const handleFinish = (values: FormValues) => {
    const nodeSelector: Record<string, string> = {};
    if (values.node_selector) {
      values.node_selector.forEach((item) => {
        if (item.key) {
          nodeSelector[item.key] = item.value;
        }
      });
    }

    createMutation.mutate(
      {
        name: values.name,
        command: values.command,
        workdir: values.workdir || '/tmp',
        cpu_limit: values.cpu_limit,
        memory_limit: values.memory_limit,
        timeout_sec: values.timeout_sec ?? 300,
        priority: values.priority ?? 0,
        node_selector: Object.keys(nodeSelector).length > 0 ? nodeSelector : undefined,
      },
      {
        onSuccess: (data) => {
          message.success('任务创建成功');
          navigate(`/tasks/${data.task_uid}`);
        },
        onError: (err: Error) => {
          message.error(`创建失败: ${err.message}`);
        },
      },
    );
  };

  return (
    <div>
      <Typography.Title level={3} style={{ marginTop: 0 }}>创建任务</Typography.Title>

      <Card>
        <Form
          form={form}
          layout="vertical"
          onFinish={handleFinish}
          initialValues={{
            workdir: '/tmp',
            timeout_sec: 300,
            priority: 0,
          }}
          style={{ maxWidth: 900 }}
        >
          <Row gutter={24}>
            {/* Left Column */}
            <Col span={12}>
              <Form.Item
                name="name"
                label="任务名称"
                rules={[{ required: true, message: '请输入任务名称' }]}
              >
                <Input placeholder="输入任务名称" />
              </Form.Item>

              <Form.Item
                name="command"
                label="命令"
                rules={[{ required: true, message: '请输入执行命令' }]}
              >
                <Input.TextArea rows={4} placeholder="输入要执行的命令" />
              </Form.Item>

              <Form.Item name="workdir" label="工作目录">
                <Input placeholder="/tmp" />
              </Form.Item>
            </Col>

            {/* Right Column */}
            <Col span={12}>
              <Form.Item name="cpu_limit" label="CPU 限制 (核)">
                <InputNumber min={0} step={0.5} style={{ width: '100%' }} placeholder="不限制" />
              </Form.Item>

              <Form.Item name="memory_limit" label="内存限制">
                <InputNumber
                  min={0}
                  addonAfter="MB"
                  style={{ width: '100%' }}
                  placeholder="不限制"
                />
              </Form.Item>

              <Form.Item name="timeout_sec" label="超时时间">
                <InputNumber
                  min={1}
                  addonAfter="秒"
                  style={{ width: '100%' }}
                />
              </Form.Item>

              <Form.Item name="priority" label="优先级">
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>

          {/* Node Selector Section */}
          <Typography.Title level={5} style={{ marginTop: 16 }}>节点选择器</Typography.Title>
          <Form.List name="node_selector">
            {(fields, { add, remove }) => (
              <div>
                {fields.map(({ key, name, ...restField }) => (
                  <Row key={key} gutter={8} style={{ marginBottom: 8 }} align="middle">
                    <Col span={8}>
                      <Form.Item
                        {...restField}
                        name={[name, 'key']}
                        rules={[{ required: true, message: '请输入键' }]}
                        style={{ marginBottom: 0 }}
                      >
                        <Input placeholder="键" />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item
                        {...restField}
                        name={[name, 'value']}
                        rules={[{ required: true, message: '请输入值' }]}
                        style={{ marginBottom: 0 }}
                      >
                        <Input placeholder="值" />
                      </Form.Item>
                    </Col>
                    <Col span={4}>
                      <MinusCircleOutlined
                        onClick={() => remove(name)}
                        style={{ fontSize: 16, color: '#ff4d4f' }}
                      />
                    </Col>
                  </Row>
                ))}
                <Button
                  type="dashed"
                  onClick={() => add()}
                  icon={<PlusOutlined />}
                  style={{ width: '100%' }}
                >
                  添加节点选择器
                </Button>
              </div>
            )}
          </Form.List>

          <Form.Item style={{ marginTop: 24 }}>
            <Button
              type="primary"
              htmlType="submit"
              loading={createMutation.isPending}
              size="large"
            >
              创建任务
            </Button>
            <Button
              style={{ marginLeft: 12 }}
              onClick={() => navigate('/tasks')}
              size="large"
            >
              返回
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
}
