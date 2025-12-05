package main

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// CPUController CPU 控制器
type CPUController struct {
	mu     sync.Mutex // 用于保护 ctx 和 cancel
	count  uint64     // 每次 sleep 前执行的计算次数（使用 atomic 保护）
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

const (
	sleepTime = 1 * time.Millisecond // 固定 sleep 时间：1ms
	initCount = 10000                // 初始计算次数
)

var cpuController = &CPUController{
	count: initCount, // 初始值：10000
}

// Start 启动 CPU 占用协程
func (cc *CPUController) Start() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.ctx != nil {
		// 已经启动
		return
	}

	cc.ctx, cc.cancel = context.WithCancel(context.Background())

	// 获取 CPU 核心数
	numCPU := runtime.NumCPU()
	if numCPU <= 0 {
		numCPU = 4 // 默认 4 核
	}

	// 为每个核心启动一个协程
	for i := 0; i < numCPU; i++ {
		cc.wg.Add(1)
		go cc.cpuWorker(i)
	}
}

// Stop 停止 CPU 占用协程
func (cc *CPUController) Stop() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.cancel != nil {
		cc.cancel()
		cc.wg.Wait()
		cc.ctx = nil
		cc.cancel = nil
	}
}

// cpuWorker CPU 工作协程
func (cc *CPUController) cpuWorker(id int) {
	defer cc.wg.Done()

	// 简单的计算密集型任务
	var counter uint64
	for {
		select {
		case <-cc.ctx.Done():
			return
		default:
			// 执行一些计算
			counter++

			// 获取当前 count 值（使用 atomic 读取，无需加锁）
			count := atomic.LoadUint64(&cc.count)

			if counter%count == 0 {
				// 每 count 次计算后 sleep 1ms
				time.Sleep(sleepTime)
			}
		}
	}
}

// AdjustCountRandom 根据随机方向调整计算次数
// shouldIncrease: true=增加占用（增加 count），false=减少占用（减少 count）
// 返回：是否成功调整，调整的方向（true=增加占用，false=减少占用），新的 count 值
func (cc *CPUController) AdjustCountRandom(shouldIncrease bool) (bool, bool, uint64) {
	var newCount uint64

	// 使用 atomic 读取当前值
	currentCount := atomic.LoadUint64(&cc.count)

	if shouldIncrease {
		// 增加 CPU 占用，增加 count
		// count = count * (1 + 0.1%) = count * 1.001
		newCount = uint64(float64(currentCount) * 1.001)
	} else {
		// 减少 CPU 占用，减少 count
		// count = count * (1 - 0.1%) = count * 0.999
		newCount = uint64(float64(currentCount) * 0.999)
		// 确保 count 不会小于 1
		if newCount < 1 {
			newCount = 1
		}
	}

	// 使用 atomic 写入新值
	atomic.StoreUint64(&cc.count, newCount)
	return true, shouldIncrease, newCount
}

// GetCount 获取当前计算次数
func (cc *CPUController) GetCount() uint64 {
	return atomic.LoadUint64(&cc.count)
}
