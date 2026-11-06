// SPDX-FileCopyrightText: Copyright © 2026 OpenCHAMI a Series of LF Projects, LLC
// SPDX-License-Identifier: MIT

package main

import (
	"sort"
	"testing"

	hmetcd "github.com/Cray-HPE/hms-hmetcd"
)

// Isolate fixtures from hmetcd's process-global mem: map. Tests using this
// store are serial and restore the original backend after every case.
type isolatedBootStore struct {
	hmetcd.Kvi
	records map[string]string
}

func (s *isolatedBootStore) Get(key string) (string, bool, error) {
	value, ok := s.records[key]
	return value, ok, nil
}

func (s *isolatedBootStore) Store(key, value string) error {
	s.records[key] = value
	return nil
}

func (s *isolatedBootStore) GetRange(start, end string) ([]hmetcd.Kvi_KV, error) {
	var keys []string
	for key := range s.records {
		if key >= start && key < end {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := make([]hmetcd.Kvi_KV, 0, len(keys))
	for _, key := range keys {
		result = append(result, hmetcd.Kvi_KV{Key: key, Value: s.records[key]})
	}
	return result, nil
}

func useIsolatedBootStore(t *testing.T) {
	t.Helper()
	original := kvstore
	kvstore = &isolatedBootStore{Kvi: original, records: map[string]string{}}
	t.Cleanup(func() { kvstore = original })
}
