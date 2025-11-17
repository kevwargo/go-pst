.PHONY: build
build:
	go build

.PHONY: install
install:
	install -s go-pst /usr/local/bin
