import client from './client';
import type { DailyStats } from '../types';

export const statsApi = {
  get: () => client.get('/stats'),
  getDaily: (date?: string) =>
    client.get<DailyStats>('/stats/daily', { params: date ? { date } : {} }),
};
