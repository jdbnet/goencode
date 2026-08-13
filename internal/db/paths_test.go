package db

import "testing"

func TestPathPrefixFilterEscapesLike(t *testing.T) {
	exact, like := pathPrefixFilter("/media/foo_bar")
	if exact != "/media/foo_bar" {
		t.Fatalf("exact = %q", exact)
	}
	if like != `/media/foo\_bar/%` {
		t.Fatalf("like = %q, want escaped underscore", like)
	}

	_, like = pathPrefixFilter(`/media/100%`)
	if like != `/media/100\%/%` {
		t.Fatalf("like = %q, want escaped percent", like)
	}
}

func TestEscapeLikeBackslash(t *testing.T) {
	got := escapeLike(`a\b%c_d`)
	want := `a\\b\%c\_d`
	if got != want {
		t.Fatalf("escapeLike = %q, want %q", got, want)
	}
}
