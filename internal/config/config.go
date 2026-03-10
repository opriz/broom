package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultHTTPPort  = 7890
	DefaultSOCKSPort = 7891
	ConfigDir        = ".config/broom"
	ConfigFileName   = "broom.yaml"
	ProxiesFileName  = "proxies.txt" // 每行一个代理 URI（ss://、vmess:// 等）
)

type BroomConfig struct {
	SubscriptionURL string `yaml:"subscription_url"`
	HTTPPort        int    `yaml:"http_port,omitempty"`
	SOCKSPort       int    `yaml:"socks_port,omitempty"`
}

func (c *BroomConfig) EnsurePorts() {
	if c.HTTPPort == 0 {
		c.HTTPPort = DefaultHTTPPort
	}
	if c.SOCKSPort == 0 {
		c.SOCKSPort = DefaultSOCKSPort
	}
}

func ConfigDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ConfigDir), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigFileName), nil
}

func ProxiesFilePath() (string, error) {
	dir, err := ConfigDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ProxiesFileName), nil
}

// SaveProxies 将代理 URI 列表写入文件（每行一个）
func SaveProxies(urls []string) error {
	dir, err := ConfigDirPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path, _ := ProxiesFilePath()
	data := []byte(strings.Join(urls, "\n") + "\n")
	return os.WriteFile(path, data, 0600)
}

// LoadProxies 从文件读取代理 URI 列表
func LoadProxies() ([]string, error) {
	path, err := ProxiesFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

func Load() (*BroomConfig, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &BroomConfig{}, nil
		}
		return nil, err
	}
	var cfg BroomConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	cfg.EnsurePorts()
	return &cfg, nil
}

func Save(cfg *BroomConfig) error {
	dir, err := ConfigDirPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path, _ := ConfigPath()
	cfg.EnsurePorts()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
