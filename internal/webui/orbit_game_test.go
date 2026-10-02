package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOrbitGameInteraction(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	htmlPath := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(htmlPath, []byte(composeWebHTML(nil, false, "")), 0o600); err != nil {
		t.Fatal(err)
	}

	script := `
const fs = require('fs');
const { JSDOM } = require('jsdom');
const html = fs.readFileSync(process.argv[1], 'utf8');
const dom = new JSDOM(html, {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/',
  beforeParse(window) {
    window.fetch = async () => { throw new Error('offline'); };
    window.setInterval = () => 1;
  },
});
setTimeout(() => {
  const { document, MouseEvent, KeyboardEvent } = dom.window;
  const dialog = document.getElementById('orbit-game');
  const player = document.getElementById('orbit-game-player');
  const logo = document.querySelector('.sidebar-brand');
  if (!dialog || !player || !logo || !dialog.hidden) process.exit(1);
  if (dialog.querySelector('[data-score]') || /score/i.test(dialog.textContent)) process.exit(2);

  logo.focus();
  const launch = new MouseEvent('click', { bubbles: true, cancelable: true, shiftKey: true });
  logo.dispatchEvent(launch);
  if (!launch.defaultPrevented || dialog.hidden || document.activeElement !== player) process.exit(3);

  const initialX = player.getAttribute('cx');
  player.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true }));
  if (player.getAttribute('cx') === initialX || player.getAttribute('aria-valuenow') === '270') process.exit(4);
  document.getElementById('orbit-game-right').click();
  if (player.getAttribute('aria-valuenow') === '280') process.exit(5);

  player.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }));
  if (!dialog.hidden || document.activeElement !== logo) process.exit(6);
  dom.window.close();
}, 30);
`
	if output, err := exec.Command("node", "-e", script, htmlPath).CombinedOutput(); err != nil {
		t.Fatalf("orbit game interaction failed: %v\n%s", err, output)
	}
}
