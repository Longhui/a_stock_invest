package paper

import (
	"encoding/json"
	"os"
	"sync"
)

var saveMu sync.Mutex

// loadState 从文件加载模拟盘状态
// 文件不存在则返回初始状态
func loadState(cfg PaperConfig) (*PaperState, error) {
	data, err := os.ReadFile(cfg.StateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &PaperState{
				Config:      cfg,
				Cash:        cfg.InitCapital,
				PeakCapital: cfg.InitCapital,
			}, nil
		}
		return nil, err
	}

	var state PaperState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	state.Config = cfg // 始终使用最新配置
	return &state, nil
}

// saveState 写入状态到 JSON 文件
func saveState(state *PaperState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(state.Config.StateFile, data, 0644)
}

// saveStateSafe 线程安全版本
func saveStateSafe(state *PaperState) error {
	saveMu.Lock()
	defer saveMu.Unlock()
	return saveState(state)
}
