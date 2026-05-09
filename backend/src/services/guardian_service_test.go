package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGuardianService_Review_AllowReadOnly(t *testing.T) {
	svc := NewGuardianService(nil)
	svc.SetEnabled(true)

	review, err := svc.Review(nil, "read_file", `{"path": "/tmp/test.txt"}`, "")
	assert.NoError(t, err)
	assert.Equal(t, VerdictAllow, review.Verdict)
}

func TestGuardianService_Review_AllowSearch(t *testing.T) {
	svc := NewGuardianService(nil)
	svc.SetEnabled(true)

	review, err := svc.Review(nil, "search", `{"query": "auth"}`, "")
	assert.NoError(t, err)
	assert.Equal(t, VerdictAllow, review.Verdict)
}

func TestGuardianService_Review_AllowWebSearch(t *testing.T) {
	svc := NewGuardianService(nil)
	svc.SetEnabled(true)

	review, err := svc.Review(nil, "web_search", `{"query": "golang best practices"}`, "")
	assert.NoError(t, err)
	assert.Equal(t, VerdictAllow, review.Verdict)
}

func TestGuardianService_Review_DenyDangerousPattern(t *testing.T) {
	svc := NewGuardianService(nil)
	svc.SetEnabled(true)

	review, err := svc.Review(nil, "execute_command", `{"command": "rm -rf /"}`, "")
	assert.NoError(t, err)
	assert.Equal(t, VerdictDeny, review.Verdict)
}

func TestGuardianService_Review_DenyRecursiveForceDelete(t *testing.T) {
	svc := NewGuardianService(nil)
	svc.SetEnabled(true)

	review, err := svc.Review(nil, "execute_command", `{"command": "rm -rf /home"}`, "")
	assert.NoError(t, err)
	assert.Equal(t, VerdictDeny, review.Verdict)
}

func TestGuardianService_Review_Disabled(t *testing.T) {
	svc := NewGuardianService(nil)
	svc.SetEnabled(false)

	review, err := svc.Review(nil, "execute_command", `{"command": "rm -rf /"}`, "")
	assert.NoError(t, err)
	assert.Equal(t, VerdictAllow, review.Verdict, "disabled guardian should allow everything")
}

func TestGuardianService_Toggle(t *testing.T) {
	svc := NewGuardianService(nil)

	assert.True(t, svc.IsEnabled())

	svc.SetEnabled(false)
	assert.False(t, svc.IsEnabled())

	svc.SetEnabled(true)
	assert.True(t, svc.IsEnabled())
}

func TestGuardianService_DangerousRmRfPatterns(t *testing.T) {
	dangerousCommands := []string{
		"rm -rf /",
		"rm -rf ~",
		"rm -r /",
	}

	svc := NewGuardianService(nil)
	svc.SetEnabled(true)

	for _, cmd := range dangerousCommands {
		review, err := svc.Review(nil, "execute_command", `{"command": "`+cmd+`"}`, "")
		assert.NoError(t, err)
		assert.Equal(t, VerdictDeny, review.Verdict, "command: %s should be denied", cmd)
	}
}
