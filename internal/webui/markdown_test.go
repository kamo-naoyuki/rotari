package webui

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The report modal renders a run report's Markdown, notes and log lines
// included. Text written by a job or an agent must stay text: raw HTML is
// escaped and a link keeps only an http(s) target.
func TestRenderMarkdownRendersReportsAndEscapesText(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	var sources []string
	for _, file := range []string{"assets/web_app_bootstrap.js", "assets/web_app_markdown.js"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, string(source))
	}
	bootstrap := sources[0]
	start := strings.Index(bootstrap, "function esc(")
	end := strings.Index(bootstrap, "function initOrbitGame(")
	if start < 0 || end <= start {
		t.Fatal("esc not found before initOrbitGame")
	}
	code, err := json.Marshal(bootstrap[start:end] + "\n" + sources[1])
	if err != nil {
		t.Fatal(err)
	}
	script := `
const vm = require('vm');
const context = {};
vm.createContext(context);
vm.runInContext(` + string(code) + `, context);
const report = [
  '# rotari run report',
  '',
  '- Run ID: ` + "`run-1`" + `',
  '- Source: ` + "`git abc (clean)`" + `',
  '',
  '## Notes',
  '',
  '**2026-10-10 20:20 JST**',
  '',
  'Sweep *LR* with <script>alert(1)</script> and [docs](https://example.test/a?b=1&c=2).',
  'Bad [link](javascript:alert(1)) and <img src=x onerror=alert(1)>',
  '',
  '1. first',
  '2. second',
  '   - nested',
  '',
  '> quoted',
  '',
  '## Jobs',
  '',
  '| Job | LR | Last log line |',
  '| --- | --- | --- |',
  '| train | ` + "`0.1`" + ` | ` + "`` ValueError: a \\\\| `b` ``" + ` |',
  '',
  '### Command',
  '` + "```json" + `',
  '["sh","-c","echo <b>"]',
  '` + "```" + `',
].join('\n');
const html = context.renderMarkdown(report);
const want = [
  '<h1>rotari run report</h1>',
  '<li>Run ID: <code>run-1</code></li>',
  '<h2>Notes</h2>',
  '<p><strong>2026-10-10 20:20 JST</strong></p>',
  'Sweep <em>LR</em> with &lt;script&gt;alert(1)&lt;/script&gt; and <a href="https://example.test/a?b=1&amp;c=2" target="_blank" rel="noopener noreferrer">docs</a>.',
  'Bad [link](javascript:alert(1)) and &lt;img src=x onerror=alert(1)&gt;',
  '<ol><li>first</li><li>second\n<ul><li>nested</li></ul></li></ol>',
  '<blockquote><p>quoted</p></blockquote>',
  '<thead><tr><th>Job</th><th>LR</th><th>Last log line</th></tr></thead>',
  '<td>train</td><td><code>0.1</code></td><td><code>ValueError: a | ` + "`b`" + `</code></td>',
  '<pre><code>[&quot;sh&quot;,&quot;-c&quot;,&quot;echo &lt;b&gt;&quot;]</code></pre>',
];
for (const piece of want) {
  if (!html.includes(piece)) throw new Error('missing ' + piece + '\n' + html);
}
for (const bad of ['<script', '<img', 'href="javascript']) {
  if (html.includes(bad)) throw new Error('unescaped ' + bad + '\n' + html);
}
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("renderMarkdown: %v\n%s", err, output)
	}
}
