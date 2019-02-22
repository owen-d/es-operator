package watcher

import (
	"gopkg.in/fsnotify.v1"
	"io/ioutil"
	"path/filepath"
)

func DirOfSymlink(file string) (string, error) {
	resolved, err := filepath.EvalSymlinks(file)
	if err != nil {
		return "", err
	}
	return filepath.Dir(resolved), nil
}

// ConfigMapChanges targets the parent dir of a wanted file (configmaps are generally mounted as a dir unless subPath is specified)
// fsnotify plays nicely with watching a parent dir -- this will only work with 1 level nested configmaps that are mounted as a dir
func ConfigMapChanges(file string) (<-chan []byte, <-chan error, <-chan struct{}, error) {
	errCh := make(chan error)
	fCh := make(chan []byte)
	done := make(chan struct{})

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, nil, err
	}

	watchDir, err := DirOfSymlink(file)
	if err != nil {
		return nil, nil, nil, err
	}

	if err = w.Add(watchDir); err != nil {
		return nil, nil, nil, err
	}

	go func() {
		var err error
		defer w.Close()

		for {
			select {
			case err = <-w.Errors:
				errCh <- err

			case event := <-w.Events:
				// backing file are removed after updates
				if event.Op == fsnotify.Remove {
					// remove the watcher since the file is removed
					w.Remove(watchDir)

					watchDir, err = DirOfSymlink(file)
					if err != nil {
						errCh <- err
						break
					}

					// add a new watcher pointing to the new symlink dir
					if err = w.Add(watchDir); err != nil {
						errCh <- err
						break
					}

					b, err := ioutil.ReadFile(file)
					if err != nil {
						errCh <- err
					}
					fCh <- b

				}

			}
		}
		done <- struct{}{}

	}()
	return fCh, errCh, done, nil
}
