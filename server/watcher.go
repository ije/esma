// The watcher implement file was stolen from https://github.com/evanw/esbuild/blob/master/pkg/api/api_impl.go#L1087
package server

import (
	"math/rand"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// The maximum number of recently-edited items to check every interval
const maxRecentItemCount = 16

// The minimum number of non-recent items to check every interval
const minItemCountPerIter = 64

// The maximum number of intervals before a change is detected
const maxIntervalsBeforeUpdate = 20

type watcher struct {
	app               *App
	interval          time.Duration
	shouldStop        int32
	recentItems       []string
	itemsToScan       []string
	itemsPerIteration int
}

func (w *watcher) setWatchData() {
	w.itemsToScan = w.itemsToScan[:0] // Reuse memory

	// Remove any recent items that weren't a part of the latest build
	end := 0
	for _, path := range w.recentItems {
		w.app.lock.RLock()
		_, ok := w.app.builds[path]
		w.app.lock.RUnlock()
		if ok {
			w.recentItems[end] = path
			end++
		}
	}
	w.recentItems = w.recentItems[:end]
}

func (w *watcher) start(onChange func(path string, exists bool)) {
	go func() {
		for atomic.LoadInt32(&w.shouldStop) == 0 {
			// Sleep for the watch interval
			time.Sleep(w.interval)

			// Rebuild if we're dirty
			filename, modtime := w.tryToFindDirtyPath()
			if filename != "" {
				exists := !modtime.IsZero()
				onChange(filename, exists)
				w.setWatchData()
			}
		}
	}()
}

func (w *watcher) stop() {
	atomic.StoreInt32(&w.shouldStop, 1)
}

func (w *watcher) tryToFindDirtyPath() (string, time.Time) {
	// If we ran out of items to scan, fill the items back up in a random order
	if len(w.itemsToScan) == 0 {
		items := w.itemsToScan[:0] // Reuse memory
		w.app.lock.RLock()
		for path := range w.app.builds {
			if !strings.HasPrefix(path, "/builtin:") {
				items = append(items, path)
			}
		}
		w.app.lock.RUnlock()
		rand.Seed(time.Now().UnixNano())
		for i := int32(len(items) - 1); i > 0; i-- { // Fisher–Yates shuffle
			j := rand.Int31n(i + 1)
			items[i], items[j] = items[j], items[i]
		}
		w.itemsToScan = items

		// Determine how many items to check every iteration, rounded up
		perIter := (len(items) + maxIntervalsBeforeUpdate - 1) / maxIntervalsBeforeUpdate
		if perIter < minItemCountPerIter {
			perIter = minItemCountPerIter
		}
		w.itemsPerIteration = perIter
	}

	// Always check all recent items every iteration
	for i, path := range w.recentItems {
		ok, modtime := w.checkModtime(path)
		if ok {
			copy(w.recentItems[i:], w.recentItems[i+1:])
			w.recentItems[len(w.recentItems)-1] = path
			return path, modtime
		}
	}

	// Check a constant number of items every iteration
	remainingCount := len(w.itemsToScan) - w.itemsPerIteration
	if remainingCount < 0 {
		remainingCount = 0
	}
	toCheck, remaining := w.itemsToScan[remainingCount:], w.itemsToScan[:remainingCount]
	w.itemsToScan = remaining

	// Check if any of the entries in this iteration have been modified
	for _, path := range toCheck {
		ok, modtime := w.checkModtime(path)
		if ok {
			w.recentItems = append(w.recentItems, path)
			if len(w.recentItems) > maxRecentItemCount {
				// Remove items from the front of the list when we hit the limit
				copy(w.recentItems, w.recentItems[1:])
				w.recentItems = w.recentItems[:maxRecentItemCount]
			}
			return path, modtime
		}
	}

	return "", time.Time{}
}

func (w *watcher) checkModtime(path string) (bool, time.Time) {
	w.app.lock.RLock()
	build, ok := w.app.builds[path]
	w.app.lock.RUnlock()
	if ok {
		fi, err := os.Lstat(path)
		if err != nil && os.IsNotExist(err) {
			return true, time.Time{}
		} else if err == nil {
			modtime := fi.ModTime()
			if !modtime.Equal(build.Modtime) {
				return true, modtime
			}
		}
	}
	return false, time.Time{}
}
