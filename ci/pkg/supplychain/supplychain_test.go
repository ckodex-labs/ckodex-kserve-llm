package supplychain

import "testing"

func TestCosignIdentityEnv(t *testing.T) {
	t.Setenv(sigstoreIDTokenEnv, "")
	t.Setenv(githubOIDCTokenEnv, "")
	t.Setenv(githubOIDCRequestURLEnv, "")

	mode, env := cosignIdentityEnv()
	if mode != "ambient" {
		t.Fatalf("expected ambient mode, got %q", mode)
	}
	if env != nil {
		t.Fatalf("expected nil env for ambient mode, got %#v", env)
	}

	t.Setenv(githubOIDCTokenEnv, "gha-token")
	t.Setenv(githubOIDCRequestURLEnv, "https://token.actions.example")
	mode, env = cosignIdentityEnv()
	if mode != "github-actions-oidc" {
		t.Fatalf("expected github-actions-oidc mode, got %q", mode)
	}
	if env[githubOIDCTokenEnv] != "gha-token" || env[githubOIDCRequestURLEnv] != "https://token.actions.example" {
		t.Fatalf("unexpected github oidc env: %#v", env)
	}

	t.Setenv(sigstoreIDTokenEnv, "sigstore-token")
	mode, env = cosignIdentityEnv()
	if mode != sigstoreIDTokenEnv {
		t.Fatalf("expected %q mode, got %q", sigstoreIDTokenEnv, mode)
	}
	if env[sigstoreIDTokenEnv] != "sigstore-token" {
		t.Fatalf("unexpected sigstore env: %#v", env)
	}
}
