MODULE ?=
BUMP ?= patch

.PHONY: release
release:
	@if [ -z "$(MODULE)" ]; then \
		echo "❌ You must set MODULE (e.g. make release MODULE=service2 BUMP=patch)"; \
		exit 1; \
	fi
	@if [ ! -d "$(MODULE)" ]; then \
		echo "❌ Module directory '$(MODULE)' not found"; \
		exit 1; \
	fi
	@if [ ! -f "$(MODULE)/go.mod" ]; then \
		echo "❌ No go.mod found in '$(MODULE)'"; \
		exit 1; \
	fi

	@LATEST_TAG=$$(git tag -l "$(MODULE)/v*" | sort -V | tail -n1); \
	if [ -z "$$LATEST_TAG" ]; then \
		echo "No tag found, starting at v0.1.0"; \
		NEW_VERSION="v0.1.0"; \
	else \
		VERSION=$${LATEST_TAG#$(MODULE)/}; \
		MAJOR=$$(echo $$VERSION | cut -d. -f1 | tr -d v); \
		MINOR=$$(echo $$VERSION | cut -d. -f2); \
		PATCH=$$(echo $$VERSION | cut -d. -f3); \
		case "$(BUMP)" in \
			major) MAJOR=$$((MAJOR + 1)); MINOR=0; PATCH=0 ;; \
			minor) MINOR=$$((MINOR + 1)); PATCH=0 ;; \
			patch) PATCH=$$((PATCH + 1)) ;; \
			*) echo "Invalid BUMP type: $(BUMP). Must be major|minor|patch"; exit 1 ;; \
		esac; \
		NEW_VERSION="v$${MAJOR}.$${MINOR}.$${PATCH}"; \
	fi; \
	NEW_TAG="$(MODULE)/$$NEW_VERSION"; \
	echo "🏷️  Creating tag $$NEW_TAG"; \
	git tag -a "$$NEW_TAG" -m "Release $$NEW_TAG"; \
	git push origin "$$NEW_TAG"; \
	echo "✅ Released $$NEW_TAG"
