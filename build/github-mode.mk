# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#    
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

##@ GitHub Mode - Image Publishing and Helm Chart Setup

# Configuration for GitHub Mode image publishing
# These can be overridden via environment variables or command line:
#   make github-mode-publish GITHUB_USER=myuser GITHUB_TOKEN=ghp_xxx
GITHUB_USER ?= $(shell gh api user -q .login 2>/dev/null || echo "")
GITHUB_REPO ?= radius
GITHUB_MODE_REGISTRY ?= ghcr.io/$(GITHUB_USER)
GITHUB_MODE_TAG ?= github-mode

# Images required for Radius control plane
RADIUS_IMAGES := ucpd applications-rp dynamic-rp controller

.PHONY: github-mode-check
github-mode-check: ## Verify prerequisites for GitHub mode image publishing
	@echo "$(ARROW) Checking GitHub mode prerequisites..."
	@if [ -z "$(GITHUB_USER)" ]; then \
		echo "ERROR: GITHUB_USER not set. Run 'gh auth login' or set GITHUB_USER variable."; \
		exit 1; \
	fi
	@echo "  GitHub User: $(GITHUB_USER)"
	@echo "  Registry: $(GITHUB_MODE_REGISTRY)"
	@echo "  Tag: $(GITHUB_MODE_TAG)"
	@echo ""
	@echo "$(ARROW) Checking gh CLI authentication..."
	@gh auth status || (echo "ERROR: Not authenticated with gh CLI. Run 'gh auth login' first." && exit 1)
	@echo ""
	@echo "$(ARROW) Checking gh CLI scopes..."
	@if ! gh auth status 2>&1 | grep -q "write:packages"; then \
		echo ""; \
		echo "WARNING: Your gh CLI token may not have 'write:packages' scope."; \
		echo ""; \
		echo "To fix, run one of these options:"; \
		echo ""; \
		echo "  Option 1 - Re-authenticate with correct scopes:"; \
		echo "    gh auth login --scopes write:packages,read:packages"; \
		echo ""; \
		echo "  Option 2 - Use a Personal Access Token (classic):"; \
		echo "    1. Create PAT at: https://github.com/settings/tokens/new"; \
		echo "    2. Select scopes: write:packages, read:packages, delete:packages"; \
		echo "    3. Run: echo YOUR_TOKEN | docker login ghcr.io -u $(GITHUB_USER) --password-stdin"; \
		echo ""; \
	fi
	@echo "✓ Prerequisites check passed"

.PHONY: github-mode-login
github-mode-login: ## Authenticate Docker to ghcr.io using gh CLI token
	@echo "$(ARROW) Authenticating Docker to ghcr.io..."
	@echo ""
	@echo "If this fails with 'permission_denied', you need a token with write:packages scope."
	@echo "Run: gh auth login --scopes write:packages,read:packages"
	@echo "Or use a PAT: echo YOUR_PAT | docker login ghcr.io -u $(GITHUB_USER) --password-stdin"
	@echo ""
	@gh auth token | docker login ghcr.io -u $(GITHUB_USER) --password-stdin
	@echo "✓ Docker authenticated to ghcr.io"

.PHONY: github-mode-build
github-mode-build: copy-manifests ## Build all Radius control plane images for GitHub mode
	@echo "$(ARROW) Building Radius images for GitHub mode..."
	@echo "  Registry: $(GITHUB_MODE_REGISTRY)"
	@echo "  Tag: $(GITHUB_MODE_TAG)"
	@echo ""
	$(MAKE) DOCKER_REGISTRY=$(GITHUB_MODE_REGISTRY) DOCKER_TAG_VERSION=$(GITHUB_MODE_TAG) docker-build-ucpd
	$(MAKE) DOCKER_REGISTRY=$(GITHUB_MODE_REGISTRY) DOCKER_TAG_VERSION=$(GITHUB_MODE_TAG) docker-build-applications-rp
	$(MAKE) DOCKER_REGISTRY=$(GITHUB_MODE_REGISTRY) DOCKER_TAG_VERSION=$(GITHUB_MODE_TAG) docker-build-dynamic-rp
	$(MAKE) DOCKER_REGISTRY=$(GITHUB_MODE_REGISTRY) DOCKER_TAG_VERSION=$(GITHUB_MODE_TAG) docker-build-controller
	@echo ""
	@echo "✓ All images built successfully"
	@echo ""
	@echo "Images created:"
	@for img in $(RADIUS_IMAGES); do \
		echo "  $(GITHUB_MODE_REGISTRY)/$$img:$(GITHUB_MODE_TAG)"; \
	done

