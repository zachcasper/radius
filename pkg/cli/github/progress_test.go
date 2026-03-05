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
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestProgressState_Constants(t *testing.T) {
	t.Parallel()

	// Verify all progress states are defined
	require.Equal(t, ProgressState(0), ProgressStateWaiting)
	require.Equal(t, ProgressState(1), ProgressStateQueued)
	require.Equal(t, ProgressState(2), ProgressStateRunning)
	require.Equal(t, ProgressState(3), ProgressStateCompleted)
	require.Equal(t, ProgressState(4), ProgressStateFailed)
}

func TestNewProgressModel(t *testing.T) {
	t.Parallel()

	pollFunc := func() tea.Msg { return nil }
	model := NewProgressModel("Test Title", pollFunc)

	require.Equal(t, "Test Title", model.Title)
	require.Equal(t, ProgressStateWaiting, model.State)
	require.Equal(t, "Waiting for workflow to start...", model.StatusMessage)
	require.False(t, model.Done)
	require.Zero(t, model.RunID)
	require.Empty(t, model.RunURL)
	require.Empty(t, model.ErrorMessage)
	require.Nil(t, model.LogLines)
}

func TestProgressModel_Update_WorkflowStatusMsg_Queued(t *testing.T) {
	t.Parallel()

	pollFunc := func() tea.Msg { return nil }
	model := NewProgressModel("Test", pollFunc)

	msg := WorkflowStatusMsg{
		State:   ProgressStateQueued,
		RunID:   12345,
		RunURL:  "https://github.com/owner/repo/actions/runs/12345",
		Message: "Queued",
	}

	newModel, _ := model.Update(msg)
	pm := newModel.(ProgressModel)

	require.Equal(t, ProgressStateQueued, pm.State)
	require.Equal(t, int64(12345), pm.RunID)
	require.Equal(t, "https://github.com/owner/repo/actions/runs/12345", pm.RunURL)
	require.Contains(t, pm.StatusMessage, "in progress")
	require.False(t, pm.Done)
}

func TestProgressModel_Update_WorkflowStatusMsg_Running(t *testing.T) {
	t.Parallel()

	pollFunc := func() tea.Msg { return nil }
	model := NewProgressModel("Test", pollFunc)

	msg := WorkflowStatusMsg{
		State:   ProgressStateRunning,
		RunID:   12345,
		RunURL:  "https://github.com/owner/repo/actions/runs/12345",
		Message: "Running",
	}

	newModel, _ := model.Update(msg)
	pm := newModel.(ProgressModel)

	require.Equal(t, ProgressStateRunning, pm.State)
	require.Equal(t, int64(12345), pm.RunID)
	require.False(t, pm.Done)
	// elapsedStart should be set
	require.False(t, pm.elapsedStart.IsZero())
}

func TestProgressModel_Update_WorkflowStatusMsg_Completed(t *testing.T) {
	t.Parallel()

	pollFunc := func() tea.Msg { return nil }
	model := NewProgressModel("Test", pollFunc)

	msg := WorkflowStatusMsg{
		State:   ProgressStateCompleted,
		RunID:   12345,
		RunURL:  "https://github.com/owner/repo/actions/runs/12345",
		Message: "Completed successfully",
	}

	newModel, cmd := model.Update(msg)
	pm := newModel.(ProgressModel)

	require.Equal(t, ProgressStateCompleted, pm.State)
	require.True(t, pm.Done)
	// Should return tea.Quit command
	require.NotNil(t, cmd)
}

func TestProgressModel_Update_WorkflowStatusMsg_Failed(t *testing.T) {
	t.Parallel()

	pollFunc := func() tea.Msg { return nil }
	model := NewProgressModel("Test", pollFunc)

	msg := WorkflowStatusMsg{
		State:   ProgressStateFailed,
		RunID:   12345,
		RunURL:  "https://github.com/owner/repo/actions/runs/12345",
		Message: "Job failed with exit code 1",
	}

	newModel, cmd := model.Update(msg)
	pm := newModel.(ProgressModel)

	require.Equal(t, ProgressStateFailed, pm.State)
	require.True(t, pm.Done)
	require.Equal(t, "Job failed with exit code 1", pm.ErrorMessage)
	// Should return tea.Quit command
	require.NotNil(t, cmd)
}

func TestProgressModel_Update_WorkflowLogMsg(t *testing.T) {
	t.Parallel()

	pollFunc := func() tea.Msg { return nil }
	model := NewProgressModel("Test", pollFunc)

	msg := WorkflowLogMsg{
		Lines: []string{"Step 1: Checkout", "Step 2: Install Radius"},
	}

	newModel, _ := model.Update(msg)
	pm := newModel.(ProgressModel)

	require.Len(t, pm.LogLines, 2)
	require.Equal(t, "Step 1: Checkout", pm.LogLines[0])
	require.Equal(t, "Step 2: Install Radius", pm.LogLines[1])
}

func TestProgressModel_Update_KeyMsg_CtrlC(t *testing.T) {
	t.Parallel()

	pollFunc := func() tea.Msg { return nil }
	model := NewProgressModel("Test", pollFunc)

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}

	newModel, cmd := model.Update(msg)
	pm := newModel.(ProgressModel)

	require.True(t, pm.Done)
	// Should return tea.Quit command
	require.NotNil(t, cmd)
}

func TestWorkflowStatusMsg_Fields(t *testing.T) {
	t.Parallel()

	msg := WorkflowStatusMsg{
		State:   ProgressStateRunning,
		RunID:   12345,
		RunURL:  "https://github.com/owner/repo/actions/runs/12345",
		Message: "Running step 3 of 5",
	}

	require.Equal(t, ProgressStateRunning, msg.State)
	require.Equal(t, int64(12345), msg.RunID)
	require.Equal(t, "https://github.com/owner/repo/actions/runs/12345", msg.RunURL)
	require.Equal(t, "Running step 3 of 5", msg.Message)
}

func TestWorkflowLogMsg_Fields(t *testing.T) {
	t.Parallel()

	msg := WorkflowLogMsg{
		Lines: []string{"line1", "line2", "line3"},
	}

	require.Len(t, msg.Lines, 3)
	require.Equal(t, "line1", msg.Lines[0])
	require.Equal(t, "line2", msg.Lines[1])
	require.Equal(t, "line3", msg.Lines[2])
}

func TestWorkflowErrorMsg_Fields(t *testing.T) {
	t.Parallel()

	msg := WorkflowErrorMsg{
		Err: nil,
	}

	require.Nil(t, msg.Err)
}

func TestProgressModel_Init_ReturnsCommands(t *testing.T) {
	t.Parallel()

	pollFunc := func() tea.Msg { return nil }
	model := NewProgressModel("Test", pollFunc)

	cmd := model.Init()
	require.NotNil(t, cmd)
}

func TestProgressModel_View_NotEmpty(t *testing.T) {
	t.Parallel()

	pollFunc := func() tea.Msg { return nil }
	model := NewProgressModel("Test", pollFunc)

	view := model.View()
	require.NotEmpty(t, view)
	require.Contains(t, view, "Test")
}
