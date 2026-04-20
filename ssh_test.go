package main

import "testing"

func TestIsRemoteUploadCommand(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{line: "rz", want: true},
		{line: "rz /tmp/file.txt", want: true},
		{line: " sz", want: false},
		{line: "echo rz", want: false},
	}

	for _, tt := range tests {
		if got := isRemoteUploadCommand(tt.line); got != tt.want {
			t.Fatalf("isRemoteUploadCommand(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}
