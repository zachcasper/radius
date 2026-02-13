/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package github

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// CommandRunner provides an interface for running external commands.
// This allows mocking of CLI commands like 'aws' and 'az' in tests.
type CommandRunner interface {
	// RunCommand executes a command and returns the stdout output.
	// Returns an error if the command fails.
	RunCommand(ctx context.Context, name string, args ...string) (string, error)
}

// DefaultCommandRunner is the default implementation of CommandRunner
// that executes actual shell commands.
type DefaultCommandRunner struct{}

// NewCommandRunner creates a new DefaultCommandRunner.
func NewCommandRunner() CommandRunner {
	return &DefaultCommandRunner{}
}

// RunCommand executes a command and returns the stdout output.
func (r *DefaultCommandRunner) RunCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("command '%s %v' failed: %w\nStderr: %s", name, args, err, stderr.String())
	}

	return stdout.String(), nil
}
