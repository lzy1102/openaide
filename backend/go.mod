module openaide/backend

go 1.25.0

require (
	golang.org/x/term v0.43.0
	gopkg.in/yaml.v3 v3.0.1
	openaide/backend/internal/infra v0.0.0-00010101000000-000000000000
	openaide/backend/internal/kernel v0.0.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	openaide/backend/internal/api v0.0.0 // indirect
	openaide/backend/internal/compress v0.0.0-00010101000000-000000000000 // indirect
	openaide/backend/internal/event v0.0.0-00010101000000-000000000000 // indirect
	openaide/backend/internal/llm v0.0.0 // indirect
	openaide/backend/internal/memory v0.0.0 // indirect
	openaide/backend/internal/orchestration v0.0.0 // indirect
	openaide/backend/internal/tools v0.0.0 // indirect
)

replace (
	openaide/backend/internal/api => ./internal/api
	openaide/backend/internal/compress => ./internal/compress
	openaide/backend/internal/config => ./internal/config
	openaide/backend/internal/event => ./internal/event
	openaide/backend/internal/git => ./internal/git
	openaide/backend/internal/identity => ./internal/identity
	openaide/backend/internal/index => ./internal/index
	openaide/backend/internal/infra => ./internal/infra
	openaide/backend/internal/kernel => ./internal/kernel
	openaide/backend/internal/knowledge => ./internal/knowledge
	openaide/backend/internal/llm => ./internal/llm
	openaide/backend/internal/memory => ./internal/memory
	openaide/backend/internal/orchestration => ./internal/orchestration
	openaide/backend/internal/tools => ./internal/tools
)
