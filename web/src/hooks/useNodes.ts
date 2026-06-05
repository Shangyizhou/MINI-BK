import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { nodesApi } from '../api/nodes';

export function useNode(nodeId: string) {
  return useQuery({
    queryKey: ['nodes', nodeId],
    queryFn: () => nodesApi.get(nodeId).then(r => r.data),
  });
}

export function useNodes(params?: { status?: string }) {
  return useQuery({
    queryKey: ['nodes', params],
    queryFn: () => nodesApi.list(params).then(r => r.data),
    refetchInterval: 10000,
  });
}

export function useDrainNode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (nodeId: string) => nodesApi.drain(nodeId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['nodes'] }),
  });
}

export function useUncordonNode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (nodeId: string) => nodesApi.uncordon(nodeId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['nodes'] }),
  });
}
