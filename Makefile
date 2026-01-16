.PHONY: build
build:
	go build -o pst

.PHONY: install
install:
	install -s -m u=rwx,go=rx,a+s pst /usr/local/bin
