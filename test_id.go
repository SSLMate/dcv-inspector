// Copyright (C) 2023 Opsmate, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.
//
// Except as contained in this notice, the name(s) of the above copyright
// holders shall not be used in advertising or otherwise to promote the
// sale, use or other dealings in this Software without prior written
// authorization.

package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strings"
)

type testID [16]byte

func (testID testID) String() string {
	return hex.EncodeToString(testID[:])
}

func generateTestID() testID {
	var id testID
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return id
}

func parseHostname(hostname string) (testID, bool) {
	hostname = strings.TrimSuffix(hostname, ".")
	prefix, found := strings.CutSuffix(hostname, ".test."+domain)
	if !found {
		return testID{}, false
	}
	lastDot := strings.LastIndexByte(prefix, '.')
	testIDStr := prefix[lastDot+1:]
	return parseTestID(testIDStr)
}

func parseTestID(testIDStr string) (testID, bool) {
	if len(testIDStr) != 32 {
		return testID{}, false
	}
	testIDSlice, err := hex.DecodeString(testIDStr)
	if err != nil {
		return testID{}, false
	}
	return ([16]byte)(testIDSlice), true
}

func isRunningTest(ctx context.Context, id testID) (bool, error) {
	var stoppedAt sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT stopped_at FROM test WHERE test_id = ?`, id[:]).Scan(&stoppedAt); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, err
	} else if stoppedAt.Valid {
		return false, nil
	} else {
		return true, nil
	}
}
