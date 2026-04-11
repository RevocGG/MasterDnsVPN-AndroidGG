// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type configSourceFormat uint8

const (
	configSourceTOML configSourceFormat = iota
	configSourceJSON
)

func decodeConfigJSONInto(dst any, raw []byte) (map[string]bool, error) {
	if dst == nil {
		return nil, fmt.Errorf("config target is nil")
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}

	value := reflect.ValueOf(dst)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return nil, fmt.Errorf("config target must be a non-nil pointer")
	}
	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return nil, fmt.Errorf("config target must point to a struct")
	}

	valueType := value.Type()
	defined := make(map[string]bool, len(doc))

	for i := 0; i < valueType.NumField(); i++ {
		field := valueType.Field(i)
		tag := field.Tag.Get("toml")
		if tag == "" || tag == "-" {
			continue
		}

		rawValue, ok := doc[tag]
		if !ok {
			continue
		}
		target := value.Field(i)
		if !target.CanSet() {
			continue
		}

		if err := decodeJSONFieldInto(target, rawValue); err != nil {
			return nil, fmt.Errorf("invalid JSON config value for %s: %w", tag, err)
		}
		defined[field.Name] = true
	}

	return defined, nil
}

func decodeJSONFieldInto(target reflect.Value, raw json.RawMessage) error {
	switch target.Kind() {
	case reflect.String:
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		target.SetString(value)
		return nil
	case reflect.Bool:
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		target.SetBool(value)
		return nil
	case reflect.Int:
		var value int
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		target.SetInt(int64(value))
		return nil
	case reflect.Float64:
		var value float64
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		target.SetFloat(value)
		return nil
	case reflect.Slice:
		switch target.Type().Elem().Kind() {
		case reflect.String:
			var value []string
			if err := json.Unmarshal(raw, &value); err != nil {
				return err
			}
			target.Set(reflect.ValueOf(value))
			return nil
		case reflect.Int:
			var value []int
			if err := json.Unmarshal(raw, &value); err != nil {
				return err
			}
			target.Set(reflect.ValueOf(value))
			return nil
		default:
			return fmt.Errorf("unsupported slice type %s", target.Type())
		}
	default:
		return fmt.Errorf("unsupported kind %s", target.Kind())
	}
}

func decodeBase64ConfigJSON(encoded string) ([]byte, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil, fmt.Errorf("empty JSON base64 payload")
	}
	raw, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func resolveConfigPathWithJSONFallback(filename string) (string, configSourceFormat, error) {
	path, err := filepath.Abs(filename)
	if err != nil {
		return "", configSourceTOML, err
	}

	if _, err := os.Stat(path); err == nil {
		if strings.EqualFold(filepath.Ext(path), ".json") {
			return path, configSourceJSON, nil
		}
		return path, configSourceTOML, nil
	}

	jsonPath := path
	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		jsonPath = strings.TrimSuffix(path, filepath.Ext(path)) + ".json"
	case ".json":
		jsonPath = path
	default:
		jsonPath = path + ".json"
	}

	if _, err := os.Stat(jsonPath); err == nil {
		return jsonPath, configSourceJSON, nil
	}

	return "", configSourceTOML, fmt.Errorf("config file not found: %s", path)
}

func currentWorkingConfigDir() string {
	wd, err := os.Getwd()
	if err != nil || strings.TrimSpace(wd) == "" {
		return "."
	}
	return wd
}
