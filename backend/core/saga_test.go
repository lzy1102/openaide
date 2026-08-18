package kernel

import (
	"errors"
	"testing"
)

func TestRunSaga_AllSuccess(t *testing.T) {
	order := []string{}
	err := RunSaga([]SagaStep{
		{
			Name:       "step1",
			Execute:    func() error { order = append(order, "1"); return nil },
			Compensate: func() error { order = append(order, "undo1"); return nil },
		},
		{
			Name:       "step2",
			Execute:    func() error { order = append(order, "2"); return nil },
			Compensate: func() error { order = append(order, "undo2"); return nil },
		},
	})
	if err != nil {
		t.Fatal("expected success:", err)
	}
	if len(order) != 2 || order[0] != "1" || order[1] != "2" {
		t.Errorf("expected [1 2], got %v", order)
	}
}

func TestRunSaga_Rollback(t *testing.T) {
	order := []string{}
	err := RunSaga([]SagaStep{
		{
			Name:       "step1",
			Execute:    func() error { order = append(order, "1"); return nil },
			Compensate: func() error { order = append(order, "undo1"); return nil },
		},
		{
			Name:       "step2",
			Execute:    func() error { return errors.New("fail") },
			Compensate: func() error { order = append(order, "undo2"); return nil },
		},
		{
			Name:       "step3",
			Execute:    func() error { order = append(order, "3"); return nil },
			Compensate: func() error { order = append(order, "undo3"); return nil },
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// step1 completed, step2 failed → compensate step1 only
	if len(order) != 2 || order[0] != "1" || order[1] != "undo1" {
		t.Errorf("expected [1 undo1], got %v", order)
	}
}

func TestRunSaga_CompensationFailure(t *testing.T) {
	// Compensation failure should not panic
	err := RunSaga([]SagaStep{
		{
			Name:       "step1",
			Execute:    func() error { return nil },
			Compensate: func() error { return errors.New("compensation failed") },
		},
		{
			Name:       "step2",
			Execute:    func() error { return errors.New("main failure") },
			Compensate: nil,
		},
	})
	if err == nil || err.Error() != "main failure" {
		t.Errorf("expected 'main failure', got %v", err)
	}
}
