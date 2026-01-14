.PHONY: release
release:
	@if [ -z "$(MODULE)" ]; then \
		echo "❌ You must set MODULE (e.g. make release MODULE=service2 BUMP=patch or MODULE=root BUMP=patch)"; \
		exit 1; \
	fi

	@if [ "$(MODULE)" = "root" ]; then \
		MODULE_PATH="."; \
	else \
		MODULE_PATH="$(MODULE)"; \
	fi; \
	\
	if [ ! -f "$$MODULE_PATH/go.mod" ]; then \
		echo "❌ No go.mod found in '$$MODULE_PATH'"; \
		exit 1; \
	fi; \
	\
	# Hämta senaste taggen
	if [ "$(MODULE)" = "root" ]; then \
		LATEST_TAG=$$(git tag -l "v*" | sort -V | tail -n1); \
	else \
		LATEST_TAG=$$(git tag -l "$(MODULE)/v*" | sort -V | tail -n1); \
	fi; \
	\
	if [ -z "$$LATEST_TAG" ]; then \
		echo "No tag found, starting at v0.1.0"; \
		NEW_VERSION="v0.1.0"; \
	else \
		if [ "$(MODULE)" = "root" ]; then \
			VERSION=$$LATEST_TAG; \
		else \
			VERSION=$${LATEST_TAG#$(MODULE)/}; \
		fi; \
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
	\
	if [ "$(MODULE)" = "root" ]; then \
		NEW_TAG="$$NEW_VERSION"; \
	else \
		NEW_TAG="$(MODULE)/$$NEW_VERSION"; \
	fi; \
	\
	echo "🏷️  Creating tag $$NEW_TAG"; \
	git tag -a "$$NEW_TAG" -m "Release $$NEW_TAG"; \
	git push origin "$$NEW_TAG"; \
	echo "✅ Released $$NEW_TAG"
