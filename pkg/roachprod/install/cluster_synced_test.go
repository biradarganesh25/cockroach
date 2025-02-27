// Copyright 2021 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package install

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach/pkg/roachprod/logger"
	"github.com/cockroachdb/cockroach/pkg/util/retry"
	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// TestRoachprodEnv tests the roachprodEnvRegex and roachprodEnvValue methods.
func TestRoachprodEnv(t *testing.T) {
	cases := []struct {
		clusterName string
		node        Node
		tag         string
		value       string
		regex       string
	}{
		{
			clusterName: "a",
			node:        1,
			tag:         "",
			value:       "1",
			regex:       `ROACHPROD=1[ \/]`,
		},
		{
			clusterName: "local-foo",
			node:        2,
			tag:         "",
			value:       "local-foo/2",
			regex:       `ROACHPROD=local-foo\/2[ \/]`,
		},
		{
			clusterName: "a",
			node:        3,
			tag:         "foo",
			value:       "3/foo",
			regex:       `ROACHPROD=3\/foo[ \/]`,
		},
		{
			clusterName: "a",
			node:        4,
			tag:         "foo/bar",
			value:       "4/foo/bar",
			regex:       `ROACHPROD=4\/foo\/bar[ \/]`,
		},
		{
			clusterName: "local-foo",
			node:        5,
			tag:         "tag",
			value:       "local-foo/5/tag",
			regex:       `ROACHPROD=local-foo\/5\/tag[ \/]`,
		},
	}

	for idx, tc := range cases {
		t.Run(fmt.Sprintf("%d", idx+1), func(t *testing.T) {
			var c SyncedCluster
			c.Name = tc.clusterName
			c.Tag = tc.tag
			if value := c.roachprodEnvValue(tc.node); value != tc.value {
				t.Errorf("expected value `%s`, got `%s`", tc.value, value)
			}
			if regex := c.roachprodEnvRegex(tc.node); regex != tc.regex {
				t.Errorf("expected regex `%s`, got `%s`", tc.regex, regex)
			}
		})
	}
}

func TestRunWithMaybeRetry(t *testing.T) {
	var testRetryOpts = retry.Options{
		InitialBackoff: 10 * time.Millisecond,
		Multiplier:     2,
		MaxBackoff:     1 * time.Second,
		// This will run a total of 3 times `runWithMaybeRetry`
		MaxRetries: 2,
	}

	l := nilLogger()

	attempt := 0
	cases := []struct {
		f                func() (*RunResultDetails, error)
		shouldRetryFn    func(*RunResultDetails) bool
		expectedAttempts int
		shouldError      bool
	}{
		{ // Happy path: no error, no retry required
			f: func() (*RunResultDetails, error) {
				return newResult(0), nil
			},
			expectedAttempts: 1,
			shouldError:      false,
		},
		{ // Error, but not retry function specified
			f: func() (*RunResultDetails, error) {
				return newResult(1), nil
			},
			expectedAttempts: 1,
			shouldError:      true,
		},
		{ // Error, with retries exhausted
			f: func() (*RunResultDetails, error) {
				return newResult(255), nil
			},
			shouldRetryFn:    func(d *RunResultDetails) bool { return d.RemoteExitStatus == 255 },
			expectedAttempts: 3,
			shouldError:      true,
		},
		{ // Eventual success after retries
			f: func() (*RunResultDetails, error) {
				attempt++
				if attempt == 3 {
					return newResult(0), nil
				}
				return newResult(255), nil
			},
			shouldRetryFn:    func(d *RunResultDetails) bool { return d.RemoteExitStatus == 255 },
			expectedAttempts: 3,
			shouldError:      false,
		},
	}

	for idx, tc := range cases {
		attempt = 0
		t.Run(fmt.Sprintf("%d", idx+1), func(t *testing.T) {
			res, _ := runWithMaybeRetry(l, testRetryOpts, tc.shouldRetryFn, tc.f)

			require.Equal(t, tc.shouldError, res.Err != nil)
			require.Equal(t, tc.expectedAttempts, res.Attempt)

			if tc.shouldError && tc.expectedAttempts == 3 {
				require.True(t, errors.Is(res.Err, ErrAfterRetry))
			}
		})
	}
}

func newResult(exitCode int) *RunResultDetails {
	var err error
	if exitCode != 0 {
		err = errors.Newf("Error with exit code %v", exitCode)
	}
	return &RunResultDetails{RemoteExitStatus: exitCode, Err: err}
}

func nilLogger() *logger.Logger {
	lcfg := logger.Config{
		Stdout: io.Discard,
		Stderr: io.Discard,
	}
	l, err := lcfg.NewLogger("" /* path */)
	if err != nil {
		panic(err)
	}
	return l
}
