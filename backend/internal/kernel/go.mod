module openaide/backend/internal/kernel

go 1.25.0

require (
	github.com/google/uuid v1.6.0
	openaide/backend/src/models v0.0.0
	openaide/backend/src/services/llm v0.0.0
)

replace (
	openaide/backend/src/models => ../../src/models
	openaide/backend/src/services/llm => ../../src/services/llm
)
