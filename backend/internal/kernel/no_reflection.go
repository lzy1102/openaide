package kernel

import "context"

// NoReflection is a no-op Reflection implementation used for A/B testing
// to measure the impact of reflection on task performance.
type NoReflection struct{}

func (n *NoReflection) Reflect(ctx context.Context, sessionID string, execution ExecutionRecord) (*ReflectionResult, error) {
	return &ReflectionResult{
		Quality:    5,
		Issues:     []string{},
		Suggestions: []string{},
		Learned:    "",
	}, nil
}
