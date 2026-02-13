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

	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	client := NewClient()
	assert.NotNil(t, client)
	assert.False(t, client.Verbose)
}

func TestClient_Verbose(t *testing.T) {
	t.Parallel()

	client := NewClient()
	client.Verbose = true
	assert.True(t, client.Verbose)
}

func TestPullRequest_Fields(t *testing.T) {
	t.Parallel()

	pr := PullRequest{
		Number:      123,
		URL:         "https://github.com/owner/repo/pull/123",
		Title:       "Test PR",
		State:       "open",
		HeadRefName: "feature-branch",
		BaseRefName: "main",
	}

	assert.Equal(t, 123, pr.Number)
	assert.Equal(t, "https://github.com/owner/repo/pull/123", pr.URL)
	assert.Equal(t, "Test PR", pr.Title)
	assert.Equal(t, "open", pr.State)
	assert.Equal(t, "feature-branch", pr.HeadRefName)
	assert.Equal(t, "main", pr.BaseRefName)
}

func TestRepoInfo_FullName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		info     RepoInfo
		expected string
	}{
		{
			name: "standard repo",
			info: RepoInfo{
				Name: "radius",
				Owner: RepoOwner{
					Login: "radius-project",
				},
			},
			expected: "radius-project/radius",
		},
		{
			name: "user repo",
			info: RepoInfo{
				Name: "my-project",
				Owner: RepoOwner{
					Login: "username",
				},
			},
			expected: "username/my-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.info.FullName())
		})
	}
}

func TestRepoInfo_Fields(t *testing.T) {
	t.Parallel()

	info := RepoInfo{
		Name: "radius",
		Owner: RepoOwner{
			Login: "radius-project",
		},
		URL: "https://github.com/radius-project/radius",
		DefaultBranchRef: BranchRef{
			Name: "main",
		},
	}

	assert.Equal(t, "radius", info.Name)
	assert.Equal(t, "radius-project", info.Owner.Login)
	assert.Equal(t, "https://github.com/radius-project/radius", info.URL)
	assert.Equal(t, "main", info.DefaultBranchRef.Name)
}

func TestRepoOwner_Fields(t *testing.T) {
	t.Parallel()

	owner := RepoOwner{
		Login: "radius-project",
	}

	assert.Equal(t, "radius-project", owner.Login)
}

func TestBranchRef_Fields(t *testing.T) {
	t.Parallel()

	ref := BranchRef{
		Name: "main",
	}

	assert.Equal(t, "main", ref.Name)
}
