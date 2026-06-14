package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

const defaultVideoPipelineConfigPath = "config/video-pipeline.yaml"

var (
	videoPipelineConfigValue atomic.Value
	videoPipelineWatcherOnce sync.Once
)

type VideoPipelineRuntimeConfig struct {
	Version    int                           `yaml:"version"`
	Defaults   VideoPipelineDefaultsConfig   `yaml:"defaults"`
	Strategies []VideoPipelineStrategyConfig `yaml:"strategies"`
}

type VideoPipelineDefaultsConfig struct {
	Enabled bool `yaml:"enabled"`
}

type VideoPipelineStrategyConfig struct {
	Name      string                       `yaml:"name"`
	Enabled   bool                         `yaml:"enabled"`
	Lifecycle VideoPipelineLifecycleConfig `yaml:"lifecycle"`
}

type VideoPipelineLifecycleConfig struct {
	Match   VideoPipelineMatchConfig   `yaml:"match"`
	Rollout VideoPipelineRolloutConfig `yaml:"rollout"`
}

type VideoPipelineMatchConfig struct {
	RelayMode            string   `yaml:"relay_mode"`
	Models               []string `yaml:"models"`
	RequestedResolutions []string `yaml:"requested_resolutions"`
}

type VideoPipelineRolloutConfig struct {
	TrafficPercent          int                                `yaml:"traffic_percent"`
	RequestOverrideMetadata VideoPipelineRequestOverrideConfig `yaml:"request_override_metadata"`
}

type VideoPipelineRequestOverrideConfig struct {
	ForceKeys  []string `yaml:"force_keys"`
	BypassKeys []string `yaml:"bypass_keys"`
}

func init() {
	videoPipelineConfigValue.Store(defaultVideoPipelineRuntimeConfig())
}

func InitVideoPipelineConfig() {
	path := videoPipelineConfigPath()
	if err := loadVideoPipelineConfigFile(path, false); err != nil {
		commonLogVideoPipelineConfig("load failed", path, err)
	}
	startVideoPipelineConfigWatcher(path)
}

func LoadVideoPipelineConfigFromFile(path string) error {
	return loadVideoPipelineConfigFile(path, true)
}

func GetVideoPipelineConfig() *VideoPipelineRuntimeConfig {
	cfg, ok := videoPipelineConfigValue.Load().(*VideoPipelineRuntimeConfig)
	if !ok || cfg == nil {
		return defaultVideoPipelineRuntimeConfig()
	}
	return cfg
}

func resetVideoPipelineConfigForTest() {
	videoPipelineConfigValue.Store(defaultVideoPipelineRuntimeConfig())
}

func videoPipelineConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("VIDEO_PIPELINE_CONFIG_PATH")); path != "" {
		return path
	}
	return defaultVideoPipelineConfigPath
}

func loadVideoPipelineConfigFile(path string, requireExists bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultVideoPipelineConfigPath
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !requireExists {
			videoPipelineConfigValue.Store(defaultVideoPipelineRuntimeConfig())
			return nil
		}
		return err
	}
	cfg, err := parseVideoPipelineConfig(raw)
	if err != nil {
		return err
	}
	videoPipelineConfigValue.Store(cfg)
	commonLogVideoPipelineConfig("loaded", path, nil)
	return nil
}

func parseVideoPipelineConfig(raw []byte) (*VideoPipelineRuntimeConfig, error) {
	var cfg VideoPipelineRuntimeConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if cfg.Version != 1 {
		return nil, fmt.Errorf("unsupported video pipeline config version %d", cfg.Version)
	}
	for i := range cfg.Strategies {
		if err := validateVideoPipelineStrategyConfig(cfg.Strategies[i]); err != nil {
			return nil, fmt.Errorf("strategies[%d]: %w", i, err)
		}
	}
	return &cfg, nil
}

func validateVideoPipelineStrategyConfig(s VideoPipelineStrategyConfig) error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if s.Lifecycle.Rollout.TrafficPercent < 0 || s.Lifecycle.Rollout.TrafficPercent > 100 {
		return fmt.Errorf("rollout.traffic_percent must be between 0 and 100")
	}
	if s.Lifecycle.Match.RelayMode != "" {
		if _, ok := videoPipelineRelayModeFromName(s.Lifecycle.Match.RelayMode); !ok {
			return fmt.Errorf("unsupported match.relay_mode %q", s.Lifecycle.Match.RelayMode)
		}
	}
	return nil
}

func defaultVideoPipelineRuntimeConfig() *VideoPipelineRuntimeConfig {
	return &VideoPipelineRuntimeConfig{
		Version:  1,
		Defaults: VideoPipelineDefaultsConfig{Enabled: false},
	}
}

func startVideoPipelineConfigWatcher(path string) {
	videoPipelineWatcherOnce.Do(func() {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			commonLogVideoPipelineConfig("watcher init failed", path, err)
			return
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			absPath = path
		}
		dir := filepath.Dir(absPath)
		if err := watcher.Add(dir); err != nil {
			commonLogVideoPipelineConfig("watch failed", dir, err)
			_ = watcher.Close()
			return
		}
		go watchVideoPipelineConfigFile(watcher, absPath)
		commonLogVideoPipelineConfig("watching", absPath, nil)
	})
}

func watchVideoPipelineConfigFile(watcher *fsnotify.Watcher, absPath string) {
	var (
		mu    sync.Mutex
		timer *time.Timer
	)
	reload := func() {
		if err := loadVideoPipelineConfigFile(absPath, false); err != nil {
			commonLogVideoPipelineConfig("reload failed", absPath, err)
		}
	}
	scheduleReload := func() {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(250*time.Millisecond, reload)
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			eventPath, err := filepath.Abs(event.Name)
			if err != nil {
				eventPath = event.Name
			}
			if eventPath != absPath {
				continue
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename|fsnotify.Remove) != 0 {
				scheduleReload()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			commonLogVideoPipelineConfig("watch error", absPath, err)
		}
	}
}

func videoPipelineRelayModeFromName(name string) (int, bool) {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "", "any":
		return relayconstant.RelayModeUnknown, true
	case "video_submit":
		return relayconstant.RelayModeVideoSubmit, true
	default:
		return relayconstant.RelayModeUnknown, false
	}
}

func commonLogVideoPipelineConfig(action, path string, err error) {
	if err != nil {
		common.SysLog(fmt.Sprintf("video pipeline config %s path=%s err=%v", action, path, err))
		return
	}
	common.SysLog(fmt.Sprintf("video pipeline config %s path=%s", action, path))
}
