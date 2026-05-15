module openaide/backend/internal/api

go 1.25.0

require (
	openaide/backend/internal/kernel v0.0.0
	openaide/backend/internal/orchestration v0.0.0
)

replace (
	openaide/backend/internal/kernel => ../kernel
	openaide/backend/internal/orchestration => ../orchestration
)
