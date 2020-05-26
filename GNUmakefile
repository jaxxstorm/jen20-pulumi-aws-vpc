SHELL = bash
PROJECT_ROOT := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

NODEJS_MAKEFILE := Makefile.nodejs
PYTHON_MAKEFILE := Makefile.python

.PHONY: bootstrap
bootstrap:
	@$(MAKE) -f $(NODEJS_MAKEFILE) $@
	@$(MAKE) -f $(PYTHON_MAKEFILE) $@

.PHONY: lint
lint:
	@$(MAKE) -f $(NODEJS_MAKEFILE) $@
	@$(MAKE) -f $(PYTHON_MAKEFILE) $@

.PHONY: test
test:
	@$(MAKE) -f $(NODEJS_MAKEFILE) $@
	@$(MAKE) -f $(PYTHON_MAKEFILE) $@

.PHONY: dist
dist:
	@$(MAKE) -f $(NODEJS_MAKEFILE) $@
	@$(MAKE) -f $(PYTHON_MAKEFILE) $@

.PHONY: travis
travis: bootstrap test lint dist
