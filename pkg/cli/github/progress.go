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
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ProgressState represents the current state of the progress model.
type ProgressState int

const (
	// ProgressStateWaiting indicates the workflow has not yet started.
	ProgressStateWaiting ProgressState = iota

	// ProgressStateQueued indicates the workflow is queued behind another run.
	ProgressStateQueued

	// ProgressStateRunning indicates the workflow is actively running.
	ProgressStateRunning

	// ProgressStateCompleted indicates the workflow completed successfully.
	ProgressStateCompleted

	// ProgressStateFailed indicates the workflow failed.
	ProgressStateFailed
)

var (
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))
	titleStyle2  = lipgloss.NewStyle().Bold(true)
	logStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#555555", Dark: "#AAAAAA"})
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	queuedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

// WorkflowStatusMsg is sent when the workflow status changes.
type WorkflowStatusMsg struct {
	State   ProgressState
	RunID   int64
	RunURL  string
	Message string
}

// WorkflowLogMsg is sent when new log lines are available.
type WorkflowLogMsg struct {
	Lines []string
}

// WorkflowErrorMsg is sent when an error occurs during workflow monitoring.
type WorkflowErrorMsg struct {
	Err error
}

// ProgressModel is a Bubble Tea model for displaying workflow progress
// with an animated spinner and automatic step status display.
type ProgressModel struct {
	// Title is the operation description displayed above the spinner.
	Title string

	// State tracks the current workflow state.
	State ProgressState

	// RunID is the GitHub Actions workflow run ID.
	RunID int64

	// RunURL is the URL to the workflow run in the GitHub UI.
	RunURL string

	// StatusMessage is the current status text shown next to the spinner.
	StatusMessage string

	// ErrorMessage is the error message displayed on failure.
	ErrorMessage string

	// LogLines contains the current step status lines from the workflow.
	LogLines []string

	// Done indicates the model should quit.
	Done bool

	// StepPollFunc is the function called to poll for workflow step status.
	// It is called automatically on each poll cycle.
	StepPollFunc func() tea.Msg

	// spinner is the animated spinner.
	spinner spinner.Model

	// elapsedStart tracks when the run started for elapsed time display.
	elapsedStart time.Time

	// pollFunc is the function called to poll for workflow status.
	// It returns a tea.Msg to update the model.
	pollFunc func() tea.Msg
}

// NewProgressModel creates a new ProgressModel with the given title.
func NewProgressModel(title string, pollFunc func() tea.Msg) ProgressModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	return ProgressModel{
		Title:         title,
		State:         ProgressStateWaiting,
		StatusMessage: "Waiting for workflow to start...",
		spinner:       s,
		pollFunc:      pollFunc,
	}
}

// Init returns the initial command for the progress model.
func (m ProgressModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.spinner.Tick,
		m.poll(),
	}
	if m.StepPollFunc != nil {
		cmds = append(cmds, m.pollSteps())
	}
	return tea.Batch(cmds...)
}

// Update handles messages and updates the model state.
func (m ProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.Done = true
			return m, tea.Quit
		}

	case spinner.TickMsg:
		if m.State == ProgressStateCompleted || m.State == ProgressStateFailed {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case WorkflowStatusMsg:
		m.State = msg.State
		m.RunID = msg.RunID
		if msg.RunURL != "" {
			m.RunURL = msg.RunURL
		}
		if msg.Message != "" {
			m.StatusMessage = msg.Message
		}

		switch msg.State {
		case ProgressStateQueued:
			m.StatusMessage = "Another deployment is in progress, this run is queued..."
		case ProgressStateRunning:
			if m.elapsedStart.IsZero() {
				m.elapsedStart = time.Now()
			}
		case ProgressStateCompleted:
			m.Done = true
			return m, tea.Quit
		case ProgressStateFailed:
			m.ErrorMessage = msg.Message
			m.Done = true
			return m, tea.Quit
		}

		// Continue polling
		return m, m.poll()

	case WorkflowLogMsg:
		if len(msg.Lines) > 0 {
			// Replace lines with latest snapshot (step status updates each poll)
			m.LogLines = msg.Lines
		}
		// Continue polling steps
		if m.StepPollFunc != nil && !m.Done {
			return m, m.pollSteps()
		}
		return m, nil

	case WorkflowErrorMsg:
		m.State = ProgressStateFailed
		m.ErrorMessage = msg.Err.Error()
		m.Done = true
		return m, tea.Quit
	}

	return m, nil
}

// View renders the progress model.
func (m ProgressModel) View() string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle2.Render(m.Title))
	b.WriteString("\n\n")

	switch m.State {
	case ProgressStateCompleted:
		b.WriteString(successStyle.Render("✓ "))
		b.WriteString(m.StatusMessage)
		b.WriteString("\n")
	case ProgressStateFailed:
		b.WriteString(errorStyle.Render("✗ "))
		b.WriteString(m.ErrorMessage)
		b.WriteString("\n")
	case ProgressStateQueued:
		b.WriteString(m.spinner.View())
		b.WriteString(" ")
		b.WriteString(queuedStyle.Render(m.StatusMessage))
		b.WriteString("\n")
	default:
		b.WriteString(m.spinner.View())
		b.WriteString(" ")
		b.WriteString(m.StatusMessage)
		if !m.elapsedStart.IsZero() {
			elapsed := time.Since(m.elapsedStart).Round(time.Second)
			b.WriteString(fmt.Sprintf(" (%s)", elapsed))
		}
		b.WriteString("\n")
	}

	// Step progress (always shown)
	if len(m.LogLines) > 0 {
		b.WriteString("\n")
		for _, line := range m.LogLines {
			b.WriteString(logStyle.Render("  " + line))
			b.WriteString("\n")
		}
	}

	// Run URL (below steps)
	if m.RunURL != "" {
		b.WriteString(fmt.Sprintf("\nView workflow: %s\n", m.RunURL))
	}

	return b.String()
}

// poll returns a tea.Cmd that calls the pollFunc after a delay.
func (m ProgressModel) poll() tea.Cmd {
	if m.pollFunc == nil {
		return nil
	}
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return m.pollFunc()
	})
}

// pollSteps returns a tea.Cmd that calls the StepPollFunc after a short delay.
func (m ProgressModel) pollSteps() tea.Cmd {
	if m.StepPollFunc == nil {
		return nil
	}
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return m.StepPollFunc()
	})
}

// IsSuccess returns true if the workflow completed successfully.
func (m ProgressModel) IsSuccess() bool {
	return m.State == ProgressStateCompleted
}

// IsFailure returns true if the workflow failed.
func (m ProgressModel) IsFailure() bool {
	return m.State == ProgressStateFailed
}
