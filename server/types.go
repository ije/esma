package server

import (
	"time"

	"github.com/evanw/esbuild/pkg/api"
)

// The config for ESMD Server
type Config struct {
	Mode    string        `json:"mode"`
	LogDir  string        `json:"logDir"`
	Autotls AutotlsConfig `json:"autotls"`
}

// The config for AutoTLS
type AutotlsConfig struct {
	Hosts    []string `json:"hosts"`
	CacheDir string   `json:"cacheDir"`
}

// FileContent cache file content in memory
type FileContent struct {
	Error   error
	Modtime time.Time
	Content []byte
}

type ESBulidRecord struct {
	FileContent
	api.BuildResult
}
