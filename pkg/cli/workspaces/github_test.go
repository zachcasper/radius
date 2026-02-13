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

package workspaces

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGitHubConnectionConfig_GetKind(t *testing.T) {
	config := &GitHubConnectionConfig{
		Kind: KindGitHub,
		URL:  "https://github.com/owner/repo",
	}

	require.Equal(t, KindGitHub, config.GetKind())
}

func TestGitHubConnectionConfig_String(t *testing.T) {
	config := &GitHubConnectionConfig{
		Kind: KindGitHub,
		URL:  "https://github.com/owner/repo",
	}

	require.Equal(t, "GitHub (url=https://github.com/owner/repo)", config.String())
}

func TestGitHubConnectionConfig_Connect_ReturnsError(t *testing.T) {
	config := &GitHubConnectionConfig{
		Kind: KindGitHub,
		URL:  "https://github.com/owner/repo",
	}

	conn, err := config.Connect()
	require.Nil(t, conn)
	require.Error(t, err)
	require.Contains(t, err.Error(), "GitHub workspaces do not support direct connections")
}

func TestGitHubConnectionConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *GitHubConnectionConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid github.com URL",
			config: &GitHubConnectionConfig{
				Kind: KindGitHub,
				URL:  "https://github.com/owner/repo",
			},
			wantErr: false,
		},
		{
			name: "valid github.com URL with .git suffix",
			config: &GitHubConnectionConfig{
				Kind: KindGitHub,
				URL:  "https://github.com/owner/repo.git",
			},
			wantErr: false,
		},
		{
			name: "empty URL",
			config: &GitHubConnectionConfig{
				Kind: KindGitHub,
				URL:  "",
			},
			wantErr: true,
			errMsg:  "GitHub connection URL is required",
		},
		{
			name: "non-GitHub URL",
			config: &GitHubConnectionConfig{
				Kind: KindGitHub,
				URL:  "https://gitlab.com/owner/repo",
			},
			wantErr: true,
			errMsg:  "URL must be a GitHub repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGitHubConnectionConfig_ParseOwnerRepo(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "simple URL",
			url:       "https://github.com/radius-project/radius",
			wantOwner: "radius-project",
			wantRepo:  "radius",
			wantErr:   false,
		},
		{
			name:      "URL with .git suffix",
			url:       "https://github.com/radius-project/radius.git",
			wantOwner: "radius-project",
			wantRepo:  "radius",
			wantErr:   false,
		},
		{
			name:    "invalid URL - no repo",
			url:     "https://github.com/owner",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &GitHubConnectionConfig{
				Kind: KindGitHub,
				URL:  tt.url,
			}

			owner, repo, err := config.ParseOwnerRepo()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantOwner, owner)
				require.Equal(t, tt.wantRepo, repo)
			}
		})
	}
}

func TestWorkspace_GitHubURL(t *testing.T) {
	t.Run("GitHub workspace returns URL", func(t *testing.T) {
		ws := Workspace{
			Connection: map[string]any{
				"kind": KindGitHub,
				"url":  "https://github.com/owner/repo",
			},
		}

		url, ok := ws.GitHubURL()
		require.True(t, ok)
		require.Equal(t, "https://github.com/owner/repo", url)
	})

	t.Run("Kubernetes workspace returns empty", func(t *testing.T) {
		ws := Workspace{
			Connection: map[string]any{
				"kind":    KindKubernetes,
				"context": "my-context",
			},
		}

		url, ok := ws.GitHubURL()
		require.False(t, ok)
		require.Empty(t, url)
	})
}

func TestWorkspace_IsGitHubWorkspace(t *testing.T) {
	t.Run("GitHub workspace", func(t *testing.T) {
		ws := Workspace{
			Connection: map[string]any{
				"kind": KindGitHub,
				"url":  "https://github.com/owner/repo",
			},
		}

		require.True(t, ws.IsGitHubWorkspace())
	})

	t.Run("Kubernetes workspace", func(t *testing.T) {
		ws := Workspace{
			Connection: map[string]any{
				"kind":    KindKubernetes,
				"context": "my-context",
			},
		}

		require.False(t, ws.IsGitHubWorkspace())
	})
}

func TestWorkspace_IsSameGitHubURL(t *testing.T) {
	ws := Workspace{
		Connection: map[string]any{
			"kind": KindGitHub,
			"url":  "https://github.com/owner/repo",
		},
	}

	require.True(t, ws.IsSameGitHubURL("https://github.com/owner/repo"))
	require.False(t, ws.IsSameGitHubURL("https://github.com/other/repo"))
}

func TestWorkspace_ConnectionConfig_GitHub(t *testing.T) {
	ws := Workspace{
		Connection: map[string]any{
			"kind": KindGitHub,
			"url":  "https://github.com/owner/repo",
		},
	}

	config, err := ws.ConnectionConfig()
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Equal(t, KindGitHub, config.GetKind())

	ghConfig, ok := config.(*GitHubConnectionConfig)
	require.True(t, ok)
	require.Equal(t, "https://github.com/owner/repo", ghConfig.URL)
}
