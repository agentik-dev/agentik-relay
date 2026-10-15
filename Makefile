# The interpreter for the stdlib scripts, resolved once (the owner's ruling 2026-09-23 14:55 (5)).
PY := $(shell pyenv which python3 2>/dev/null || command -v python3)
# make through a variable: a literal $(MAKE) in check's line would run it under
# `make -n check` (the relay's verify probe), taking the lock or waiting on load.
UNLOCKED := $(MAKE)
DOCLINT_SHA := 3040666c1e9ad08faf1befe9c2efffaff0660f0464554d240f1d07437a03fcda
BINDIR ?= $(HOME)/.local/bin

JAVA ?= $(shell test -x /opt/homebrew/opt/openjdk/bin/java && echo /opt/homebrew/opt/openjdk/bin/java || command -v java)
TLA_JAR = design/tla/.tools/tla2tools.jar

.PHONY: help check check-unlocked test vet fmt lint doclint hygiene validate build install doctor readme readme-status readme-stamp clean tla tla-tools
help:
	@grep -E '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | sed 's/:.*## /\t/'
check:  ## the gate, bounded (conventions §5): one per tree (.check.lock), an 840 s wall, held above load 4 x cores only when the CPU confirms the load
	$(PY) -B scripts/bounded --lock .check.lock --wall 840 --load 4 -- $(UNLOCKED) check-unlocked
check-unlocked: test vet fmt doclint hygiene validate  ## the gate's body: test, vet, fmt, doclint, hygiene, validate, README freshness
	@if [ ! -f design/readme/source.md ]; then echo "readme: skipped, absent design/readme/source.md"; \
	elif shasum -c --status design/readme/rendered.sha256; then echo "readme: fresh"; \
	else echo "readme: stale, run make readme"; exit 2; fi
test:  ## go test (no network, no quota): every package but internal/night in one go test, beside one go test of internal/night per line of its testdata/shards.txt, all at once, non-zero when any is (AR84); the 15 min package timeout a ceiling
	test -s internal/night/testdata/shards.txt || { echo "test: internal/night/testdata/shards.txt is missing or empty, so no night test would run"; exit 2; }; \
	pids=; go test -race -timeout 15m $$(go list ./... | grep -v '/internal/night$$') & pids=$$!; \
	while read -r p; do go test -race -timeout 15m -run "$$p" ./internal/night & pids="$$pids $$!"; done < internal/night/testdata/shards.txt; \
	rc=0; for p in $$pids; do wait $$p || rc=$$?; done; exit $$rc
vet:  ## go vet
	go vet ./...
fmt:  ## gofmt: exit 2 naming the files it would change
	@files=$$(gofmt -l cmd internal); if [ -n "$$files" ]; then echo "fmt: gofmt would change:"; echo "$$files"; exit 2; fi
lint: vet  ## conventions §5 name for vet
doclint:  ## the doc gate (conventions §5), its bytes pinned (conventions §6)
	@echo "$(DOCLINT_SHA)  scripts/doclint" | shasum -a 256 -c --status || { echo "doclint: scripts/doclint is not conventions §6's copy"; exit 2; }
	$(PY) -B scripts/doclint
hygiene:  ## the public-hygiene scan (conventions §6); scripts/{agentik,hygiene,bounded} pinned by scripts/vendored.sha256 (agentik-commons scripts/sync)
	@shasum -a 256 -c --status scripts/vendored.sha256 || { echo "hygiene: scripts/{agentik,hygiene,bounded} are not agentik-commons' copies (scripts/vendored.sha256; run its scripts/sync)"; exit 2; }
	$(PY) -B scripts/hygiene
validate:  ## claude plugin validate plugin/ (doclint's plugin rule runs in check's doclint step)
	$(PY) -B scripts/bounded --wall 60 -- claude plugin validate plugin/
build:  ## bin/agentik-relay
	go build -o bin/agentik-relay ./cmd/agentik-relay
install:  ## agentik-relay into BINDIR, relay a link to it (until 2026-12-31), the agentik front door beside it; GOPATH's relay relinked when it exists (agentik-r1 §4.1, D3)
	mkdir -p $(BINDIR)
	go build -o $(BINDIR)/agentik-relay ./cmd/agentik-relay
	ln -sf agentik-relay $(BINDIR)/relay
	install -m 0755 scripts/agentik $(BINDIR)/agentik
	g="$$(go env GOPATH)/bin/relay"; if [ -e "$$g" ] || [ -L "$$g" ]; then ln -sf $(BINDIR)/agentik-relay "$$g"; fi
doctor:  ## claude, git, tmux on PATH
	go run ./cmd/agentik-relay doctor
readme-status:  ## rewrite the source's one Status: line (plugin version, today, the Go test count); idempotent
	@v=$$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' plugin/.claude-plugin/plugin.json); \
	n=$$(grep -rhc '^func Test' internal cmd | paste -sd+ - | bc); d=$$(date +%F); \
	sed "s/^Status: .*/Status: version \`$$v\`, rendered \`$$d\`, \`$$n\` Go tests./" design/readme/source.md > design/readme/source.md.tmp && \
	mv design/readme/source.md.tmp design/readme/source.md
readme-stamp:  ## write the two-line sidecar design/readme/rendered.sha256 (source, README)
	shasum -a 256 design/readme/source.md README.md > design/readme/rendered.sha256
readme: readme-status  ## render README.md from the source through respeak under design/readme/contract.txt, then stamp (spawns claude; never run by check)
	claude -p "/respeak:respeak technical design/readme/source.md; $$(cat design/readme/contract.txt)" --allowedTools 'Read Write' --max-turns 8
	@$(UNLOCKED) readme-stamp
tla:  ## TLC over design/tla (relay-r1 §16): the night clean, the two storms found; never fetches, never in check
	@JAVA="$(JAVA)" TLA_JAR="$(TLA_JAR)" design/tla/check.sh
e2e:  ## the night end to end (relay-r1 §11): nine cases over a fixture repo and the fake claude, about 3 minutes; never in check
	go test -tags e2e ./cmd/agentik-relay -run 'TestE2E_' -count=1 -v -timeout 12m
tla-tools: $(TLA_JAR)  ## fetch tla2tools.jar once into the gitignored design/tla/.tools/ (the network)
$(TLA_JAR):
	mkdir -p $(dir $(TLA_JAR))
	curl -fsSL -o $@ https://github.com/tlaplus/tlaplus/releases/latest/download/tla2tools.jar
clean:
	rm -rf bin
