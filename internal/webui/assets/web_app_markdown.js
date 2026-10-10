// renderMarkdown turns the Markdown of a run report, notes included, into
// HTML. It covers what reports and short notes use: headings, paragraphs,
// lists, block quotes, fenced code, tables, code spans, emphasis, and links.
// Every piece of text is escaped first, and a link keeps only an http(s)
// target, so text written by a job or an agent cannot add markup or script.
// Syntax it does not cover stays visible as text.
function renderMarkdown(source) {
  const lines = String(source || "")
    .replace(/\r\n?/g, "\n")
    .split("\n");
  return renderMarkdownBlocks(lines);
}
function renderMarkdownBlocks(lines) {
  const html = [];
  let index = 0;
  const isTableDivider = (line) =>
    /^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?\s*$/.test(line);
  const listItem = (line) => line.match(/^(\s*)([-*+]|\d+[.)])\s+(.*)$/);
  const startsBlock = (line) =>
    /^\s*(#{1,6}\s|```|>)/.test(line) || !!listItem(line);
  while (index < lines.length) {
    const line = lines[index];
    if (!line.trim()) {
      index++;
      continue;
    }
    const fence = line.match(/^\s*(`{3,})(.*)$/);
    if (fence) {
      const body = [];
      index++;
      while (index < lines.length && !lines[index].trim().startsWith(fence[1]))
        body.push(lines[index++]);
      index++;
      html.push("<pre><code>" + esc(body.join("\n")) + "</code></pre>");
      continue;
    }
    const heading = line.match(/^\s*(#{1,6})\s+(.*?)\s*#*\s*$/);
    if (heading) {
      const level = heading[1].length;
      html.push(
        "<h" +
          level +
          ">" +
          renderMarkdownInline(heading[2]) +
          "</h" +
          level +
          ">",
      );
      index++;
      continue;
    }
    if (
      line.includes("|") &&
      index + 1 < lines.length &&
      isTableDivider(lines[index + 1])
    ) {
      const header = splitMarkdownRow(line);
      index += 2;
      const rows = [];
      while (
        index < lines.length &&
        lines[index].includes("|") &&
        lines[index].trim()
      )
        rows.push(splitMarkdownRow(lines[index++]));
      html.push(
        '<div class="markdown-table"><table><thead><tr>' +
          header
            .map((cell) => "<th>" + renderMarkdownInline(cell) + "</th>")
            .join("") +
          "</tr></thead><tbody>" +
          rows
            .map(
              (row) =>
                "<tr>" +
                header
                  .map(
                    (_, column) =>
                      "<td>" +
                      renderMarkdownInline(row[column] || "") +
                      "</td>",
                  )
                  .join("") +
                "</tr>",
            )
            .join("") +
          "</tbody></table></div>",
      );
      continue;
    }
    if (/^\s*>/.test(line)) {
      const quoted = [];
      while (index < lines.length && /^\s*>/.test(lines[index]))
        quoted.push(lines[index++].replace(/^\s*>\s?/, ""));
      html.push(
        "<blockquote>" + renderMarkdownBlocks(quoted) + "</blockquote>",
      );
      continue;
    }
    const item = listItem(line);
    if (item) {
      const ordered = /\d/.test(item[2]);
      const indent = item[1].length;
      const items = [];
      while (index < lines.length) {
        const current = listItem(lines[index]);
        if (current && current[1].length === indent) {
          items.push([current[3]]);
          index++;
        } else if (
          lines[index].trim() &&
          /^\s/.test(lines[index]) &&
          items.length
        ) {
          // A more indented line continues the item, as a nested list or
          // a further line of its text.
          items[items.length - 1].push(
            lines[index].slice(Math.min(indent + 2, lines[index].search(/\S/))),
          );
          index++;
        } else break;
      }
      const tag = ordered ? "ol" : "ul";
      html.push(
        "<" +
          tag +
          ">" +
          items
            .map((body) => {
              const inner =
                body.length === 1
                  ? renderMarkdownInline(body[0])
                  : renderMarkdownBlocks(body).replace(
                      /^<p>([\s\S]*?)<\/p>/,
                      "$1",
                    );
              return "<li>" + inner + "</li>";
            })
            .join("") +
          "</" +
          tag +
          ">",
      );
      continue;
    }
    const paragraph = [];
    while (
      index < lines.length &&
      lines[index].trim() &&
      !startsBlock(lines[index]) &&
      !(
        lines[index].includes("|") &&
        index + 1 < lines.length &&
        isTableDivider(lines[index + 1])
      )
    )
      paragraph.push(lines[index++].trim());
    if (!paragraph.length) paragraph.push(lines[index++].trim());
    html.push("<p>" + renderMarkdownInline(paragraph.join("\n")) + "</p>");
  }
  return html.join("\n");
}
// splitMarkdownRow splits a table row at the pipes that are not escaped.
function splitMarkdownRow(line) {
  const cells = [];
  let cell = "";
  const text = line.trim().replace(/^\|/, "").replace(/\|$/, "");
  for (let index = 0; index < text.length; index++) {
    if (text[index] === "\\" && text[index + 1] === "|") {
      cell += "|";
      index++;
    } else if (text[index] === "|") {
      cells.push(cell.trim());
      cell = "";
    } else cell += text[index];
  }
  cells.push(cell.trim());
  return cells;
}
function renderMarkdownInline(text) {
  let html = "";
  let index = 0;
  while (index < text.length) {
    const ticks = text.slice(index).match(/^`+/);
    if (ticks) {
      const close = text.indexOf(ticks[0], index + ticks[0].length);
      if (close !== -1) {
        let code = text.slice(index + ticks[0].length, close);
        if (/^ .* $/.test(code)) code = code.slice(1, -1);
        html += "<code>" + esc(code) + "</code>";
        index = close + ticks[0].length;
        continue;
      }
    }
    const link = text
      .slice(index)
      .match(/^\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/);
    if (link) {
      html +=
        '<a href="' +
        esc(link[2]) +
        '" target="_blank" rel="noopener noreferrer">' +
        renderMarkdownInline(link[1]) +
        "</a>";
      index += link[0].length;
      continue;
    }
    const strong = text.slice(index).match(/^(\*\*|__)(?=\S)([\s\S]*?\S)\1/);
    if (strong) {
      html += "<strong>" + renderMarkdownInline(strong[2]) + "</strong>";
      index += strong[0].length;
      continue;
    }
    const emphasis = text.slice(index).match(/^\*(?=\S)([^*]*?\S)\*/);
    if (emphasis) {
      html += "<em>" + renderMarkdownInline(emphasis[1]) + "</em>";
      index += emphasis[0].length;
      continue;
    }
    if (text[index] === "\n") html += "<br>";
    else html += esc(text[index]);
    index++;
  }
  return html;
}
