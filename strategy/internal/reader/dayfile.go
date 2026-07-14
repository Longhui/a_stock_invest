package reader

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"
)

// DayRecord 通达信日线数据单条记录 (32 bytes)
type DayRecord struct {
	Date   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Amount float64 // 成交额(元)
	Volume int64   // 成交量(股)
}

type dayRecordRaw struct {
	Date   int32   // YYYYMMDD
	Open   int32   // 价格*100(2位小数)
	High   int32
	Low    int32
	Close  int32
	Amount float32
	Volume int32
	_      int32 // 保留
}

// ReadDayFile 读取通达信本地 .day 文件
// filePath: vipdoc/sh/lday/sh600036.day
func ReadDayFile(filePath string) ([]DayRecord, error) {
	fd, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer fd.Close()

	fi, err := fd.Stat()
	if err != nil {
		return nil, fmt.Errorf("获取文件信息失败: %w", err)
	}

	size := fi.Size()
	if size%32 != 0 {
		return nil, fmt.Errorf("文件大小%d不是32的倍数", size)
	}

	count := int(size / 32)
	records := make([]DayRecord, 0, count)

	for i := 0; i < count; i++ {
		var raw dayRecordRaw
		if err := binary.Read(fd, binary.LittleEndian, &raw); err != nil {
			return nil, fmt.Errorf("读取第%d条记录失败: %w", i, err)
		}

		dateStr := fmt.Sprintf("%d", raw.Date)
		t, err := time.Parse("20060102", dateStr)
		if err != nil {
			continue
		}

		records = append(records, DayRecord{
			Date:   t,
			Open:   float64(raw.Open) / 100,
			High:   float64(raw.High) / 100,
			Low:    float64(raw.Low) / 100,
			Close:  float64(raw.Close) / 100,
			Amount: float64(raw.Amount),
			Volume: int64(raw.Volume),
		})
	}

	return records, nil
}

// GetDayFilePath 根据股票代码返回 .day 文件路径
func GetDayFilePath(tdxDir, code string) (string, error) {
	if len(code) < 6 {
		return "", fmt.Errorf("股票代码长度不足: %s", code)
	}

	var market string
	switch {
	case code[:1] == "6":
		market = "sh"
	case code[:2] == "30" || code[:1] == "0":
		market = "sz"
	case code[:1] == "8" || code[:2] == "92" || code[:2] == "43":
		market = "bj"
	default:
		return "", fmt.Errorf("未知市场: %s", code)
	}

	return filepath.Join(tdxDir, "vipdoc", market, "lday", market+code+".day"), nil
}

// DayFileExists 检查本地 .day 文件是否存在
func DayFileExists(tdxDir, code string) (string, bool, error) {
	path, err := GetDayFilePath(tdxDir, code)
	if err != nil {
		return "", false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, false, nil
		}
		return path, false, err
	}
	return path, info.Size() >= 32, nil // 至少有一条记录
}

// SaveDayFile 将K线数据保存为.day文件(32字节/条)
func SaveDayFile(filePath string, records []DayRecord) error {
	fd, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer fd.Close()

	for _, r := range records {
		raw := dayRecordRaw{
			Date:   int32(r.Date.Year()*10000 + int(r.Date.Month())*100 + r.Date.Day()),
			Open:   int32(math.Round(r.Open * 100)),
			High:   int32(math.Round(r.High * 100)),
			Low:    int32(math.Round(r.Low * 100)),
			Close:  int32(math.Round(r.Close * 100)),
			Amount: float32(r.Amount),
			Volume: int32(r.Volume),
		}
		if err := binary.Write(fd, binary.LittleEndian, &raw); err != nil {
			return fmt.Errorf("写入记录失败: %w", err)
		}
	}
	return nil
}

// GetDaysBetween 计算两个日期之间的自然日
func GetDaysBetween(start, end time.Time) int {
	return int(end.Sub(start) / (24 * time.Hour))
}

// CalcMA 计算移动平均线
func CalcMA(data []float64, period int) []float64 {
	if len(data) < period {
		return nil
	}
	result := make([]float64, len(data))
	sum := 0.0
	for i := 0; i < len(data); i++ {
		sum += data[i]
		if i >= period-1 {
			if i >= period {
				sum -= data[i-period]
			}
			result[i] = sum / float64(period)
		}
	}
	return result
}

// CalcEMA 计算指数移动平均
func CalcEMA(data []float64, period int) []float64 {
	if len(data) == 0 {
		return nil
	}
	result := make([]float64, len(data))
	alpha := 2.0 / float64(period+1)
	result[0] = data[0]
	for i := 1; i < len(data); i++ {
		result[i] = alpha*data[i] + (1-alpha)*result[i-1]
	}
	return result
}

// CalcSMA 计算SMA (用于KDJ)
func CalcSMA(data []float64, period, weight int) []float64 {
	if len(data) == 0 {
		return nil
	}
	result := make([]float64, len(data))
	result[0] = data[0]
	for i := 1; i < len(data); i++ {
		result[i] = (float64(weight)*data[i] + float64(period-weight)*result[i-1]) / float64(period)
	}
	return result
}

// CalcStdDev 计算标准差
func CalcStdDev(data []float64, period int) []float64 {
	if len(data) < period {
		return nil
	}
	result := make([]float64, len(data))
	for i := period - 1; i < len(data); i++ {
		mean := 0.0
		for j := i - period + 1; j <= i; j++ {
			mean += data[j]
		}
		mean /= float64(period)
		variance := 0.0
		for j := i - period + 1; j <= i; j++ {
			v := data[j] - mean
			variance += v * v
		}
		variance /= float64(period)
		result[i] = math.Sqrt(variance)
	}
	return result
}

// CalcHHV 计算周期内最高价
func CalcHHV(data []float64, period int) []float64 {
	if len(data) < period {
		return nil
	}
	result := make([]float64, len(data))
	for i := period - 1; i < len(data); i++ {
		maxVal := data[i-period+1]
		for j := i - period + 2; j <= i; j++ {
			if data[j] > maxVal {
				maxVal = data[j]
			}
		}
		result[i] = maxVal
	}
	return result
}

// CalcLLV 计算周期内最低价
func CalcLLV(data []float64, period int) []float64 {
	if len(data) < period {
		return nil
	}
	result := make([]float64, len(data))
	for i := period - 1; i < len(data); i++ {
		minVal := data[i-period+1]
		for j := i - period + 2; j <= i; j++ {
			if data[j] < minVal {
				minVal = data[j]
			}
		}
		result[i] = minVal
	}
	return result
}
