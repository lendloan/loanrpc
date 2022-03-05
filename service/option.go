package service

import (
	"errors"
	"io/ioutil"
	"os"

	goyaml "gopkg.in/yaml.v2"

	s2s "github.com/lendloan/loanrpc/registry"
	"github.com/micro/go-micro/v2/config"
	"github.com/micro/go-micro/v2/config/encoder/yaml"
	"github.com/micro/go-micro/v2/config/source"
	"github.com/micro/go-micro/v2/config/source/file"
)

// 检查文件是否存在
//
// @param path
// @return {bool}
//
func fileExist(path string) bool {
	_, err := os.Lstat(path)

	return !os.IsNotExist(err)
}

// 加载配置，配置格式必须是yaml
// 如果conf不等于nil,则将其解码到conf结构中
//
// @param   conffile    配置文件
// @param   conf        解析引入结构
//
func LoadConf(conffile string, conf interface{}) (err error) {
	if !fileExist(conffile) {
		err = errors.New(conffile + " not exists.")

		return
	}

	enc := yaml.NewEncoder()
	fileSource := file.NewSource(file.WithPath(conffile), source.WithEncoder(enc))
	err = config.Load(fileSource)
	if nil != err {
		return
	}

	if nil == conf {
		return
	}

	yamldata, err := ioutil.ReadFile(conffile)
	if nil != err {
		return
	}
	err = goyaml.Unmarshal(yamldata, conf)
	if nil != err {
		return
	}

	// 获取服务中使用的nodes列表
	// 主要用于定时获取s2s信息
	mp := config.Map()
	watchnodes := make([]string, 0)
	if _, ok := mp["nodes"]; ok {
		switch mp["nodes"].(type) {
		case map[string]interface{}:
			nodes := mp["nodes"].(map[string]interface{})
			for _, v := range nodes {
				watchnodes = append(watchnodes, v.(string))
			}

			break

		default:
			break
		}
	}

	s2s.SetWatchNode(watchnodes)
	return
}
