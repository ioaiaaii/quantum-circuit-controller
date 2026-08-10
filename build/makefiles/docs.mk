# Documentation gates and assets.

##@ Documentation

# --offline keeps the gate hermetic so PR runs cannot flake on third-party
# outages; --include-fragments verifies heading anchors, where rot shows up.
.PHONY: docs-check
docs-check: ## Verify links and heading anchors in all tracked markdown (offline).
	git ls-files -co --exclude-standard '*.md' | while IFS= read -r f; do [ -f "$$f" ] && printf '%s\n' "$$f"; done | xargs lychee --offline --include-fragments --no-progress

# Manual: needs a live cluster with pre-seeded results, plus ttyd and ffmpeg
# alongside vhs. Preconditions and the display width are in the tape.
.PHONY: docs-demo
docs-demo: ## Record the README demo GIF (needs a running deployment).
	vhs docs/assets/demo.tape
	@ls -lh docs/assets/figures/qcc-demo.gif