.PHONY: github-mode-push
github-mode-push: ## Push all Radius control plane images to GHCR
	@echo "$(ARROW) Pushing Radius images to ghcr.io..."
	@echo "  Registry: $(GITHUB_MODE_REGISTRY)"
	@echo "  Tag: $(GITHUB_MODE_TAG)"
	@echo ""
	$(MAKE) DOCKER_REGISTRY=$(GITHUB_MODE_REGISTRY) DOCKER_TAG_VERSION=$(GITHUB_MODE_TAG) docker-push-ucpd
	$(MAKE) DOCKER_REGISTRY=$(GITHUB_MODE_REGISTRY) DOCKER_TAG_VERSION=$(GITHUB_MODE_TAG) docker-push-applications-rp
	$(MAKE) DOCKER_REGISTRY=$(GITHUB_MODE_REGISTRY) DOCKER_TAG_VERSION=$(GITHUB_MODE_TAG) docker-push-dynamic-rp
	$(MAKE) DOCKER_REGISTRY=$(GITHUB_MODE_REGISTRY) DOCKER_TAG_VERSION=$(GITHUB_MODE_TAG) docker-push-controller
	@echo ""
	@echo "✓ All images pushed successfully"

.PHONY: github-mode-publish
github-mode-publish: github-mode-check github-mode-login github-mode-build github-mode-push ## Build and push all images to GHCR (full workflow)
	@echo ""
	@echo "============================================="
	@echo "GitHub Mode Images Published Successfully!"
	@echo "============================================="
	@echo ""
	@echo "Registry: $(GITHUB_MODE_REGISTRY)"
	@echo "Tag: $(GITHUB_MODE_TAG)"
	@echo ""
	@echo "Images available:"
	@for img in $(RADIUS_IMAGES); do \
		echo "  $(GITHUB_MODE_REGISTRY)/$$img:$(GITHUB_MODE_TAG)"; \
	done
	@echo ""
	@echo "To install Radius using these images, run:"
	@echo ""
	@echo "  rad install kubernetes \\"
	@echo "    --set global.imageRegistry=$(GITHUB_MODE_REGISTRY) \\"
	@echo "    --set global.imageTag=$(GITHUB_MODE_TAG)"
	@echo ""
	@echo "Or update workflows.go to use these values."

.PHONY: github-mode-values
github-mode-values: ## Generate values.yaml override for GitHub mode
	@echo "# GitHub Mode Helm Values Override"
	@echo "# Generated for user: $(GITHUB_USER)"
	@echo "# Usage: rad install kubernetes -f github-mode-values.yaml"
	@echo ""
	@echo "global:"
	@echo "  imageRegistry: $(GITHUB_MODE_REGISTRY)"
	@echo "  imageTag: $(GITHUB_MODE_TAG)"

.PHONY: github-mode-generate-values
github-mode-generate-values: ## Write github-mode-values.yaml file
	@echo "$(ARROW) Generating github-mode-values.yaml..."
	@echo "# GitHub Mode Helm Values Override" > github-mode-values.yaml
	@echo "# Generated for user: $(GITHUB_USER)" >> github-mode-values.yaml
	@echo "# Usage: rad install kubernetes -f github-mode-values.yaml" >> github-mode-values.yaml
	@echo "" >> github-mode-values.yaml
	@echo "global:" >> github-mode-values.yaml
	@echo "  imageRegistry: $(GITHUB_MODE_REGISTRY)" >> github-mode-values.yaml
	@echo "  imageTag: $(GITHUB_MODE_TAG)" >> github-mode-values.yaml
	@echo "✓ Created github-mode-values.yaml"

.PHONY: github-mode-info
github-mode-info: ## Show current GitHub mode configuration
	@echo "GitHub Mode Configuration"
	@echo "========================="
	@echo ""
	@echo "User:     $(GITHUB_USER)"
	@echo "Registry: $(GITHUB_MODE_REGISTRY)"
	@echo "Tag:      $(GITHUB_MODE_TAG)"
	@echo ""
	@echo "Images:"
	@for img in $(RADIUS_IMAGES); do \
		echo "  - $$img"; \
	done
	@echo ""
	@echo "Commands:"
	@echo "  make github-mode-check    - Verify prerequisites"
	@echo "  make github-mode-publish  - Build and push all images"
	@echo "  make github-mode-values   - Show Helm values override"

.PHONY: github-mode-workflow-update
github-mode-workflow-update: ## Show how to update workflows for custom images
	@echo "To use custom images in GitHub Actions workflows:"
	@echo ""
	@echo "1. Update the rad install command in generated workflows to include:"
	@echo ""
	@echo "   rad install kubernetes \\"
	@echo "     --set global.imageRegistry=$(GITHUB_MODE_REGISTRY) \\"
	@echo "     --set global.imageTag=$(GITHUB_MODE_TAG) \\"
	@echo "     --skip-contour-install \\"
	@echo "     --set dashboard.enabled=false"
	@echo ""
	@echo "2. Or set these as GitHub repository variables:"
	@echo "   - RADIUS_IMAGE_REGISTRY=$(GITHUB_MODE_REGISTRY)"
	@echo "   - RADIUS_IMAGE_TAG=$(GITHUB_MODE_TAG)"
