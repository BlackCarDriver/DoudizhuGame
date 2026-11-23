package main

import "testing"

func TestPickIntPtrAllowsZero(t *testing.T) {
	zero := 0
	if got := pickIntPtr(&zero, "", 100); got != 0 {
		t.Fatalf("文件里的 0 应生效，得到 %d", got)
	}
	if got := pickIntPtr(nil, "", 100); got != 100 {
		t.Fatalf("缺省应使用默认值 100，得到 %d", got)
	}
}
