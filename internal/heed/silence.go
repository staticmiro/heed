package heed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type SilenceRule struct {
	Check string    `json:"check,omitempty"`
	Until time.Time `json:"until"`
}

type SilenceManager struct {
	path string
}

func NewSilenceManager(dataFile string) *SilenceManager {
	dir := filepath.Dir(dataFile)
	return &SilenceManager{
		path: filepath.Join(dir, ".heed-silence.json"),
	}
}

func (s *SilenceManager) AddRule(rule SilenceRule) error {
	rules, err := s.Load()
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Clean up expired rules
	now := time.Now()
	var active []SilenceRule
	for _, r := range rules {
		if r.Until.After(now) {
			active = append(active, r)
		}
	}
	active = append(active, rule)

	data, err := json.MarshalIndent(active, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *SilenceManager) Load() ([]SilenceRule, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var rules []SilenceRule
	if len(data) == 0 {
		return rules, nil
	}
	err = json.Unmarshal(data, &rules)
	return rules, err
}

func (s *SilenceManager) IsSilenced(check string) bool {
	rules, err := s.Load()
	if err != nil || len(rules) == 0 {
		return false
	}
	now := time.Now()
	for _, r := range rules {
		if r.Until.After(now) {
			if r.Check == "" || r.Check == check {
				return true
			}
		}
	}
	return false
}
