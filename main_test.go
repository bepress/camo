package main

import (
	"testing"
)

func TestHelloWorld(t *testing.T) {
	got := Hello()
	if got != "vim-go" {
		t.Errorf("got %q but wanted %q", got, "vim-go")
	}
}
