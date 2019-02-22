package watcher

import (
	"gopkg.in/fsnotify.v1"
	"io/ioutil"
	"path/filepath"
)

func ConfigMapChanges(file string) (<-chan []byte, <-chan error, <-chan struct{}, error) {
	errCh := make(chan error)
	fCh := make(chan []byte)
	done := make(chan struct{})

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, nil, err
	}

	if err = w.Add(file); err != nil {
		return nil, nil, nil, err
	}

	curPath, changed, err := BackingSymlinkChange(file, "")
	if err != nil {
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
				if event.Op == fsnotify.Remove {
					// remove the watcher since the file is removed
					w.Remove(event.Name)
					// add a new watcher pointing to the new symlink/file
					if err = w.Add(event.Name); err != nil {
						errCh <- err
						break
					}
				}

				curPath, changed, err = BackingSymlinkChange(file, curPath)
				if changed {
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

func BackingSymlinkChange(file string, lastPath string) (string, bool, error) {
	resolved, err := filepath.EvalSymlinks(file)
	return resolved, lastPath == resolved, err
}
