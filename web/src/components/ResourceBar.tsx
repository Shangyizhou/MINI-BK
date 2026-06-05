import { Progress } from 'antd';

interface ResourceBarProps {
  percent: number;
  label: string;
}

export default function ResourceBar({ percent, label }: ResourceBarProps) {
  const status = percent >= 90 ? 'exception' : percent >= 70 ? 'active' : 'normal';
  return (
    <div style={{ marginBottom: 8 }}>
      <div style={{ marginBottom: 2, fontSize: 12, color: '#666' }}>{label}</div>
      <Progress percent={Math.round(percent)} size="small" status={status} />
    </div>
  );
}
