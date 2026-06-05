import { useQuery } from '@tanstack/react-query';
import { statsApi } from '../api/stats';

export function useStats() {
  return useQuery({
    queryKey: ['stats'],
    queryFn: () => statsApi.get().then(r => r.data),
  });
}

export function useDailyStats(date?: string) {
  return useQuery({
    queryKey: ['stats', 'daily', date],
    queryFn: () => statsApi.getDaily(date).then(r => r.data),
  });
}
