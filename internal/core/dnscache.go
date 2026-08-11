package core

import (
	"context"
	"net"
	"sync"
	"time"
)

// defaultDNSTTL 是 DNS 缓存的默认有效期。
const defaultDNSTTL = 60 * time.Second

// ipResolver 抽象主机名解析,便于测试注入。
type ipResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// dnsEntry 是单个主机的缓存条目。
type dnsEntry struct {
	ips    []net.IPAddr
	expire time.Time
}

// DNSCache 是按 TTL 缓存主机解析结果的线程安全解析器包装。
// 解析失败不缓存,拨号层在失败时回退系统解析。
type DNSCache struct {
	ttl      time.Duration
	resolver ipResolver

	mu      sync.Mutex
	entries map[string]dnsEntry
}

// NewDNSCache 创建 DNS 缓存;ttl <= 0 时使用默认值(60s)。
func NewDNSCache(ttl time.Duration) *DNSCache {
	if ttl <= 0 {
		ttl = defaultDNSTTL
	}
	return &DNSCache{
		ttl:      ttl,
		resolver: net.DefaultResolver,
		entries:  make(map[string]dnsEntry),
	}
}

// LookupIPAddr 返回主机解析结果;缓存未过期时直接命中,
// 否则调用底层 resolver 并更新缓存。
func (c *DNSCache) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	now := time.Now()
	c.mu.Lock()
	entry, ok := c.entries[host]
	c.mu.Unlock()
	if ok && now.Before(entry.expire) {
		return entry.ips, nil
	}
	ips, err := c.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.entries[host] = dnsEntry{ips: ips, expire: now.Add(c.ttl)}
	c.mu.Unlock()
	return ips, nil
}

// Reset 清空全部缓存条目。
func (c *DNSCache) Reset() {
	c.mu.Lock()
	c.entries = make(map[string]dnsEntry)
	c.mu.Unlock()
}
