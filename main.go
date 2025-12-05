package main

import (
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

const (
	defaultPeakUsage = 40
	hardPeakLimit    = 70
	minPeakUsage     = 5
)

var (
	logger *slog.Logger
)

func init() {
	// 初始化日志：使用 slog，输出到标准输出
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

var (
	peakUsageOrigin int          // 原始的 peakUsage 值
	peakUsage       int          // 当前浮动的 peakUsage 值
	peakUsageMu     sync.RWMutex // 保护 peakUsage 的读写锁
)

func main() {
	// 读取环境变量
	peakUsageOrigin = getPeakUsage()
	peakUsage = peakUsageOrigin
	logger.Info("程序启动", "peak_usage_origin", peakUsageOrigin, "peak_usage", peakUsage, "hard_peak_limit", hardPeakLimit)

	// 初始化系统资源监控
	stats, err := GetSystemStats()
	if err != nil {
		logger.Warn("初始化系统资源监控失败，使用保守策略", "error", err)
		stats = &SystemStats{}
	} else {
		memoryController.SetTotalMemory(stats.TotalMemory)
		logger.Info("系统资源初始化成功",
			"total_memory_gb", stats.TotalMemory/(1024*1024*1024),
			"cpu_cores", runtime.NumCPU())
	}

	// 启动 CPU 控制器
	cpuController.Start()
	defer cpuController.Stop()

	// 设置信号处理，优雅退出
	//sigChan := make(chan os.Signal, 1)
	//signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 主循环
	monitorTicker := time.NewTicker(3 * time.Second)
	defer monitorTicker.Stop()

	gcTicker := time.NewTicker(1 * time.Minute)
	defer gcTicker.Stop()

	// 每 5 分钟更新一次 peakUsage
	peakUsageTicker := time.NewTicker(5 * time.Minute)
	defer peakUsageTicker.Stop()

	lastStats := stats

	for {
		select {
		//case <-sigChan:
		//	logger.Info("收到退出信号，开始优雅退出...")
		//	return

		case <-gcTicker.C:
			// 每隔 1 分钟触发 GC
			runtime.GC()
			logger.Info("触发垃圾回收")

		case <-peakUsageTicker.C:
			// 每 5 分钟更新一次 peakUsage
			updatePeakUsage()

		case <-monitorTicker.C:
			// 获取系统资源信息
			currentStats, err := GetSystemStats()
			if err != nil {
				logger.Warn("获取系统资源信息失败，使用上次的值", "error", err)
				currentStats = lastStats
			} else {
				lastStats = currentStats
			}

			// 获取当前的 peakUsage（加读锁）
			peakUsageMu.RLock()
			currentPeakUsage := peakUsage
			peakUsageMu.RUnlock()

			// 计算期望占用值
			expectedUsage := calculateExpectedUsage(currentPeakUsage)
			isNightTime := isNightTime()

			// 打印资源监控信息
			logger.Info("系统资源监控",
				"cpu_percent", currentStats.CPUPercent,
				"memory_percent", currentStats.MemoryPercent,
				"expected_usage", expectedUsage,
				"is_night_time", isNightTime,
				"current_memory_mb", memoryController.GetCurrentMemory()/(1024*1024),
				"cpu_count", cpuController.GetCount())

			// 随机间隔 5-10 秒执行调整
			//adjustInterval := time.Duration(5+rand.Intn(6)) * time.Second
			//time.Sleep(adjustInterval)

			// 执行资源调整
			adjustResources(currentStats, expectedUsage)
		}
	}
}

// getPeakUsage 从环境变量获取峰值使用率
func getPeakUsage() int {
	// 尝试读取 P 或 p 环境变量（不区分大小写）
	value := os.Getenv("P")
	if value == "" {
		value = os.Getenv("p")
	}

	if value == "" {
		return defaultPeakUsage
	}

	peakUsage, err := strconv.Atoi(value)
	if err != nil {
		logger.Warn("环境变量 P 值无效，使用默认值", "value", value, "default", defaultPeakUsage)
		return defaultPeakUsage
	}

	// 验证范围
	if peakUsage < 1 || peakUsage > 100 {
		logger.Warn("环境变量 P 值超出范围，使用默认值", "value", peakUsage, "default", defaultPeakUsage)
		return defaultPeakUsage
	}

	// 如果设置值过低，使用最小值
	if peakUsage < minPeakUsage {
		logger.Warn("环境变量 P 值过低，使用最小值", "value", peakUsage, "min", minPeakUsage)
		return minPeakUsage
	}

	return peakUsage
}

// isNightTime 判断是否是凌晨时段（UTC 16:00-20:00）
func isNightTime() bool {
	now := time.Now().UTC()
	hour := now.Hour()
	return hour >= 16 && hour < 20
}

// calculateExpectedUsage 计算期望占用值
func calculateExpectedUsage(userPeakUsage int) float64 {
	var expectedUsage float64

	if isNightTime() {
		// 凌晨时段：期望占用 = min(用户设置值, 70%)
		expectedUsage = float64(userPeakUsage)
	} else {
		// 其他时段：期望占用 = min(用户设置值 * 0.8, 70%)
		expectedUsage = float64(userPeakUsage) * 0.8
	}

	// 硬峰值限制：任何时段都不能超过 70%
	if expectedUsage > hardPeakLimit {
		expectedUsage = hardPeakLimit
	}

	return expectedUsage
}

// adjustResources 调整资源占用
func adjustResources(stats *SystemStats, expectedUsage float64) {
	// 调整内存
	adjustMemory(stats, expectedUsage)

	// 调整 CPU
	adjustCPU(stats, expectedUsage)
}

// adjustMemory 调整内存占用
func adjustMemory(stats *SystemStats, expectedUsage float64) {
	currentPercent := stats.MemoryPercent

	// 硬峰值检查：如果超过70%，必须强制降低（安全机制）
	if currentPercent > hardPeakLimit {
		logger.Warn("内存占用超过硬峰值，强制降低", "current_percent", currentPercent, "hard_peak", hardPeakLimit)
		success, _, _ := memoryController.AdjustMemoryRandom(false) // 强制减少
		if success {
			// 格式化：内存-当前占用%-强制-减少
			logger.Info("内存-" + formatPercent(currentPercent) + "-强制-减少")
		}
		return
	}

	diff := currentPercent - expectedUsage // 正数表示当前 > 期望（需要减少），负数表示当前 < 期望（需要增加）

	// 计算调整概率（是否执行调整）
	adjustProb := calculateAdjustProbability(abs(diff))
	if !shouldAdjust(adjustProb) {
		// 格式化：内存-当前占用%-跳过
		logger.Info("内存-" + formatPercent(currentPercent) + "-跳过")
		return
	}

	// 根据差值计算上涨/下跌的概率
	// 差值越大，概率越极端；差值越小，概率越接近
	increaseProb := calculateDirectionProbability(diff, expectedUsage)

	// 随机决定是增加还是减少
	shouldIncrease := rand.Float64() < increaseProb

	// 执行调整
	success, increased, _ := memoryController.AdjustMemoryRandom(shouldIncrease)
	if success {
		action := "减少"
		if increased {
			action = "增加"
		}
		// 格式化：内存-当前占用%-增加概率-实际动作
		logger.Info("内存-" + formatPercent(currentPercent) + "-" + formatProbability(increaseProb) + "-" + action)
	}
}

// adjustCPU 调整 CPU 占用
func adjustCPU(stats *SystemStats, expectedUsage float64) {
	currentPercent := stats.CPUPercent

	// 硬峰值检查：如果超过70%，必须强制降低（安全机制）
	if currentPercent > hardPeakLimit {
		logger.Warn("CPU 占用超过硬峰值，强制降低", "current_percent", currentPercent, "hard_peak", hardPeakLimit)
		success, _, _ := cpuController.AdjustCountRandom(false) // 强制减少
		if success {
			// 格式化：CPU-当前占用%-强制-减少
			logger.Info("CPU-" + formatPercent(currentPercent) + "-强制-减少")
		}
		return
	}

	diff := currentPercent - expectedUsage // 正数表示当前 > 期望（需要减少），负数表示当前 < 期望（需要增加）

	// 计算调整概率（是否执行调整）
	adjustProb := calculateAdjustProbability(abs(diff))
	if !shouldAdjust(adjustProb) {
		// 格式化：CPU-当前占用%-跳过
		logger.Info("CPU-" + formatPercent(currentPercent) + "-跳过")
		return
	}

	// 根据差值计算上涨/下跌的概率
	// 差值越大，概率越极端；差值越小，概率越接近
	increaseProb := calculateDirectionProbability(diff, expectedUsage)

	// 随机决定是增加还是减少占用
	shouldIncrease := rand.Float64() < increaseProb

	// 执行调整
	success, increasedCount, _ := cpuController.AdjustCountRandom(shouldIncrease)
	if success {
		action := "减少"
		if increasedCount {
			action = "增加"
		}
		// 格式化：CPU-当前占用%-增加概率-实际动作
		logger.Info("CPU-" + formatPercent(currentPercent) + "-" + formatProbability(increaseProb) + "-" + action)
	}
}

// calculateAdjustProbability 计算是否执行调整的概率
func calculateAdjustProbability(diff float64) float64 {
	if diff > 5 {
		return 0.90 // 90%
	} else if diff >= 2 {
		return 0.70 // 70%
	} else {
		return 0.60 // 60%
	}
}

// calculateDirectionProbability 计算上涨（增加占用）的概率
// diff: 当前值 - 期望值（正数表示当前 > 期望，负数表示当前 < 期望）
// expectedUsage: 期望值
// 返回：上涨的概率（0.0 - 1.0）
func calculateDirectionProbability(diff, expectedUsage float64) float64 {
	absDiff := abs(diff)

	if diff < 0 {
		// 当前 < 期望，应该上涨（增加占用）
		// 差值越大，上涨概率越大
		if absDiff > 50 {
			return 0.90 // 差70%，上涨概率90%
		} else if absDiff > 20 {
			return 0.80 // 差20-50%，上涨概率80%
		} else if absDiff > 10 {
			return 0.70 // 差10-20%，上涨概率70%
		} else if absDiff > 5 {
			return 0.65 // 差5-10%，上涨概率65%
		} else if absDiff >= 2 {
			return 0.60 // 差2-5%，上涨概率60%
		} else {
			return 0.55 // 差<2%，上涨概率55%（接近期望值）
		}
	} else {
		// 当前 > 期望，应该下跌（减少占用）
		// 差值越大，下跌概率越大（上涨概率越小）
		if absDiff > 50 {
			return 0.10 // 差70%，上涨概率10%（下跌概率90%）
		} else if absDiff > 20 {
			return 0.20 // 差20-50%，上涨概率20%（下跌概率80%）
		} else if absDiff > 10 {
			return 0.30 // 差10-20%，上涨概率30%（下跌概率70%）
		} else if absDiff > 5 {
			return 0.35 // 差5-10%，上涨概率35%（下跌概率65%）
		} else if absDiff >= 2 {
			return 0.40 // 差2-5%，上涨概率40%（下跌概率60%）
		} else {
			return 0.45 // 差<2%，上涨概率45%（下跌概率55%，接近期望值）
		}
	}
}

// shouldAdjust 根据概率决定是否调整
func shouldAdjust(probability float64) bool {
	return rand.Float64() < probability
}

// abs 计算绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// formatPercent 格式化百分比，保留1位小数
func formatPercent(p float64) string {
	return fmt.Sprintf("%.1f%%", p)
}

// formatProbability 格式化概率，保留1位小数（0.0-1.0）
func formatProbability(p float64) string {
	return fmt.Sprintf("%.1f", p)
}

// updatePeakUsage 每 5 分钟更新一次 peakUsage
// 新值范围：rand[0.2 * peakUsage_origin, peakUsage_origin]
func updatePeakUsage() {
	peakUsageMu.Lock()
	defer peakUsageMu.Unlock()

	// 保存旧值用于日志
	oldPeakUsage := peakUsage

	// 计算范围：0.2 * peakUsage_origin 到 peakUsage_origin
	minValue := float64(peakUsageOrigin) * 0.2
	maxValue := float64(peakUsageOrigin)

	// 生成随机值
	newValue := minValue + rand.Float64()*(maxValue-minValue)

	// 转换为整数，并确保不小于最小值
	peakUsage = int(newValue)
	if peakUsage < int(minValue) {
		peakUsage = int(minValue)
	}
	if peakUsage < minPeakUsage {
		peakUsage = minPeakUsage
	}

	logger.Info("peakUsage 更新",
		"peak_usage_origin", peakUsageOrigin,
		"peak_usage_old", oldPeakUsage,
		"peak_usage_new", peakUsage,
		"range", fmt.Sprintf("[%.1f, %d]", minValue, peakUsageOrigin))
}
