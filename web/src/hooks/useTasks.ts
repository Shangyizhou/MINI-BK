import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { tasksApi } from '../api/tasks';
import type { CreateTaskRequest } from '../types';

export function useTasks(params?: { status?: string; page?: number; size?: number; node_id?: string }) {
  return useQuery({
    queryKey: ['tasks', params],
    queryFn: () => tasksApi.list(params).then(r => r.data),
    refetchInterval: params?.status === 'running' ? 5000 : false,
  });
}

export function useTask(taskUid: string) {
  return useQuery({
    queryKey: ['tasks', taskUid],
    queryFn: () => tasksApi.get(taskUid).then(r => r.data),
  });
}

export function useCreateTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateTaskRequest) => tasksApi.create(data).then(r => r.data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  });
}

export function useCancelTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (taskUid: string) => tasksApi.cancel(taskUid),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  });
}

export function useRerunTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (taskUid: string) => tasksApi.rerun(taskUid).then(r => r.data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  });
}
