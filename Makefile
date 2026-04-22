BINDIR ?= /usr/local/bin

.PHONY: build
build:
	go build $(GO_OPTS) -o pst

.PHONY: install
install:
	install -s -m u=rwx,go=rx,a+s pst $(BINDIR)

.PHONY: dock-install
dock-install:
	docker compose run --rm builder make build install
