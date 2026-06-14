// SSHConnCache：一轮采集（tick）内按目标复用 SSH 连接。
//
// 同一目标可能被多个采集任务引用（如同一主机的 host.basic 与 host.disk），
// 若每个任务各自拨号，目标越多连接开销越大。该缓存让一轮内每个目标只拨号一次，
// tick 结束统一关闭。连接失败也会缓存，避免对不可达目标反复重试拨号。

package monitor

import (
	"sync"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// cachedConn 缓存一次拨号结果（成功的连接或失败的错误）。
type cachedConn struct {
	client *clients.LinuxSshClient
	err    error
}

// SSHConnCache 按 targetID（环境配置 ID）缓存 SSH 连接，生命周期 = 一轮 tick。
type SSHConnCache struct {
	env   core.EnvironmentDef
	mu    sync.Mutex
	conns map[string]*cachedConn
}

// NewSSHConnCache 创建与某环境绑定的连接缓存。
func NewSSHConnCache(env core.EnvironmentDef) *SSHConnCache {
	return &SSHConnCache{env: env, conns: map[string]*cachedConn{}}
}

// Get 返回 targetID 对应的 SSH 连接，首次调用时拨号并缓存（成功或失败都缓存）。
func (c *SSHConnCache) Get(targetID string) (*clients.LinuxSshClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cc, ok := c.conns[targetID]; ok {
		return cc.client, cc.err
	}
	cc := &cachedConn{}
	fields, err := sshFieldsByTarget(c.env, targetID)
	if err != nil {
		cc.err = err
		c.conns[targetID] = cc
		return nil, err
	}
	dial, err := clients.ParseLinuxSshDialConfig(fields)
	if err != nil {
		cc.err = err
		c.conns[targetID] = cc
		return nil, err
	}
	cc.client, cc.err = dial.Dial()
	c.conns[targetID] = cc
	return cc.client, cc.err
}

// Close 关闭本轮打开的所有连接。
func (c *SSHConnCache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, cc := range c.conns {
		if cc.client != nil {
			_ = cc.client.Close()
		}
	}
	c.conns = map[string]*cachedConn{}
}
