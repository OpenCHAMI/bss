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
	"fmt"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
)

// makeKey creates a key from a key and subkey.  If key is not empty, it will
// be prepended with a '/' if it does not already start with one.  If subkey is
// not empty, it will be appended with a '/' if it does not already end with
// one.  The two will be concatenated with no '/' between them.
func makeKey(key, subkey string) string {
	ret := key
	if key != "" && key[0] != '/' {
		ret = "/" + key
	}
	if subkey != "" {
		if subkey[0] != '/' {
			ret += "/"
		}
		ret += subkey
	}
	return ret
}

// tagToColName extracts the field name from the JSON struct tag. Replace - with
// _.
// E.g: From `json:"params,omitempty"` comes `params`.
func tagToColName(tag string) string {
	re := regexp.MustCompile(`json:"([a-z0-9-]+)(?:,[a-z0-9-]+)*"`)
	colName := re.FindString(tag)
	return strings.Replace(colName, "-", "_", -1)
}

// fieldNameToColName converts the struct field name (in Pascal case) into
// the format for the database column (in snake case).
func fieldNameToColName(fieldName string) string {
	firstCap := regexp.MustCompile(`(.)([A-Z][a-z]+)`)
	allCaps := regexp.MustCompile(`([a-z0-9])([A-Z])`)
	colName := firstCap.ReplaceAllString(fieldName, `${1}_${2}`)
	colName = allCaps.ReplaceAllString(colName, `${1}_${2}`)
	return strings.ToLower(colName)
}

func stringSliceToSql(ss []string) string {
	if len(ss) == 0 {
		return "('')"
	}
	sep := ""
	s := "("
	for i, st := range ss {
		s += sep + fmt.Sprintf("'%s'", st)
		if i == len(ss)-1 {
			sep = ""
		} else {
			sep = ", "
		}
	}
	s += ")"
	return s
}

func int32SliceToSql(is []int32) string {
	sep := ""
	s := "("
	for i, in := range is {
		s += sep + fmt.Sprintf("%d", in)
		if i == len(is)-1 {
			sep = ""
		} else {
			sep = ", "
		}
	}
	s += ")"
	return s
}

// Return the intersection of a and b (matches) and those elements in b but not in a (exclusions).
func getMatches(a, b []string) (matches, exclusions []string) {
	for _, aVal := range a {
		aInB := false
		for _, bVal := range b {
			if aVal == bVal {
				matches = append(matches, aVal)
				aInB = true
				break
			}
		}
		if !aInB {
			exclusions = append(exclusions, aVal)
		}
	}
	return matches, exclusions
}

// Connect opens a new connections to a Postgres database and ensures it is reachable.
// If not, an error is thrown.
func Connect(host string, port uint, dbName, user, password string, ssl bool, extraDbOpts string) (BootDataDatabase, error) {
	var (
		sslmode string
		bddb    BootDataDatabase
	)
	if ssl {
		sslmode = "verify-full"
	} else {
		sslmode = "disable"
	}
	connStr := fmt.Sprintf("user=%s password=%s host=%s port=%d dbname=%s sslmode=%s", user, password, host, port, dbName, sslmode)
	if extraDbOpts != "" {
		connStr += " " + extraDbOpts
	}
	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		return bddb, err
	}
	// Create a new mapper which will use the struct field tag "json" instead of "db",
	// and ignore extra JSON config values, e.g. "omitempty".
	db.Mapper = reflectx.NewMapperTagFunc("json", fieldNameToColName, tagToColName)
	bddb.DB = db

	return bddb, err
}

// Close calls the Close() method on the database object within the BootDataDatabase. If it errs, an
// error is returned.
func (bddb BootDataDatabase) Close() error {
	return bddb.DB.Close()
}
