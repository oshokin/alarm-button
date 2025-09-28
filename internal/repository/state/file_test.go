package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domain "github.com/oshokin/alarm-button/internal/domain/alarm"
)

// TestFileRepository_NotFound verifies Load returns ErrNotFound for missing file.
func TestFileRepository_NotFound(t *testing.T) {
	t.Parallel()
	repo := NewFileRepository(filepath.Join(t.TempDir(), "missing.json"))
	s, err := repo.Load(context.Background())
	require.ErrorIs(t, err, ErrNotFound)
	require.Nil(t, s)
}

// TestFileRepository_SaveLoad_Roundtrip ensures Save followed by Load returns equal state.
func TestFileRepository_SaveLoad_Roundtrip(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "state.json")
	repo := NewFileRepository(file)

	ts := time.Now().UTC().Truncate(time.Second)
	want := &domain.State{
		Timestamp: ts,
		LastActor: &domain.Actor{
			Hostname: "Oleg Shokin",
			Username: "o.shokin",
		},
		IsEnabled: true,
	}

	require.NoError(t, repo.Save(context.Background(), want))

	got, err := repo.Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, want.IsEnabled, got.IsEnabled)
	require.Equal(t, want.Timestamp.Unix(), got.Timestamp.Unix())
	require.Equal(t, want.LastActor, got.LastActor)

	_, err = os.Stat(file)
	require.NoError(t, err)
}
