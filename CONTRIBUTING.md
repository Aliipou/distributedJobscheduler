# Contributing to distributedJobscheduler

## Setup

```bash
git clone https://github.com/Aliipou/distributedJobscheduler.git
cd distributedJobscheduler
go mod download
docker run -d -p 6379:6379 redis:7-alpine  # Redis for tests
```

## Running Tests

```bash
go test ./... -v -race -count=1
```

Integration tests require Redis running on `localhost:6379`.

## Code Style

- Format with `gofmt`
- Lint with `golangci-lint run`
- All exported symbols need GoDoc comments
- Errors must be wrapped: `fmt.Errorf("operation: %w", err)`
- Avoid global state — pass dependencies explicitly

## Adding a New Queue Type

1. Define the queue name constant in `queue/constants.go`
2. Add the priority weight in `queue/priority.go`
3. Update the worker configuration schema
4. Add tests in `queue/queue_test.go`
5. Document in README

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):
`feat:`, `fix:`, `docs:`, `test:`, `perf:`, `chore:`

## Pull Requests

1. Fork and create a feature branch
2. Write tests — aim for the same coverage as surrounding code
3. Run `golangci-lint run` before pushing
4. Describe the motivation and approach in the PR description
