import client from './client';
import type { Node } from '../types';

export const nodesApi = {
  list: (params?: { status?: string }) => client.get<Node[]>('/nodes', { params }),
  get: (nodeId: string) => client.get<Node>(`/nodes/${nodeId}`),
  drain: (nodeId: string) => client.post(`/nodes/${nodeId}/drain`),
  uncordon: (nodeId: string) => client.post(`/nodes/${nodeId}/uncordon`),
};
