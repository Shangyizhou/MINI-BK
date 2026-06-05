import { useState } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { Layout, Menu, Typography, theme } from 'antd';
import {
  DashboardOutlined,
  UnorderedListOutlined,
  PlusCircleOutlined,
  CloudServerOutlined,
} from '@ant-design/icons';

const { Header, Sider, Content, Footer } = Layout;

const menuItems = [
  { key: '/', icon: <DashboardOutlined />, label: '仪表盘' },
  {
    key: 'tasks',
    icon: <UnorderedListOutlined />,
    label: '任务管理',
    children: [
      { key: '/tasks', label: '任务列表' },
      { key: '/tasks/new', icon: <PlusCircleOutlined />, label: '创建任务' },
    ],
  },
  { key: '/nodes', icon: <CloudServerOutlined />, label: '节点管理' },
];

export default function AppLayout() {
  const [collapsed, setCollapsed] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();
  const { token } = theme.useToken();

  const selectedKey = location.pathname === '/tasks/new' ? '/tasks' : location.pathname;

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider collapsible collapsed={collapsed} onCollapse={setCollapsed}>
        <div style={{ height: 48, display: 'flex', alignItems: 'center', justifyContent: 'center', color: token.colorWhite, fontWeight: 700, fontSize: collapsed ? 14 : 18 }}>
          {collapsed ? 'MB' : 'Mini-BK'}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selectedKey]}
          defaultOpenKeys={['tasks']}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>
      <Layout>
        <Header style={{ background: token.colorBgContainer, padding: '0 24px', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <Typography.Title level={4} style={{ margin: 0 }}>Mini-BK Console</Typography.Title>
        </Header>
        <Content style={{ margin: 16, padding: 24, background: token.colorBgContainer, borderRadius: token.borderRadiusLG, minHeight: 280, overflow: 'auto' }}>
          <Outlet />
        </Content>
        <Footer style={{ textAlign: 'center', padding: '8px 50px', fontSize: 12, color: token.colorTextSecondary }}>
          Mini-BK ResourceOps v0.4.0
        </Footer>
      </Layout>
    </Layout>
  );
}
