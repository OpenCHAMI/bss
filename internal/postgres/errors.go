// Copyright © 2024 Triad National Security, LLC. All rights reserved.
//
// This program was produced under U.S. Government contract 89233218CNA000001
// for Los Alamos National Laboratory (LANL), which is operated by Triad
// National Security, LLC for the U.S. Department of Energy/National Nuclear
// Security Administration. All rights in the program are reserved by Triad
// National Security, LLC, and the U.S. Department of Energy/National Nuclear
// Security Administration. The Government is granted for itself and others
// acting on its behalf a nonexclusive, paid-up, irrevocable worldwide license
// in this material to reproduce, prepare derivative works, distribute copies to
// the public, perform publicly and display publicly, and to permit others to do
// so.

package postgres

import (
	"errors"
	"fmt"
	"strings"
)

// ErrPostgresAdd represents an error emitted by the Add() function. The data
// structure contains the error it wraps.
type ErrPostgresAdd struct {
	Err error
}

func (epa ErrPostgresAdd) Error() string {
	return fmt.Sprintf("postgres.Add: %v", epa.Err)
}

func (epa ErrPostgresAdd) Is(e error) bool {
	return strings.HasPrefix(e.Error(), "postgres.Add: ") || errors.Is(e, epa.Err)
}

// ErrPostgresDelete represents an error emitted by the Delete() function. The
// data structure contains the error it wraps.
type ErrPostgresDelete struct {
	Err error
}

func (epd ErrPostgresDelete) Error() string {
	return fmt.Sprintf("postgres.Delete: %v", epd.Err)
}

func (epd ErrPostgresDelete) Is(e error) bool {
	return strings.HasPrefix(e.Error(), "postgres.Delete: ") || errors.Is(e, epd.Err)
}

// ErrPostgresUpdate represents an error emitted by the Update() function. The
// data structure contains the error it wraps.
type ErrPostgresUpdate struct {
	Err error
}

func (epu ErrPostgresUpdate) Error() string {
	return fmt.Sprintf("postgres.Update: %v", epu.Err)
}

func (epu ErrPostgresUpdate) Is(e error) bool {
	return strings.HasPrefix(e.Error(), "postgres.Update: ") || errors.Is(e, epu.Err)
}

// ErrPostgresSet represents an error emitted by the Set() function. The data
// structure contains the error it wraps.
type ErrPostgresSet struct {
	Err error
}

func (eps ErrPostgresSet) Error() string {
	return fmt.Sprintf("postgres.Set: %v", eps.Err)
}

func (eps ErrPostgresSet) Is(e error) bool {
	return strings.HasPrefix(e.Error(), "postgres.Set: ") || errors.Is(e, eps.Err)
}

// ErrPostgresGet represents an error emitted by any of the Get() functions. The
// data structure contains the error it wraps.
type ErrPostgresGet struct {
	Err error
}

func (epg ErrPostgresGet) Error() string {
	return fmt.Sprintf("postgres.Get: %v", epg.Err)
}

func (epg ErrPostgresGet) Is(e error) bool {
	return strings.HasPrefix(e.Error(), "postgres.Get: ") || errors.Is(e, epg.Err)
}

// ErrPostgresDuplicate represents an error that occurs when data being
// manipulated already exists in the database. The data being manipulated is
// contained in the data structure.
type ErrPostgresDuplicate struct {
	Data interface{}
}

func (epd ErrPostgresDuplicate) Error() string {
	var msg string
	switch d := epd.Data.(type) {
	case string:
		if d == "" {
			msg = "data already exists"
		} else {
			msg = fmt.Sprintf("data already exists: %s", d)
		}
	default:
		if d == nil {
			msg = "data already exists"
		} else {
			msg = fmt.Sprintf("data already exists: %v", d)
		}
	}
	return msg
}

func (epd ErrPostgresDuplicate) Is(e error) bool {
	return strings.HasPrefix(e.Error(), "data already exists")
}

// ErrPostgresNotExists represents an error that occurs when data being queried
// does not exist in the database. The data being queried is contained in the
// data structure.
type ErrPostgresNotExists struct {
	Data interface{}
}

func (epne ErrPostgresNotExists) Error() string {
	var msg string
	switch d := epne.Data.(type) {
	case string:
		if d == "" {
			msg = "data does not exist"
		} else {
			msg = fmt.Sprintf("data does not exist: %s", d)
		}
	default:
		if d == nil {
			msg = "data does not exist"
		} else {
			msg = fmt.Sprintf("data does not exist: %v", d)
		}
	}
	return msg
}

func (epne ErrPostgresNotExists) Is(e error) bool {
	return strings.HasPrefix(e.Error(), "data does not exist")
}
