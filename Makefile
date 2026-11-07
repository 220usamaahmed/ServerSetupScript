PREFIX ?= $(HOME)/.local
SHARE  := $(PREFIX)/share/sss

.PHONY: build install test clean
build:
	go build -o sss .

# Copies the binary, steps/ and lib/ together and links sss into PATH.
install: build
	mkdir -p $(SHARE) $(PREFIX)/bin
	cp sss $(SHARE)/sss
	rm -rf $(SHARE)/steps $(SHARE)/lib
	cp -r steps lib $(SHARE)/
	ln -sf $(SHARE)/sss $(PREFIX)/bin/sss
	@echo "installed: $(PREFIX)/bin/sss -> $(SHARE)/sss"

# Needs Docker. See test/run.sh for variants (PLATFORM=linux/amd64, SSS_USER=plain, BASE=debian:12).
test:
	go vet ./...
	test/run.sh --yes

clean:
	rm -f sss
