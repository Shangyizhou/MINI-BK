package config

import (
	"context"
	"log/slog"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type ConfigChangeCallback func(key string, value []byte)

type ConfigWatcher struct {
	client *clientv3.Client
}

func NewConfigWatcher(client *clientv3.Client) *ConfigWatcher {
	return &ConfigWatcher{client: client}
}

// Watch watches for changes on a prefix and calls callback on each change
func (cw *ConfigWatcher) Watch(ctx context.Context, prefix string, callback ConfigChangeCallback) {
	if cw == nil || cw.client == nil {
		slog.Warn("ConfigWatcher: etcd client not available, skipping watch", "prefix", prefix)
		return
	}

	// Initial load
	resp, err := cw.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err == nil {
		for _, kv := range resp.Kvs {
			callback(string(kv.Key), kv.Value)
		}
	}

	// Watch for changes
	var watchRev int64
	if resp != nil {
		watchRev = resp.Header.Revision + 1
	}

	watchCh := cw.client.Watch(ctx, prefix, clientv3.WithPrefix(), clientv3.WithRev(watchRev))
	go func() {
		for resp := range watchCh {
			for _, ev := range resp.Events {
				if ev.Type == clientv3.EventTypePut {
					callback(string(ev.Kv.Key), ev.Kv.Value)
				}
			}
		}
	}()
	slog.Info("配置热更新已启动", "prefix", prefix)
}

// UpdateConfig publishes a config value to etcd
func (cw *ConfigWatcher) UpdateConfig(ctx context.Context, key, value string) error {
	if cw == nil || cw.client == nil {
		return nil
	}
	_, err := cw.client.Put(ctx, key, value)
	return err
}
