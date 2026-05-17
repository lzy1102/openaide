package kernel

import (
	"testing"
)

func TestAdaptiveRounds_Simple(t *testing.T) {
	ar := NewAdaptiveRounds(5, 30)
	r := ar.Calculate("hello", 0)
	if r != 5 {
		t.Errorf("simple query should use min rounds, got %d", r)
	}
}

func TestAdaptiveRounds_Complex(t *testing.T) {
	ar := NewAdaptiveRounds(5, 30)
	r := ar.Calculate("分析所有代码然后修复每个bug并且重构全部模块", 15)
	if r < 10 {
		t.Errorf("complex query should use more rounds, got %d", r)
	}
}

func TestAdaptiveRounds_MaxCap(t *testing.T) {
	ar := NewAdaptiveRounds(5, 30)
	veryLong := ""
	for i := 0; i < 600; i++ {
		veryLong += "分析"
	}
	r := ar.Calculate(veryLong, 20)
	if r > 30 {
		t.Errorf("should cap at 30, got %d", r)
	}
}

func TestAdaptiveRounds_LongQuery(t *testing.T) {
	ar := NewAdaptiveRounds(5, 30)
	long := ""
	for i := 0; i < 300; i++ {
		long += "修复 "
	}
	r := ar.Calculate(long, 2)
	if r < 10 {
		t.Errorf("long query should increase rounds, got %d", r)
	}
}
