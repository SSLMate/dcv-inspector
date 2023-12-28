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
