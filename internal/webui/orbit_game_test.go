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
    const randomValues = [0.1, 0.9, 0.3, 0.7, 0.2, 0.8];
    let randomIndex = 0;
    window.Math.random = () => randomValues[randomIndex++ % randomValues.length];
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

  logo.focus();
  const launch = new MouseEvent('click', { bubbles: true, cancelable: true, shiftKey: true });
  logo.dispatchEvent(launch);
  if (!launch.defaultPrevented || dialog.hidden || document.activeElement !== document.getElementById('orbit-game-left')) process.exit(3);
  if (whites().length !== 6 || whites().some(ball => Number(ball.getAttribute('r')) < 7 || Number(ball.getAttribute('r')) > 17)) process.exit(2);

  dom.window.__runFrame(0);
  let balls = [player, ...whites()];
  const initial = balls.map(ball => Number(ball.dataset.angle));
  dom.window.__runFrame(50);
  for (let index = 1; index < balls.length; index++) {
    const angularVelocity = Number(balls[index].dataset.angularVelocity);
    if (Math.cos(initial[index]) < 0 && angularVelocity >= 0) process.exit(13);
    if (Math.cos(initial[index]) > 0 && angularVelocity <= 0) process.exit(14);
  }
  if (Number(player.dataset.angularVelocity) >= 0) process.exit(15);
  const naturalSpeeds = whites().map((ball, index) => ({
    radius: Number(ball.getAttribute('r')),
    speed: Math.abs(Number(ball.dataset.angularVelocity) / Math.cos(initial[index + 1])),
  }));
  naturalSpeeds.sort((a, b) => a.radius - b.radius);
  if (!(naturalSpeeds[0].speed > naturalSpeeds.at(-1).speed)) process.exit(4);

  for (let frame = 100; frame <= 3000 && document.getElementById('orbit-game-over').hidden; frame += 50) {
    dom.window.__runFrame(frame);
  }
  const whiteAngles = whites().map(ball => Number(ball.dataset.angle));
  const spread = Math.hypot(
    whiteAngles.reduce((sum, angle) => sum + Math.cos(angle), 0) / whiteAngles.length,
    whiteAngles.reduce((sum, angle) => sum + Math.sin(angle), 0) / whiteAngles.length,
  );
  if (spread > 0.98) process.exit(17);

  document.getElementById('orbit-game-restart').click();
  balls = [player, ...whites()];
  dom.window.__runFrame(0);
  const beforeRight = balls.map(ball => Number(ball.dataset.angularVelocity));
  dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true }));
  dom.window.__runFrame(100);
  const afterRight = balls.map(ball => Number(ball.dataset.angularVelocity));
  if (afterRight.some((velocity, index) => velocity <= beforeRight[index])) process.exit(5);
  dialog.dispatchEvent(new KeyboardEvent('keyup', { key: 'ArrowRight', bubbles: true }));

  const beforeLeft = balls.map(ball => Number(ball.dataset.angularVelocity));
  dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true, cancelable: true }));
  dom.window.__runFrame(150);
  const afterLeft = balls.map(ball => Number(ball.dataset.angularVelocity));
  if (afterLeft.some((velocity, index) => velocity >= beforeLeft[index])) process.exit(6);
  dialog.dispatchEvent(new KeyboardEvent('keyup', { key: 'ArrowLeft', bubbles: true }));

  const beforeButton = balls.map(ball => Number(ball.dataset.angularVelocity));
  document.getElementById('orbit-game-right').click();
  const afterButton = balls.map(ball => Number(ball.dataset.angularVelocity));
  if (afterButton.some((velocity, index) => velocity <= beforeButton[index])) process.exit(11);

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
