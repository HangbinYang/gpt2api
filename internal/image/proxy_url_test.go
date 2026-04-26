package image

import "testing"

func TestWithPublicBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		rawURL     string
		apiBaseURL string
		want       string
	}{
		{
			name:       "empty base keeps relative path",
			rawURL:     "/p/img/task/0?exp=1&sig=abc",
			apiBaseURL: "",
			want:       "/p/img/task/0?exp=1&sig=abc",
		},
		{
			name:       "absolute image url is preserved",
			rawURL:     "https://cdn.example.com/p/img/task/0?exp=1&sig=abc",
			apiBaseURL: "https://api.example.com/v1",
			want:       "https://cdn.example.com/p/img/task/0?exp=1&sig=abc",
		},
		{
			name:       "extract origin from v1 api base",
			rawURL:     "/p/img/task/0?exp=1&sig=abc",
			apiBaseURL: "https://api.example.com/v1",
			want:       "https://api.example.com/p/img/task/0?exp=1&sig=abc",
		},
		{
			name:       "extract origin from nested path api base",
			rawURL:     "/p/img/task/0?exp=1&sig=abc",
			apiBaseURL: "https://api.example.com/gateway/v1?foo=bar",
			want:       "https://api.example.com/p/img/task/0?exp=1&sig=abc",
		},
		{
			name:       "invalid base keeps relative path",
			rawURL:     "/p/img/task/0?exp=1&sig=abc",
			apiBaseURL: "://bad",
			want:       "/p/img/task/0?exp=1&sig=abc",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := WithPublicBaseURL(tt.rawURL, tt.apiBaseURL); got != tt.want {
				t.Fatalf("WithPublicBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
