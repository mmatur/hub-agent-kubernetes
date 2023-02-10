default: dist

SRCS := $(shell find src/ -name '*.tsx' -o -name '*.ts')
STATICS := $(shell find public -type f)

start: yarn.lock
	@yarn start

lint: yarn.lock
	@yarn lint

dist: $(SRCS) $(STATICS) package.json
	@docker buildx build -f buildx.Dockerfile --output type=local,dest=. .

clean:
	rm -rf dist

.PHONY: clean start lint