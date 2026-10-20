package launcher

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// tensorBoardEnv names the shell command that starts TensorBoard, for
// example "tensorboard" or "conda run -n tb tensorboard". The test is
// skipped without it; CI does not install TensorBoard.
const tensorBoardEnv = "ROTARI_LAUNCHER_TEST_TENSORBOARD"

// TestTensorBoardBehindPrefix launches a real TensorBoard as a registered
// command would and checks that its listener passes VerifyListener and that
// it serves its data under the allocated prefix only. An empty log directory
// is enough: the runs list is then "[]".
func TestTensorBoardBehindPrefix(t *testing.T) {
	tensorBoard := os.Getenv(tensorBoardEnv)
	if tensorBoard == "" {
		t.Skip(tensorBoardEnv + " is not set")
	}
	lifetime := newTestLifetime(t)
	port := allocate(t)
	prefix := "/b/main/api/launches/3f9a1c/proxy"
	spec := testSpec(tensorBoard+` --logdir "$ROTARI_LAUNCHER_ARTIFACT" --host 127.0.0.1 --port "$ROTARI_LAUNCHER_PORT" --path_prefix "$ROTARI_LAUNCHER_BASE_PATH"`, outputFile(t))
	spec.Env = append(spec.Env,
		"ROTARI_LAUNCHER_ARTIFACT="+filepath.Join(t.TempDir(), "logs with space"),
		"ROTARI_LAUNCHER_PORT="+strconv.Itoa(port),
		"ROTARI_LAUNCHER_BASE_PATH="+prefix,
		"NO_PROXY=127.0.0.1,localhost,::1", "no_proxy=127.0.0.1,localhost,::1")
	process := startTest(t, lifetime, spec)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
	deadline := time.Now().Add(60 * time.Second)
	for {
		if err := VerifyListener(port, process.PGID()); err == nil {
			break
		}
		select {
		case <-process.Done():
			output, _ := os.ReadFile(spec.Output.(*os.File).Name())
			t.Fatalf("TensorBoard exited before listening:\n%s", output)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("TensorBoard did not listen within 60s")
		}
		time.Sleep(200 * time.Millisecond)
	}
	get := func(path string) (int, string) {
		response, err := client.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(body)
	}
	if code, body := get(prefix + "/data/runs"); code != http.StatusOK || strings.TrimSpace(body) != "[]" {
		t.Fatalf("GET prefix/data/runs = %d %q", code, body)
	}
	if code, _ := get("/data/runs"); code != http.StatusNotFound {
		t.Fatalf("GET /data/runs outside the prefix = %d, want 404", code)
	}
}
