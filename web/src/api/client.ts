import axios from 'axios';
import { message } from 'antd';

const client = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api/v1',
  timeout: 10000,
  headers: { 'Content-Type': 'application/json' },
});

client.interceptors.response.use(
  (response) => response,
  (error) => {
    let msg = '请求失败';
    if (error.response) {
      const status = error.response.status;
      const serverMsg = error.response.data?.error;
      if (status === 429) msg = '请求过于频繁，请稍后重试';
      else if (status === 500) msg = serverMsg || '服务器内部错误';
      else if (status === 404) msg = '资源不存在';
      else msg = serverMsg || `请求错误 (${status})`;
    } else if (error.request) {
      msg = '网络连接失败，请检查后端服务是否启动';
    }
    message.error(msg);
    return Promise.reject(error);
  }
);

export default client;
