package transcript

import "testing"

func TestNormalizeChinese(t *testing.T) {
	got := NormalizeChinese("  你  好， WORLD！ ")
	if got != "你好, world!" {
		t.Fatalf("unexpected normalized text %q", got)
	}
}

func TestReconcileOverlapIsStable(t *testing.T) {
	got := ReconcileOverlap("你好世界", "世界很好")
	if got != "你好世界很好" {
		t.Fatalf("unexpected reconciliation %q", got)
	}
	if repeated := ReconcileOverlap(got, "世界很好"); repeated != got {
		t.Fatalf("reconciliation is not idempotent: %q", repeated)
	}
	if got := ReconcileOverlap("hello world", "world again"); got != "hello world again" {
		t.Fatalf("unexpected Latin reconciliation %q", got)
	}
}

func TestMerge(t *testing.T) {
	if got := Merge("你好", "你好", "世界"); got != "你好世界" {
		t.Fatalf("unexpected merged text %q", got)
	}
}
