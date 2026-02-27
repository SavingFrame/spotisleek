package config

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_NoFileNoEnv_ReturnsZeroValues(t *testing.T) {
	resetViper(t)
	withWorkingDir(t, t.TempDir())

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, Config{}, cfg)
}

func TestLoadConfig_LoadsFromEnvironment(t *testing.T) {
	resetViper(t)
	withWorkingDir(t, t.TempDir())

	t.Setenv("NAVIDROME_URL", "http://localhost:4533")
	t.Setenv("NAVIDROME_USERNAME", "alice")
	t.Setenv("NAVIDROME_PASSWORD", "secret")
	t.Setenv("SLSKD_URL", "http://localhost:5030")
	t.Setenv("SLSKD_API_KEY", "api-key")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:4533", cfg.NAVIDROME_URL)
	assert.Equal(t, "alice", cfg.NAVIDROME_USERNAME)
	assert.Equal(t, "secret", cfg.NAVIDROME_PASSWORD)
	assert.Equal(t, "http://localhost:5030", cfg.SLSKD_URL)
	assert.Equal(t, "api-key", cfg.SLSKD_API_KEY)
}

func TestLoadConfig_LoadsFromDotEnvFile(t *testing.T) {
	resetViper(t)
	tmpDir := t.TempDir()
	withWorkingDir(t, tmpDir)

	err := os.WriteFile(tmpDir+"/.env", []byte("NAVIDROME_URL=http://from-file\nSLSKD_API_KEY=file-key\n"), 0o600)
	require.NoError(t, err)

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, "http://from-file", cfg.NAVIDROME_URL)
	assert.Equal(t, "file-key", cfg.SLSKD_API_KEY)
}

func resetViper(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()

	oldWd, err := os.Getwd()
	require.NoError(t, err)

	err = os.Chdir(dir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := os.Chdir(oldWd)
		require.NoError(t, err)
	})
}
