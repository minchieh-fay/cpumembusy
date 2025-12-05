package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// SystemStats 系统资源统计
type SystemStats struct {
	CPUPercent    float64 // CPU 使用率百分比
	MemoryPercent float64 // 内存使用率百分比
	TotalMemory   uint64  // 总内存（字节）
	UsedMemory   uint64  // 已用内存（字节）
}

var (
	lastCPUStats = make(map[string]uint64)
	lastCPUTime  time.Time
)

// GetSystemStats 获取系统资源使用情况
func GetSystemStats() (*SystemStats, error) {
	stats := &SystemStats{}

	// 获取内存信息
	if err := getMemoryStats(stats); err != nil {
		return nil, fmt.Errorf("获取内存信息失败: %w", err)
	}

	// 获取 CPU 信息
	if err := getCPUStats(stats); err != nil {
		return nil, fmt.Errorf("获取 CPU 信息失败: %w", err)
	}

	return stats, nil
}

// getMemoryStats 从 /proc/meminfo 获取内存信息
func getMemoryStats(stats *SystemStats) error {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return err
	}
	defer file.Close()

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := fields[0]
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		// 转换为字节（meminfo 中的单位是 KB）
		value *= 1024

		switch key {
		case "MemTotal:":
			memTotal = value
		case "MemAvailable:":
			memAvailable = value
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if memTotal == 0 {
		return fmt.Errorf("无法获取总内存信息")
	}

	stats.TotalMemory = memTotal
	stats.UsedMemory = memTotal - memAvailable
	stats.MemoryPercent = float64(stats.UsedMemory) / float64(memTotal) * 100

	return nil
}

// getCPUStats 从 /proc/stat 获取 CPU 使用率
func getCPUStats(stats *SystemStats) error {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return fmt.Errorf("无法读取 /proc/stat")
	}

	line := scanner.Text()
	fields := strings.Fields(line)
	if len(fields) < 8 || fields[0] != "cpu" {
		return fmt.Errorf("无效的 CPU 统计信息")
	}

	// 解析 CPU 时间
	cpuTimes := make(map[string]uint64)
	for i := 1; i < len(fields); i++ {
		val, err := strconv.ParseUint(fields[i], 10, 64)
		if err != nil {
			continue
		}
		cpuTimes[fmt.Sprintf("field%d", i-1)] = val
	}

	// 计算总 CPU 时间
	var totalTime uint64
	for _, v := range cpuTimes {
		totalTime += v
	}

	now := time.Now()
	if lastCPUTime.IsZero() {
		// 第一次调用，保存状态
		lastCPUStats = cpuTimes
		lastCPUTime = now
		stats.CPUPercent = 0
		return nil
	}

	// 计算时间差
	elapsed := now.Sub(lastCPUTime).Seconds()
	if elapsed <= 0 {
		stats.CPUPercent = 0
		return nil
	}

	// 计算空闲时间（field3 是 idle，field4 是 iowait）
	idle := cpuTimes["field3"] + cpuTimes["field4"]
	lastIdle := lastCPUStats["field3"] + lastCPUStats["field4"]

	idleDelta := idle - lastIdle
	lastTotal := uint64(0)
	for _, v := range lastCPUStats {
		lastTotal += v
	}
	totalDelta := totalTime - lastTotal

	if totalDelta == 0 {
		stats.CPUPercent = 0
	} else {
		// CPU 使用率 = (总时间 - 空闲时间) / 总时间 * 100
		usedPercent := float64(totalDelta-idleDelta) / float64(totalDelta) * 100
		stats.CPUPercent = usedPercent
	}

	// 更新状态
	lastCPUStats = cpuTimes
	lastCPUTime = now

	return nil
}

