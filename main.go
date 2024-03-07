package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/fsnotify/fsnotify"
)

func newWatch(ctx context.Context, config string) (context.CancelFunc, <-chan struct{}, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}

	if err := watcher.Add(filepath.Dir(filepath.Clean(config))); err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer func() { done <- struct{}{} }()

		for {
			select {
			case <-ctx.Done():
				if err := watcher.Close(); err != nil {
					log.Printf("error closing watcher: %v", err)
				}
			case e, ok := <-watcher.Events:
				if !ok {
					log.Printf("watch ended")
					return
				}

				log.Printf("event received|\t%s", e.String())
			case err, ok := <-watcher.Errors:
				if !ok {
					log.Printf("watch ended")
					return
				}
				log.Printf("watcher error: %v", err)
			}
		}
	}()

	return cancel, done, nil
}

const data = "hello world"

func writeFile(path string) error {
	return os.WriteFile(path, []byte(data), os.ModePerm)
}

func atomicWrite(path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "temp")
	if err != nil {
		return err
	}

	if err := tmp.Chmod(stat.Mode()); err != nil {
		return err
	}

	if _, err := tmp.WriteString(data); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), path)
}

func main() {
	config := flag.String("config", "config.txt", "config file to watch")
	atomic := flag.Bool("atomic", false, "write to a temp file then atomically os.Rename instead of directly writing")

	flag.Parse()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT)

	cancel, done, err := newWatch(context.Background(), *config)
	if err != nil {
		log.Fatal(err)
	}

	switch {
	case *atomic:
		err = atomicWrite(*config)
	default:
		err = writeFile(*config)
	}

	if err != nil {
		log.Printf("error writing config: %v", err)
		cancel()
		<-done
		return
	}

	s := <-sig
	log.Printf("recevied signal %v, shutting down ...", s)
	cancel()

	<-done
	log.Printf("done.")
}
