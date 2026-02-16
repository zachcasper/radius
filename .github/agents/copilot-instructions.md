# github-radius Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-02-12

## Active Technologies
- Go 1.25.7 + Cobra (CLI framework), Viper (config), `gh` CLI (GitHub API), `go-git/v5` (Git operations), `gopkg.in/yaml.v3` (YAML marshaling), `go.uber.org/mock` (test mocks) (001-github-mode)
- GitHub Environments API (env variables), GitHub repository variables, Git repository (`.radius/` directory tree), cloud backends (S3/Azure Storage for Terraform state) (001-github-mode)

- Go 1.21+ (per go.mod) + spf13/cobra (CLI), spf13/viper (config), go-git/go-git (git operations), hashicorp/terraform-exec (Terraform execution), aws-sdk-go-v2 (AWS OIDC), azure-sdk-for-go (Azure OIDC) (001-github-mode)

## Project Structure

```text
src/
tests/
```

## Commands

# Add commands for Go 1.21+ (per go.mod)

## Code Style

Go 1.21+ (per go.mod): Follow standard conventions

## Recent Changes
- 001-github-mode: Added Go 1.25.7 + Cobra (CLI framework), Viper (config), `gh` CLI (GitHub API), `go-git/v5` (Git operations), `gopkg.in/yaml.v3` (YAML marshaling), `go.uber.org/mock` (test mocks)

- 001-github-mode: Added Go 1.21+ (per go.mod) + spf13/cobra (CLI), spf13/viper (config), go-git/go-git (git operations), hashicorp/terraform-exec (Terraform execution), aws-sdk-go-v2 (AWS OIDC), azure-sdk-for-go (Azure OIDC)

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
