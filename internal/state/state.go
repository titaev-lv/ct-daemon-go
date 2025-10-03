package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type DaemonState struct {
	Active bool `json:"active"`
}

func LoadState(file string) *DaemonState {
	st := &DaemonState{Active: false}

	// Создать директорию для state-файла, если не существует
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0755); err != nil {
		// Если не можем создать директорию, возвращаем дефолтное состояние
		return st
	}

	f, err := os.ReadFile(file)
	if err == nil {
		json.Unmarshal(f, st)
	}
	// Если файл не существует, создаем его с дефолтным состоянием
	if err != nil {
		SaveState(file, st)
	}
	return st
}

func SaveState(file string, st *DaemonState) {
	// Создать директорию для state-файла, если не существует
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0755); err != nil {
		// Если не можем создать директорию, просто возвращаемся
		return
	}

	data, _ := json.Marshal(st)
	os.WriteFile(file, data, 0644)
}

func SetActive(stateFile string, active bool) *DaemonState {
	st := &DaemonState{Active: active}
	// Создать директорию для state-файла, если не существует
	dir := filepath.Dir(stateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		// Не критично, просто не сохраним state
		return st
	}
	SaveState(stateFile, st)
	return st
}
