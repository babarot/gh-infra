package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// A GitHub App installation token reports permissions.admin as false even
// with the Administration permission, so what GitHub leaves out of a
// repository that is not public is told from a token without access by
// whether the token can read what only an admin can.
func TestFetchSecretScanning_AppToken(t *testing.T) {
	spec := &manifest.Repository{Spec: manifest.RepositorySpec{Security: &manifest.Security{SecretScanning: manifest.Ptr(false)}}}
	for _, tt := range []struct {
		name    string
		state   func(*fakeState)
		wantNil bool   // not offered
		wantErr string // not readable
		want    bool   // secret scanning, when read
	}{
		{name: "not offered to an installation token", wantNil: true,
			state: func(s *fakeState) { s.Repo.Visibility, s.NoSecretScanning, s.AppToken = "private", true, true }},
		{name: "offered to an installation token", want: true,
			state: func(s *fakeState) { s.Repo.Visibility, s.AppToken = "private", true }},
		{name: "public, left out for a token without access", wantErr: "only to repository admins",
			state: func(s *fakeState) { s.AppToken, s.NotAdmin = true, true }},
		{name: "not offered to a user token", wantNil: true,
			state: func(s *fakeState) { s.Repo.Visibility, s.NoSecretScanning = "private", true }},
		{name: "left out for a token without access", wantErr: "only to repository admins",
			state: func(s *fakeState) { s.Repo.Visibility, s.NoSecretScanning, s.NotAdmin = "private", true, true }},
		{name: "left out for an installation token without access", wantErr: "only to repository admins",
			state: func(s *fakeState) {
				s.Repo.Visibility, s.NoSecretScanning, s.AppToken, s.NotAdmin = "private", true, true, true
			}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := propEnvState()
			tt.state(&s)
			current, err := NewProcessor(newFakeGitHub("o", "r", s), nil).FetchRepository(context.Background(), "o", "r", spec, nil)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := current.Security.SecretScanning
			switch {
			case tt.wantNil && got != nil:
				t.Errorf("secret scanning = %v, want not offered (nil)", *got)
			case !tt.wantNil && (got == nil || *got != tt.want):
				t.Errorf("secret scanning = %v, want %v", got, tt.want)
			}
		})
	}
}
