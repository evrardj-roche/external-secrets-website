package main

import "testing"

func Test_convertClientGoToRealK8sVersion(t *testing.T) {
	type args struct {
		clientGoVersion string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Incomplete version",
			args: args{"v0.35"},
			want: "v1.35",
		},
		{
			name: "Complete version",
			args: args{"v0.35.0"},
			want: "v1.35",
		},
		{
			name: "Complete version no v",
			args: args{"0.35.0"},
			want: "v1.35",
		},
		{
			name: "Incomplete version no v",
			args: args{"0.35"},
			want: "v1.35",
		},
		{
			name: "Incorrect data",
			args: args{"qwqwwq"},
			want: "qwqwwq",
		},
		{
			name: "Incomplete version no v above 1",
			args: args{"1.35"},
			want: "1.35",
		},
		{
			name: "Incomplete version with v above 1",
			args: args{"v1.35"},
			want: "v1.35",
		},
		{
			name: "Complete version with v above 1",
			args: args{"v1.35.0"},
			want: "v1.35.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convertClientGoToRealK8sVersion(tt.args.clientGoVersion); got != tt.want {
				t.Errorf("convertClientGoToRealK8sVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractMajorMinor(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"v0.15.3", "v0.15"},
		{"v1.2.10", "v1.2"},
		{"v0.9.0", "v0.9"},
		{"v10.20.30", "v10.20"},
	}

	for _, tt := range tests {
		result := extractMajorMinor(tt.input)
		if result != tt.expected {
			t.Errorf("extractMajorMinor(%s) = %s; want %s", tt.input, result, tt.expected)
		}
	}
}

func TestIsDirectoryUsedByOtherRelease(t *testing.T) {
	tests := []struct {
		name         string
		majorMinor   string
		tagToRemove  string
		versions     []Version
		expectedUsed bool
	}{
		{
			name:         "directory still used by v0.15.1",
			majorMinor:   "v0.15",
			tagToRemove:  "v0.15.3",
			versions:     []Version{{Tag: "v0.15.3"}, {Tag: "v0.15.1"}, {Tag: "v0.14.2"}},
			expectedUsed: true,
		},
		{
			name:         "directory not used by any other",
			majorMinor:   "v0.14",
			tagToRemove:  "v0.14.2",
			versions:     []Version{{Tag: "v0.15.3"}, {Tag: "v0.15.1"}, {Tag: "v0.14.2"}},
			expectedUsed: false,
		},
		{
			name:         "only one version for major.minor",
			majorMinor:   "v0.16",
			tagToRemove:  "v0.16.0",
			versions:     []Version{{Tag: "v0.16.0"}, {Tag: "v0.15.1"}},
			expectedUsed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDirectoryUsedByOtherRelease(tt.majorMinor, tt.tagToRemove, tt.versions)
			if result != tt.expectedUsed {
				t.Errorf("isDirectoryUsedByOtherRelease() = %v; want %v", result, tt.expectedUsed)
			}
		})
	}
}
