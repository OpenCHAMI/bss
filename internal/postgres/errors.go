// MIT License
//
// Copyright © 2024-2025 Contributors to the OpenCHAMI Project
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

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
