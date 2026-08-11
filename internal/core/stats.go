package core

import "sync/atomic"

// Stats 是客户端运行统计快照,跨 H1/H2/H3 一致。
type Stats struct {
	// TotalRequests 累计请求尝试次数。
	TotalRequests uint64
	// ActiveRequests 当前活跃(未返回)请求数。
	ActiveRequests uint64
	// TotalErrors 累计请求错误次数。
	TotalErrors uint64
	// Retries 累计重试次数(首次尝试不计)。
	Retries uint64
}

// clientStats 是原子计数器,保证并发安全。
type clientStats struct {
	totalRequests  atomic.Uint64
	activeRequests atomic.Uint64
	totalErrors    atomic.Uint64
	retries        atomic.Uint64
}

// Stats 返回客户端运行统计快照。
func (c *Client) Stats() Stats {
	return Stats{
		TotalRequests:  c.stats.totalRequests.Load(),
		ActiveRequests: c.stats.activeRequests.Load(),
		TotalErrors:    c.stats.totalErrors.Load(),
		Retries:        c.stats.retries.Load(),
	}
}
