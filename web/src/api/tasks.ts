import client from './client';
import type { Task, TaskListResult, CreateTaskRequest } from '../types';

export const tasksApi = {
  create: (data: CreateTaskRequest) => client.post<Task>('/tasks', data),
  list: (params?: { status?: string; page?: number; size?: number }) =>
    client.get<TaskListResult>('/tasks', { params }),
  get: (taskUid: string) => client.get<Task>(`/tasks/${taskUid}`),
  cancel: (taskUid: string) => client.post(`/tasks/${taskUid}/cancel`),
  rerun: (taskUid: string) => client.post(`/tasks/${taskUid}/rerun`),
  getLog: (taskUid: string) => client.get<{ stdout: string; stderr: string }>(`/tasks/${taskUid}/log`),
};
