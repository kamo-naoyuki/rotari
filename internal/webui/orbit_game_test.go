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
    let nextFrameId = 0;
    const frames = new Map();
    window.requestAnimationFrame = callback => {
      const id = ++nextFrameId;
      frames.set(id, callback);
      return id;
    };
    window.cancelAnimationFrame = id => frames.delete(id);
    window.__runFrame = timestamp => {
      const entry = frames.entries().next().value;
      if (!entry) throw new Error('no animation frame is queued');
      const [id, callback] = entry;
      frames.delete(id);
      callback(timestamp);
    };
  },
});
setTimeout(() => {
  const { document, MouseEvent, KeyboardEvent } = dom.window;
  const dialog = document.getElementById('orbit-game');
  const player = document.getElementById('orbit-game-player');
  const logo = document.querySelector('.sidebar-brand');
  if (!dialog || !player || !logo || !dialog.hidden) process.exit(1);
  const whites = () => [...document.querySelectorAll('#orbit-game-white-balls circle')];
  const angles = () => [player, ...whites()].map(ball => Number(ball.getAttribute('data-angle')));
  const radiansMoved = (before, after) => {
    let difference = after - before;
    while (difference > Math.PI) difference -= Math.PI * 2;
    while (difference < -Math.PI) difference += Math.PI * 2;
    return difference;
  };

  logo.focus();
  const launch = new MouseEvent('click', { bubbles: true, cancelable: true, shiftKey: true });
  logo.dispatchEvent(launch);
  if (!launch.defaultPrevented || dialog.hidden || document.activeElement !== document.getElementById('orbit-game-left')) process.exit(3);
  if (whites().length !== 6 || whites().some(ball => Number(ball.getAttribute('r')) < 7 || Number(ball.getAttribute('r')) > 17)) process.exit(2);

  dom.window.__runFrame(0);
  const initial = angles();
  const initialPositions = [player, ...whites()].map(ball => ({
    x: Number(ball.getAttribute('cx')),
    y: Number(ball.getAttribute('cy')),
  }));
  dom.window.__runFrame(1000);
  const afterGravity = [player, ...whites()].map(ball => ({
    x: Number(ball.getAttribute('cx')),
    y: Number(ball.getAttribute('cy')),
  }));
  for (let index = 1; index < initialPositions.length; index++) {
    if (initialPositions[index].x < 0 && afterGravity[index].y >= initialPositions[index].y) process.exit(13);
    if (initialPositions[index].x > 0 && afterGravity[index].y <= initialPositions[index].y) process.exit(14);
  }
  if (afterGravity[0].x <= initialPositions[0].x) process.exit(15);
  const naturalDeltas = whites().map((ball, index) => ({
    radius: Number(ball.getAttribute('r')),
    delta: radiansMoved(initial[index + 1], Number(ball.getAttribute('data-angle'))),
  }));
  if (radiansMoved(initial[0], Number(player.getAttribute('data-angle'))) <= 0) process.exit(12);
  naturalDeltas.sort((a, b) => a.radius - b.radius);
  if (!(naturalDeltas[0].delta > naturalDeltas.at(-1).delta)) process.exit(4);

  const beforeRight = angles();
  dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true }));
  dom.window.__runFrame(1050);
  const afterRight = angles();
  if (afterRight.some((angle, index) => radiansMoved(beforeRight[index], angle) <= 0)) process.exit(5);
  dialog.dispatchEvent(new KeyboardEvent('keyup', { key: 'ArrowRight', bubbles: true }));

  const beforeLeft = angles();
  dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true, cancelable: true }));
  dom.window.__runFrame(1100);
  const afterLeft = angles();
  if (afterLeft.some((angle, index) => radiansMoved(beforeLeft[index], angle) >= 0)) process.exit(6);
  dialog.dispatchEvent(new KeyboardEvent('keyup', { key: 'ArrowLeft', bubbles: true }));

  const beforeButton = angles();
  document.getElementById('orbit-game-right').click();
  const afterButton = angles();
  if (afterButton.some((angle, index) => radiansMoved(beforeButton[index], angle) <= 0)) process.exit(11);

  player.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }));
  if (!dialog.hidden || document.activeElement !== logo) process.exit(7);

  logo.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, shiftKey: true }));
  dom.window.__runFrame(0);
  dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true }));
  for (let frame = 1; frame <= 2000 && document.getElementById('orbit-game-over').hidden; frame++) {
    dom.window.__runFrame(frame * 50);
  }
  const gameOver = document.getElementById('orbit-game-over');
  if (gameOver.hidden || !gameOver.textContent.includes('Game over')) process.exit(8);
  document.getElementById('orbit-game-restart').click();
  if (!gameOver.hidden || whites().length !== 6) process.exit(9);
  dom.window.__runFrame(0);
  document.getElementById('orbit-game-close').click();
  if (!dialog.hidden) process.exit(10);
  dom.window.close();
}, 30);
`
	if output, err := exec.Command("node", "-e", script, htmlPath).CombinedOutput(); err != nil {
		t.Fatalf("orbit game interaction failed: %v\n%s", err, output)
	}
}
