lint:
	@echo "linting all go files..."  &&\
	goimports -w $(shell find . -name "*.go")

append_commit: lint
	@git add .
	@git commit --amend --no-edit
	@echo "appended commit"