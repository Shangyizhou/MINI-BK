import { createBrowserRouter } from 'react-router-dom';
import { lazy } from 'react';
import AppLayout from './components/Layout/AppLayout';
import ErrorBoundary from './components/ErrorBoundary';

const Dashboard = lazy(() => import('./pages/Dashboard'));
const TaskList = lazy(() => import('./pages/Tasks/TaskList'));
const TaskCreate = lazy(() => import('./pages/Tasks/TaskCreate'));
const TaskDetail = lazy(() => import('./pages/Tasks/TaskDetail'));
const NodeList = lazy(() => import('./pages/Nodes/NodeList'));
const NodeDetail = lazy(() => import('./pages/Nodes/NodeDetail'));

export const router = createBrowserRouter([
  {
    element: <ErrorBoundary><AppLayout /></ErrorBoundary>,
    children: [
      { path: '/', element: <Dashboard /> },
      { path: '/tasks', element: <TaskList /> },
      { path: '/tasks/new', element: <TaskCreate /> },
      { path: '/tasks/:taskUid', element: <TaskDetail /> },
      { path: '/nodes', element: <NodeList /> },
      { path: '/nodes/:nodeId', element: <NodeDetail /> },
    ],
  },
]);
