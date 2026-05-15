module openaide/backend/internal/infra

go 1.25.0

require (
	openaide/backend/internal/api v0.0.0
	openaide/backend/internal/config v0.0.0
	openaide/backend/internal/kernel v0.0.0
	openaide/backend/internal/llm v0.0.0
	openaide/backend/internal/memory v0.0.0
	openaide/backend/internal/orchestration v0.0.0
	openaide/backend/internal/tools v0.0.0
)

replace (
	openaide/backend/internal/api => ../api
	openaide/backend/internal/config => ../config
	openaide/backend/internal/kernel => ../kernel
	openaide/backend/internal/llm => ../llm
	openaide/backend/internal/memory => ../memory
	openaide/backend/internal/orchestration => ../orchestration
	openaide/backend/internal/tools => ../tools
)
