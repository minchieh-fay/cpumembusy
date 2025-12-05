package main

import (
	"sync"
)

// MemoryController 内存控制器
type MemoryController struct {
	mu          sync.RWMutex
	buffer      [][]byte // 内存缓冲区
	totalMemory uint64   // 整机总内存
}

var memoryController = &MemoryController{}

// AdjustMemoryRandom 根据随机方向调整内存占用
// shouldIncrease: true=增加，false=减少
// 返回：是否成功调整，调整的方向（true=增加，false=减少），调整的字节数
func (mc *MemoryController) AdjustMemoryRandom(shouldIncrease bool) (bool, bool, uint64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// 计算当前程序占用的内存
	currentProgramBytes := mc.getCurrentProgramMemory()

	// 计算需要调整的字节数（0.1% 的整机内存）
	adjustBytes := mc.totalMemory / 1000 // 0.1% = 1/1000

	var targetProgramBytes uint64

	if shouldIncrease {
		// 增加内存
		targetProgramBytes = currentProgramBytes + adjustBytes
	} else {
		// 减少内存
		if currentProgramBytes > adjustBytes {
			targetProgramBytes = currentProgramBytes - adjustBytes
		} else {
			targetProgramBytes = 0
		}
	}

	// 执行调整
	actualBytes := mc.adjustTo(targetProgramBytes)
	return true, shouldIncrease, actualBytes
}

// adjustTo 调整内存到目标大小
func (mc *MemoryController) adjustTo(targetBytes uint64) uint64 {
	currentBytes := mc.getCurrentProgramMemory()

	if targetBytes > currentBytes {
		// 增加内存
		needBytes := targetBytes - currentBytes
		mc.allocateMemory(needBytes)
	} else if targetBytes < currentBytes {
		// 减少内存
		releaseBytes := currentBytes - targetBytes
		mc.releaseMemory(releaseBytes)
	}

	return mc.getCurrentProgramMemory()
}

// allocateMemory 分配内存
func (mc *MemoryController) allocateMemory(bytes uint64) {
	// 每次分配 1MB 的块
	blockSize := uint64(1024 * 1024)              // 1MB
	blocks := (bytes + blockSize - 1) / blockSize // 向上取整

	for i := uint64(0); i < blocks; i++ {
		// 分配 1MB 的字节数组
		buf := make([]byte, blockSize)
		// 写入一些数据确保内存真正被分配
		for j := range buf {
			buf[j] = byte(j % 256)
		}
		mc.buffer = append(mc.buffer, buf)
	}
}

// releaseMemory 释放内存
func (mc *MemoryController) releaseMemory(bytes uint64) {
	// 每次释放 1MB 的块
	blockSize := uint64(1024 * 1024)              // 1MB
	blocks := (bytes + blockSize - 1) / blockSize // 向上取整

	if blocks > uint64(len(mc.buffer)) {
		blocks = uint64(len(mc.buffer))
	}

	// 从末尾释放
	if blocks > 0 {
		mc.buffer = mc.buffer[:len(mc.buffer)-int(blocks)]
	}
}

// getCurrentProgramMemory 获取当前程序占用的内存（字节）
func (mc *MemoryController) getCurrentProgramMemory() uint64 {
	blockSize := uint64(1024 * 1024) // 1MB
	return uint64(len(mc.buffer)) * blockSize
}

// SetTotalMemory 设置整机总内存
func (mc *MemoryController) SetTotalMemory(totalMemory uint64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.totalMemory = totalMemory
}

// GetCurrentMemory 获取当前程序占用的内存（字节）
func (mc *MemoryController) GetCurrentMemory() uint64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.getCurrentProgramMemory()
}
