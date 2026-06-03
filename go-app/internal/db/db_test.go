package db

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildDSN_Table: table-driven AAA style using require.
func TestBuildDSN_Table(t *testing.T) {
	t.Parallel()

	type args struct {
		user, pass, host, port, dbname, sslmode string
	}
	testCases := []struct {
		name string
		in   args
		want string
	}{
		{ //nolint:gosec // G101: test placeholder values only
			name: "basic dsn",
			in:   args{"u", "p", "h", "5432", "d", "disable"},
			want: "postgres://u:p@h:5432/d?sslmode=disable",
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			a := tc.in
			// Act
			got := BuildDSN(a.user, a.pass, a.host, a.port, a.dbname, a.sslmode)
			// Assert
			require.Equal(t, tc.want, got)
		})
	}
}

// TestGetenv_Table: table-driven tests for getenv behavior.
func TestGetenv_Table(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		key        string
		setEnv     bool
		envValue   string
		defaultVal string
		want       string
	}{
		{
			name:       "unset -> default",
			key:        "NON_EXISTENT_ENV_XYZ",
			setEnv:     false,
			envValue:   "",
			defaultVal: "def",
			want:       "def",
		},
		{name: "set -> value", key: "FOO_BAR", setEnv: true, envValue: "hello", defaultVal: "def", want: "hello"},
		{name: "empty -> default", key: "BAZ_QUX", setEnv: true, envValue: "", defaultVal: "zzz", want: "zzz"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			if tc.setEnv {
				require.NoError(t, os.Setenv(tc.key, tc.envValue))
				t.Cleanup(func() { require.NoError(t, os.Unsetenv(tc.key)) })
			} else {
				require.NoError(t, os.Unsetenv(tc.key))
			}
			// Act
			got := getenv(tc.key, tc.defaultVal)
			// Assert
			require.Equal(t, tc.want, got)
		})
	}
}
