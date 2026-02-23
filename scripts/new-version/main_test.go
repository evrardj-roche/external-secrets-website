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
