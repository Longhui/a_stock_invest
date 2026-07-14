package reader

import (
	"archive/zip"
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// BlockInfo 板块信息
type BlockInfo struct {
	Code string // 板块代码 (880xxx/881xxx)
	Name string // 板块名称 (煤炭, 5G概念, ...)
}

// SectorMap 股票所属板块映射
// key: 6位股票代码, value: 行业板块名称列表
type SectorMap map[string][]string

// LoadBlockMap 从通达信BlockMapXML.dat加载板块列表
// BlockMapXML.dat 是一个ZIP包,包含多个XML文件(行业板块/概念板块/地区板块/风格板块)
// XML中 Label 节点的 id=板块代码, userstr=板块名称
func LoadBlockMap(tdxDir string) ([]BlockInfo, error) {
	zipPath := filepath.Join(tdxDir, "BlockMap", "BlockMapXML.dat")
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("BlockMapXML.dat not found at %s", zipPath)
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("打开BlockMapXML.dat失败: %w", err)
	}
	defer r.Close()

	var blocks []BlockInfo
	seen := make(map[string]bool) // 去重

	for _, f := range r.File {
		name := f.Name
		// 只处理板块XML文件
		if !strings.HasSuffix(name, ".xml") ||
			strings.HasPrefix(name, "blockmap") ||
			strings.HasPrefix(name, "Font") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			continue
		}

		blocksFromXML, err := parseBlockXML(rc)
		rc.Close()
		if err != nil {
			continue
		}

		for _, b := range blocksFromXML {
			if !seen[b.Code] {
				seen[b.Code] = true
				blocks = append(blocks, b)
			}
		}
	}

	if len(blocks) == 0 {
		return nil, fmt.Errorf("BlockMapXML.dat中未解析到板块数据")
	}
	return blocks, nil
}

// blockXMLNode XML解析用的中间结构
type blockXMLNode struct {
	XMLName xml.Name
	Labels  []blockLabel `xml:"Label"`
	Nodes   []blockXMLNode `xml:"Container"`
}

type blockLabel struct {
	ID      string `xml:"id,attr"`
	UserStr string `xml:"userstr,attr"`
}

func parseBlockXML(rc io.ReadCloser) ([]BlockInfo, error) {
	decoder := xml.NewDecoder(rc)
	var root blockXMLNode
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}

	var blocks []BlockInfo
	collectLabels(&root, &blocks)
	return blocks, nil
}

func collectLabels(node *blockXMLNode, blocks *[]BlockInfo) {
	for _, label := range node.Labels {
		code := strings.TrimSpace(label.ID)
		name := strings.TrimSpace(label.UserStr)
		if len(code) == 6 && name != "" {
			*blocks = append(*blocks, BlockInfo{Code: code, Name: name})
		}
	}
	for _, child := range node.Nodes {
		collectLabels(&child, blocks)
	}
}

// LoadStockSectors 从通达信板块缓存文件加载股票->行业映射
// 尝试读取 T0002/blocknew/ 下的系统缓存文件
// 如果读取失败,返回空映射(调用方应回退到启发式分类)
func LoadStockSectors(tdxDir string) SectorMap {
	result := make(SectorMap)

	blockDir := filepath.Join(tdxDir, "T0002", "blocknew")
	entries, err := os.ReadDir(blockDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".blk") {
			continue
		}
		// 跳过用户自选股
		if name == "zxg.blk" || name == "tjg.blk" {
			continue
		}
		sectorName := strings.TrimSuffix(name, ".blk")
		filePath := filepath.Join(blockDir, name)
		stocks, err := readBlockFile(filePath)
		if err != nil {
			continue
		}
		for _, stock := range stocks {
			result[stock] = append(result[stock], sectorName)
		}
	}

	return result
}

// BuildSectorMapFromBlocks 从板块列表和成分股查询接口构建股票→板块映射
// blockStocksFn 接受板块代码返回该板块的所有成分股代码
// 只处理行业板块(881xxx)和概念板块(880xxx)
func BuildSectorMapFromBlocks(blocks []BlockInfo, blockStocksFn func(code string) ([]string, error)) SectorMap {
	result := make(SectorMap)

	for _, b := range blocks {
		// 只处理行业板块和概念板块
		isIndustry := strings.HasPrefix(b.Code, "881")
		isConcept := strings.HasPrefix(b.Code, "880")
		if !isIndustry && !isConcept {
			continue
		}

		stocks, err := blockStocksFn(b.Code)
		if err != nil || len(stocks) == 0 {
			continue
		}

		for _, stock := range stocks {
			stock = strings.TrimSpace(stock)
			if len(stock) == 6 {
				result[stock] = append(result[stock], b.Name)
			}
		}
	}

	return result
}

// readBlockFile 读取通达信板块文件(文本格式,每行一个股票代码)
func readBlockFile(filePath string) ([]string, error) {
	fd, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	var stocks []string
	scanner := bufio.NewScanner(fd)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) >= 6 {
			// 兼容带交易所前缀和纯6位代码格式
			code := line
			if len(code) > 6 {
				code = code[len(code)-6:]
			}
			stocks = append(stocks, code)
		}
	}
	return stocks, scanner.Err()
}

// SectorName 从板块映射查询股票所属行业
// 如果有映射数据则返回第一个匹配的行业,否则返回""
func (m SectorMap) SectorName(code string) string {
	if m == nil {
		return ""
	}
	sectors, ok := m[code]
	if !ok || len(sectors) == 0 {
		return ""
	}
	return sectors[0]
}
