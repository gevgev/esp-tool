package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Device represents a discovered ESPHome device.
type Device struct {
	Name string // from esphome.name in YAML
	File string // relative path to YAML file (e.g. "air-quality-internal.yaml")
	Host string // mDNS hostname (e.g. "air-quality-internal.local")
}

// esphomeYAML is a minimal struct to extract the device name and substitutions.
type esphomeYAML struct {
	Substitutions map[string]string `yaml:"substitutions"`
	ESPHome       struct {
		Name string `yaml:"name"`
	} `yaml:"esphome"`
}

// Scan globs all *.yaml files in dir, skipping secrets.yaml and any file
// not at the top level (e.g. inside archive/ or helper files/).
// Returns devices sorted in the order found by filepath.Glob.
func Scan(dir string) ([]Device, error) {
	pattern := filepath.Join(dir, "*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", pattern, err)
	}

	var devices []Device
	for _, path := range matches {
		base := filepath.Base(path)
		// Skip secrets file and any non-device YAMLs at root
		if base == "secrets.yaml" {
			continue
		}

		dev, err := parseDevice(path)
		if err != nil {
			// Skip files that don't parse as ESPHome device configs
			continue
		}
		devices = append(devices, dev)
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no ESPHome device YAML files found in %s", dir)
	}

	return devices, nil
}

func parseDevice(path string) (Device, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Device{}, err
	}

	var cfg esphomeYAML
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Device{}, err
	}

	name := strings.TrimSpace(cfg.ESPHome.Name)
	if name == "" {
		return Device{}, fmt.Errorf("no esphome.name in %s", path)
	}

	// Resolve ESPHome substitution variables: ${varname} or $varname
	name = resolveSubstitutions(name, cfg.Substitutions)

	// If substitution couldn't be resolved, the name still contains $ — skip
	if strings.Contains(name, "$") {
		return Device{}, fmt.Errorf("unresolved substitution in esphome.name %q in %s", name, path)
	}

	return Device{
		Name: name,
		File: filepath.Base(path),
		Host: name + ".local",
	}, nil
}

// resolveSubstitutions replaces ${key} and $key patterns using the given map.
func resolveSubstitutions(s string, subs map[string]string) string {
	if len(subs) == 0 {
		return s
	}
	// Replace ${key} first, then $key
	for k, v := range subs {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
		s = strings.ReplaceAll(s, "$"+k, v)
	}
	return s
}
