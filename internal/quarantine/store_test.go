package quarantine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHoldRestoreRoundTrip(t *testing.T) {
	work := t.TempDir()
	victim := filepath.Join(work, "eicar.com")
	if err := os.WriteFile(victim, []byte("X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(filepath.Join(work, "q"))
	if err != nil {
		t.Fatal(err)
	}

	sc, err := store.Hold(victim, "Win.Test.EICAR_HDB-1", 42)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(victim); !os.IsNotExist(err) {
		t.Fatal("original file should be gone after Hold")
	}
	if sc.SHA256 == "" || sc.Size == 0 || sc.OrigMode == 0 || sc.ScanID != 42 {
		t.Fatalf("sidecar incomplete: %+v", sc)
	}
	if _, err := os.Stat(store.sidecarPath(sc.Name)); err != nil {
		t.Fatalf("sidecar file missing: %v", err)
	}

	// The sidecar on disk carries the scan id.
	loaded, err := store.SidecarFor(sc.Name)
	if err != nil || loaded.ScanID != 42 {
		t.Fatalf("SidecarFor = %+v, %v", loaded, err)
	}

	if err := store.Restore(sc); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("restore did not put the file back: %v", err)
	}
	if string(got) != "X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR" {
		t.Errorf("restored content mismatch: %q", got)
	}
	if _, err := os.Stat(store.sidecarPath(sc.Name)); !os.IsNotExist(err) {
		t.Error("sidecar should be removed after restore")
	}
}

func TestRestoreRefusesToClobber(t *testing.T) {
	work := t.TempDir()
	victim := filepath.Join(work, "f")
	os.WriteFile(victim, []byte("bad"), 0o644)
	store, _ := NewStore(filepath.Join(work, "q"))

	sc, err := store.Hold(victim, "sig", 0)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(victim, []byte("something new"), 0o644) // path reused

	if err := store.Restore(sc); err != ErrOriginExists {
		t.Fatalf("err = %v, want ErrOriginExists", err)
	}
}

func TestRestoreRefusesMissingParent(t *testing.T) {
	work := t.TempDir()
	sub := filepath.Join(work, "sub")
	os.Mkdir(sub, 0o755)
	victim := filepath.Join(sub, "f")
	os.WriteFile(victim, []byte("bad"), 0o644)
	store, _ := NewStore(filepath.Join(work, "q"))

	sc, err := store.Hold(victim, "sig", 0)
	if err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(sub) // parent dir vanished

	if err := store.Restore(sc); err != ErrOriginParentGone {
		t.Fatalf("err = %v, want ErrOriginParentGone", err)
	}
}

func TestPurgeRemovesBlobAndSidecar(t *testing.T) {
	work := t.TempDir()
	victim := filepath.Join(work, "f")
	os.WriteFile(victim, []byte("payload payload payload"), 0o644)
	store, _ := NewStore(filepath.Join(work, "q"))

	sc, err := store.Hold(victim, "sig", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Purge(sc.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, "q", sc.Name)); !os.IsNotExist(err) {
		t.Error("blob still present after Purge")
	}
	if _, err := os.Stat(store.sidecarPath(sc.Name)); !os.IsNotExist(err) {
		t.Error("sidecar still present after Purge")
	}
	if err := store.Purge(sc.Name); err != nil {
		t.Errorf("Purge should be idempotent, got %v", err)
	}
}

func TestHoldRejectsNonRegular(t *testing.T) {
	work := t.TempDir()
	store, _ := NewStore(filepath.Join(work, "q"))
	if _, err := store.Hold(work, "sig", 0); err == nil {
		t.Fatal("expected error holding a directory")
	}
}
