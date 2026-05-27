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
	Name   string // from esphome.name in YAML
	File   string // relative path to YAML file (e.g. "air-quality-internal.yaml")
	Host   string // mDNS hostname (e.g. "air-quality-internal.local")
	APIKey string // base64 Noise PSK from api.encryption.key; empty if !secret or absent
}

// esphomeYAML is a minimal struct to extract the device name and substitutions.
type esphomeYAML struct {
	Substitutions map[string]string `yaml:"substitutions"`
	ESPHome       struct {
		Name string `yaml:"name"`
	} `yaml:"esphome"`
}

// Scan reads all *.yaml files in dir (case-insensitive extension match),
// skipping secrets.yaml and any file that does not parse as a device config.
// Returns devices in lexicographic order (same as os.ReadDir).
//
// filepath.Glob("*.yaml") is case-sensitive even on Windows, so files saved
// with a .YAML extension (e.g. by Notepad) would be silently skipped.
// Using os.ReadDir + strings.ToLower handles .yaml, .YAML, .Yaml, etc.
// Scan reads all *.yaml files in dir (case-insensitive extension match),
// skipping secrets.yaml and any file that does not parse as a device config.
// Returns devices in lexicographic order (same as os.ReadDir).
//
// When no devices are found the error message lists every skipped file and
// the reason it was rejected, so misconfigured YAMLs are easy to diagnose.
func Scan(dir string) ([]Device, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	type skipped struct {
		name string
		err  error
	}
	var devices []Device
	var skips []skipped

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Case-insensitive extension check: accept .yaml, .YAML, .Yaml, …
		// filepath.Glob("*.yaml") is case-sensitive even on Windows, so files
		// saved with a .YAML extension (e.g. by Notepad) would be silently
		// skipped without this check.
		if strings.ToLower(filepath.Ext(name)) != ".yaml" {
			continue
		}
		// Skip secrets file (case-insensitive: SECRETS.YAML counts too).
		if strings.EqualFold(name, "secrets.yaml") {
			continue
		}

		dev, err := parseDevice(filepath.Join(dir, name))
		if err != nil {
			skips = append(skips, skipped{name, err})
			continue
		}
		devices = append(devices, dev)
	}

	if len(devices) == 0 {
		msg := fmt.Sprintf("no ESPHome device YAML files found in %s", dir)
		if len(skips) > 0 {
			msg += "\n  YAML files were found but could not be parsed as device configs:"
			for _, s := range skips {
				msg += fmt.Sprintf("\n    %-40s %v", s.name+":", s.err)
			}
			msg += "\n  Each device YAML must contain an 'esphome: name:' field."
		}
		return nil, fmt.Errorf("%s", msg)
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
		Name:   name,
		File:   filepath.Base(path),
		Host:   name + ".local",
		APIKey: parseAPIKey(data),
	}, nil
}

// parseAPIKey traverses the raw YAML node tree to extract api.encryption.key.
// It returns an empty string if the key is absent or uses the !secret tag
// (which would require reading secrets.yaml — not currently supported).
func parseAPIKey(data []byte) string {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return ""
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "api" {
			continue
		}
		apiNode := root.Content[i+1]
		if apiNode.Kind != yaml.MappingNode {
			return ""
		}
		for j := 0; j+1 < len(apiNode.Content); j += 2 {
			if apiNode.Content[j].Value != "encryption" {
				continue
			}
			encNode := apiNode.Content[j+1]
			if encNode.Kind != yaml.MappingNode {
				return ""
			}
			for k := 0; k+1 < len(encNode.Content); k += 2 {
				if encNode.Content[k].Value != "key" {
					continue
				}
				keyNode := encNode.Content[k+1]
				// Skip !secret references — would need secrets.yaml
				if keyNode.Tag != "!!str" && keyNode.Tag != "" {
					return ""
				}
				return strings.TrimSpace(keyNode.Value)
			}
		}
	}
	return ""
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
