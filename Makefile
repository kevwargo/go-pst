.PHONY: build
build:
	go build

.PHONY: install
install:
	install -s -m u=rwx,go=rx,a+s go-pst /usr/local/bin
