TARGETS := $(shell ls scripts)

.dapper:
	@echo Downloading dapper
	@curl -sL https://releases.rancher.com/dapper/latest/dapper-`uname -s`-`uname -m` > .dapper.tmp
	@@chmod +x .dapper.tmp
	@./.dapper.tmp -v
	@mv .dapper.tmp .dapper

export DAPPER_MODE := "auto"

ifeq ($(shell uname -s),Linux)
DAPPER_MODE = bind
endif

$(TARGETS): .dapper
	@./.dapper -q $@

.DEFAULT_GOAL := build

.PHONY: $(TARGETS)
