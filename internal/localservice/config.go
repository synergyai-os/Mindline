package localservice

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	ConfigSchemaVersion = "mindline-local-agent-config/v0.1"
	APISchemaVersion    = "mindline-local-agent-api/v0.1"
)

type Config struct {
	SchemaVersion  string `json:"schema_version"`
	RuntimeRoot    string `json:"runtime_root"`
	MemoryRoot     string `json:"memory_root"`
	SocketPath     string `json:"socket_path"`
	StatePath      string `json:"state_path"`
	OllamaURL      string `json:"ollama_url"`
	EmbeddingModel string `json:"embedding_model"`
}

func DefaultConfig() (Config, error) {
	controlRoot, err := privateio.DefaultControlPlaneRoot()
	if err != nil {
		return Config{}, err
	}
	return ConfigFromRoots(
		filepath.Join(controlRoot, "agent-runtime"),
		filepath.Join(controlRoot, "personal-memory"),
	)
}

func DefaultConfigPath() (string, error) {
	config, err := DefaultConfig()
	if err != nil {
		return "", err
	}
	return filepath.Join(config.RuntimeRoot, "config.json"), nil
}

func LoadConfig(configPath string) (Config, error) {
	if strings.TrimSpace(configPath) == "" {
		var err error
		configPath, err = DefaultConfigPath()
		if err != nil {
			return Config{}, err
		}
	}
	if !filepath.IsAbs(configPath) {
		return Config{}, errors.New("local agent config path must be absolute")
	}
	var config Config
	if err := privateio.ReadJSONStrictBounded(filepath.Dir(configPath), configPath, 64<<10, &config); err != nil {
		return Config{}, errors.New("read local agent config")
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	if filepath.Dir(filepath.Clean(configPath)) != config.RuntimeRoot {
		return Config{}, errors.New("local agent config is outside its runtime root")
	}
	if err := privateio.ValidateContained(config.RuntimeRoot, config.StatePath); err != nil {
		return Config{}, errors.New("invalid local agent runtime paths")
	}
	return config, nil
}

func SaveConfig(configPath string, config Config) error {
	if !filepath.IsAbs(configPath) || filepath.Dir(configPath) != config.RuntimeRoot {
		return errors.New("local agent config must be inside its runtime root")
	}
	if err := config.Prepare(); err != nil {
		return err
	}
	if err := privateio.WriteJSON(configPath, config); err != nil {
		return errors.New("write local agent config")
	}
	return nil
}

func ConfigFromRoots(runtimeRoot, memoryRoot string) (Config, error) {
	if !filepath.IsAbs(runtimeRoot) || !filepath.IsAbs(memoryRoot) {
		return Config{}, errors.New("local agent roots must be absolute")
	}
	runtimeRoot = filepath.Clean(runtimeRoot)
	memoryRoot = filepath.Clean(memoryRoot)
	return Config{
		SchemaVersion:  ConfigSchemaVersion,
		RuntimeRoot:    runtimeRoot,
		MemoryRoot:     memoryRoot,
		SocketPath:     filepath.Join(runtimeRoot, "mindline.sock"),
		StatePath:      filepath.Join(runtimeRoot, "agent-state.sqlite"),
		OllamaURL:      "http://127.0.0.1:11434",
		EmbeddingModel: "embeddinggemma:latest",
	}, nil
}

func (config Config) Validate() error {
	if config.SchemaVersion != ConfigSchemaVersion ||
		!filepath.IsAbs(config.RuntimeRoot) || !filepath.IsAbs(config.MemoryRoot) ||
		!filepath.IsAbs(config.SocketPath) || !filepath.IsAbs(config.StatePath) ||
		len([]byte(config.SocketPath)) > 100 ||
		strings.TrimSpace(config.OllamaURL) == "" ||
		strings.TrimSpace(config.EmbeddingModel) == "" {
		return errors.New("invalid local agent config")
	}
	if filepath.Clean(config.RuntimeRoot) != config.RuntimeRoot ||
		filepath.Clean(config.MemoryRoot) != config.MemoryRoot ||
		filepath.Clean(config.SocketPath) != config.SocketPath ||
		filepath.Clean(config.StatePath) != config.StatePath ||
		filepath.Dir(config.SocketPath) != config.RuntimeRoot ||
		filepath.Dir(config.StatePath) != config.RuntimeRoot {
		return errors.New("invalid local agent config paths")
	}
	return nil
}

func (config Config) Prepare() error {
	if err := config.Validate(); err != nil {
		return err
	}
	if err := privateio.PrepareDir(config.RuntimeRoot); err != nil {
		return errors.New("prepare local agent runtime")
	}
	if err := privateio.PrepareDir(config.MemoryRoot); err != nil {
		return errors.New("prepare personal memory root")
	}
	if err := privateio.ValidateContained(config.RuntimeRoot, config.StatePath); err != nil {
		return errors.New("invalid local agent runtime paths")
	}
	return nil
}
