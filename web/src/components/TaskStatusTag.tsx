import { Tag } from 'antd';
import type { Task } from '../types';

const statusConfig: Record<Task['status'], { color: string; label: string }> = {
  created: { color: 'default', label: '已创建' },
  pending: { color: 'blue', label: '等待中' },
  running: { color: 'processing', label: '运行中' },
  success: { color: 'green', label: '成功' },
  failed: { color: 'red', label: '失败' },
  canceled: { color: 'orange', label: '已取消' },
};

export default function TaskStatusTag({ status }: { status: Task['status'] }) {
  const config = statusConfig[status] || { color: 'default', label: status };
  return <Tag color={config.color}>{config.label}</Tag>;
}
