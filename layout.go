package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

type ReplacementFile struct {
	Path string
	Type string
}

type Layout struct {
	TemplateFile     string
	OverlayFile      string
	EnvFile          string
	ReplacementFiles []ReplacementFile
}

func DirectoryLayout(configDir string) Layout {
	configDir = filepath.Clean(configDir)
	baseDir := filepath.Join(filepath.Dir(configDir), "base")
	return Layout{
		TemplateFile: filepath.Join(baseDir, "template.yaml"),
		OverlayFile:  filepath.Join(configDir, "overlay.yaml"),
		EnvFile:      filepath.Join(configDir, ".env"),
		ReplacementFiles: []ReplacementFile{
			{Path: filepath.Join(configDir, "replace.yaml"), Type: "yaml"},
			{Path: filepath.Join(baseDir, "global.yaml"), Type: "yaml"},
		},
	}
}

func (layout Layout) validate() error {
	if strings.TrimSpace(layout.TemplateFile) == "" {
		return fmt.Errorf("config: template file is required")
	}
	for i, file := range layout.ReplacementFiles {
		if strings.TrimSpace(file.Path) == "" {
			return fmt.Errorf("config: replacement file %d has no path", i)
		}
		if strings.TrimSpace(file.Type) == "" {
			return fmt.Errorf("config: replacement file %q has no type", file.Path)
		}
	}
	return nil
}

func cloneLayout(layout Layout) Layout {
	result := layout
	result.ReplacementFiles = append([]ReplacementFile(nil), layout.ReplacementFiles...)
	return result
}
