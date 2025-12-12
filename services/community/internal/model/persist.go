package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "/" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func readJSONFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return errors.New("empty json file")
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("unmarshal json: %w", err)
	}
	return nil
}

func writeJSONAtomic(path string, v any) error {
	if err := ensureParentDir(path); err != nil {
		return err
	}

	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	b = append(b, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
