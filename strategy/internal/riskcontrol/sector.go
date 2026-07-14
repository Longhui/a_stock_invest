package riskcontrol

// ClassifySector 根据代码前缀判断板块
func ClassifySector(code string) string {
	bare := code
	if len(code) >= 8 {
		bare = code[2:]
	}
	if len(bare) < 6 {
		return "未知"
	}
	switch {
	case bare[:1] == "6":
		return "主板蓝筹"
	case bare[:2] == "00":
		return "主板传统"
	case bare[:2] == "30":
		return "创业板"
	case bare[:3] == "688":
		return "科创板"
	case bare[:1] == "8" || bare[:2] == "92" || bare[:2] == "43":
		return "北交所"
	default:
		return "主板综合"
	}
}

// ClassifyTopic 根据代码前缀判断题材
func ClassifyTopic(code string) string {
	bare := code
	if len(code) >= 8 {
		bare = code[2:]
	}
	if len(bare) < 6 {
		return "轮动题材"
	}
	switch {
	case bare[:2] == "30":
		return "科技成长"
	case bare[:3] == "688":
		return "硬科技"
	case bare[:1] == "6":
		return "传统制造"
	case bare[:2] == "00":
		return "综合蓝筹"
	default:
		return "轮动题材"
	}
}

// SectorLevel 返回题材等级(0=主线,1=次主线,2=轮动,3=冷门)
// v1简化版: 创业板和科创板默认次主线,其余轮动
func SectorLevel(sector, topic string) int {
	switch {
	case sector == "创业板" || sector == "科创板":
		return 1 // 次主线
	case sector == "主板蓝筹":
		return 2 // 轮动
	default:
		return 2 // 轮动
	}
}
