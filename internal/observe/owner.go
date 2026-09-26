// Owner lock: exactly one worker daemon may drive playback. A second
// instance exits instead of creating competing controllers.
package observe

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/comcreate-io/context.fm/internal/queue"
)

// Lock is a held exclusive file lock.
type Lock struct{ f *os.File }

// AcquireOwner tries to become the playback owner. An error means another
// daemon holds it (or the lock dir failed); the caller must not drive
// playback.
func AcquireOwner() (*Lock, error) {
	if err := os.MkdirAll(queue.DataDir(), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(queue.DataDir(), "owner.lock"),
		os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("observe: playback already owned: %w", err)
	}
	return &Lock{f: f}, nil
}

// Release frees ownership.
func (l *Lock) Release() error { return l.f.Close() }
