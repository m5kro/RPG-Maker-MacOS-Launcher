// benchmark-mz runs only user-supplied, trusted games; it is not a sandbox.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed benchmark.js workload.js
var assets embed.FS

func copyGame(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.Mkdir(target, 0700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file: %s", path)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		return errors.Join(copyErr, out.Close())
	})
}

func runtimeBinary(path, platform string) (string, error) {
	if platform == "darwin" {
		path = filepath.Join(path, "Contents", "MacOS", "nwjs")
	} else if platform != "windows" {
		return "", fmt.Errorf("unsupported platform: %s", platform)
	}
	binary, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(binary)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || (platform == "darwin" && info.Mode().Perm()&0111 == 0) ||
		(platform == "windows" && !strings.EqualFold(filepath.Ext(binary), ".exe")) {
		return "", errors.New("NW.js executable missing or not executable")
	}
	return binary, nil
}

func checkOutputOutsideGame(source, output string) error {
	// Windows games and results may live on different drives.
	if !strings.EqualFold(filepath.VolumeName(source), filepath.VolumeName(output)) {
		return nil
	}
	rel, err := filepath.Rel(source, output)
	if err != nil {
		return err
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
		return errors.New("output must be outside game")
	}
	return nil
}

func run(args []string) error {
	f := flag.NewFlagSet("benchmark-mz", flag.ContinueOnError)
	game := f.String("game", "", "MZ game directory")
	runtimePath := f.String("runtime", "", "path to nwjs.app (macOS) or NW.js .exe (Windows)")
	root := f.String("root", "", "new output directory")
	warm := f.Int("warmup", 10, "warmup seconds")
	duration := f.Int("duration", 30, "measurement seconds")
	fixed := f.Bool("fixed", false, "run the fixed synthetic map workload")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *game == "" || *runtimePath == "" || *root == "" || *warm < 0 || *warm > 3600 || *duration < 1 || *duration > 3600 {
		return errors.New("require --game DIR --runtime NWJS_PATH --root NEW_DIR; warmup 0..3600, duration 1..3600")
	}
	source, err := filepath.EvalSymlinks(*game)
	if err != nil {
		return err
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return err
	}
	output, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return err
	}
	output = filepath.Join(parent, filepath.Base(output))
	if err := checkOutputOutsideGame(source, output); err != nil {
		return err
	}
	binary, err := runtimeBinary(*runtimePath, runtime.GOOS)
	if err != nil {
		return err
	}
	for _, p := range []string{"index.html", "js/rmmz_core.js", "js/main.js"} {
		info, err := os.Stat(filepath.Join(source, p))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("not a file: %s", p)
		}
	}
	index, err := os.ReadFile(filepath.Join(source, "index.html"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(index), "</body>") {
		return errors.New("index.html must contain </body>")
	}
	if err = os.Mkdir(output, 0700); err != nil {
		return err
	}
	copied := filepath.Join(output, "game")
	if err = copyGame(source, copied); err != nil {
		return err
	}
	if *fixed {
		workload, err := assets.ReadFile("workload.js")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(copied, "js", "plugins"), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(copied, "js", "plugins", "BenchmarkWorkload.js"), workload, 0600); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(copied, "js", "plugins.js"), []byte(`var $plugins = [{"name":"BenchmarkWorkload","status":true,"description":"","parameters":{}}];`), 0600); err != nil {
			return err
		}
	}
	script, err := assets.ReadFile("benchmark.js")
	if err != nil {
		return err
	}
	config, _ := json.Marshal(map[string]any{"warmup": *warm, "duration": *duration, "result": filepath.Join(output, "result.json"), "fixed": *fixed})
	injected := "<script>window.__mzBenchmark = " + string(config) + ";</script><script src=\"benchmark-mz.js\"></script></body>"
	if err = os.WriteFile(filepath.Join(copied, "index.html"), []byte(strings.Replace(string(index), "</body>", injected, 1)), 0600); err != nil {
		return err
	}
	scriptFile, err := os.OpenFile(filepath.Join(copied, "benchmark-mz.js"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := scriptFile.Write(script)
	if err = errors.Join(writeErr, scriptFile.Close()); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Waiting for a focused, started game scene; bring the game window to the front if needed.")
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*warm+*duration+120)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--user-data-dir="+filepath.Join(output, "profile"), copied)
	cmd.Dir = copied
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("NW.js failed: %w", err)
	}
	if _, err = os.Stat(filepath.Join(output, "result.json")); err != nil {
		return fmt.Errorf("no benchmark result: %w", err)
	}
	fmt.Println(filepath.Join(output, "result.json"))
	return nil
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
