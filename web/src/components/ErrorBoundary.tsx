import { Component, type ReactNode } from 'react';
import { Result, Button } from 'antd';

interface Props { children: ReactNode; }
interface State { hasError: boolean; error?: Error; }

export default class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(error: Error) {
    return { hasError: true, error };
  }

  render() {
    if (this.state.hasError) {
      return (
        <Result
          status="error"
          title="页面发生错误"
          subTitle={this.state.error?.message}
          extra={
            <Button type="primary" onClick={() => { this.setState({ hasError: false }); window.location.reload(); }}>
              刷新页面
            </Button>
          }
        />
      );
    }
    return this.props.children;
  }
}
