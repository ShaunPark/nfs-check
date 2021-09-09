package utils

import (
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/ShaunPark/nfsCheck/types"

	"gopkg.in/yaml.v2"
)

type ConfigManager struct {
	config       *types.Config
	lastReadTime time.Time
	configFile   string
}

func NewConfigManager(configFile string) *ConfigManager {
	config := readConfig(configFile)
	return &ConfigManager{config: config, lastReadTime: time.Now(), configFile: configFile}
}

func (c *ConfigManager) GetConfig() *types.Config {
	now := time.Now()

	if now.After(c.lastReadTime.Add(time.Minute * 1)) {
		c.config = readConfig(c.configFile)
	}

	return c.config
}

func readConfig(configFile string) *types.Config {
	fileName, _ := filepath.Abs(configFile)
	yamlFile, err := ioutil.ReadFile(fileName)
	if err != nil {
		panic(err)
	}

	var config types.Config

	err = yaml.Unmarshal(yamlFile, &config)

	if err != nil {
		panic(err)
	}
	return &config
}
